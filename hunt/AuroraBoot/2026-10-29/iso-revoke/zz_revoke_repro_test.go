package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/isoserve"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// Simulates a BMC that streams the virtual CD over HTTP Range reads (as iDRAC/
// iLO/XCC do) and keeps reading it while the installer boots after Deploy returns.
var _ = Describe("REPRO f1: local-serve ISO URL revoked when Deploy returns", func() {
	It("keeps the served ISO fetchable while the BMC is still booting the installer", func() {
		var serve *isoserve.Server
		isoTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serve.Handler().ServeHTTP(w, r)
		}))
		defer isoTS.Close()
		serve = isoserve.New(isoserve.Config{BaseURL: isoTS.URL})

		dir := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(dir, "art-1"), 0755)).To(Succeed())
		isoPath := filepath.Join(dir, "art-1", "kairos.iso")
		Expect(os.WriteFile(isoPath, []byte(strings.Repeat("ISO-BYTES-", 1000)), 0644)).To(Succeed())

		artifacts := &fakeArtifactStore{}
		Expect(artifacts.Create(context.Background(), &store.ArtifactRecord{ID: "art-1", ArtifactFiles: []string{isoPath}})).To(Succeed())
		deployments := &fakeDeploymentStore{}
		settings := newFakeSettingsStore()
		settings.values[handlers.SettingLocalServeEnabled] = "true"
		h := handlers.NewDeployHandler(artifacts, deployments, &fakeBMCTargetStore{}, nil, dir, serve, nil).WithSettings(settings)

		var mu sync.Mutex
		var image string
		var log []string
		rangeGet := func(phase string) int {
			mu.Lock()
			u := image
			mu.Unlock()
			req, _ := http.NewRequest(http.MethodGet, u, nil)
			req.Header.Set("Range", "bytes=0-99")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()
			mu.Lock()
			log = append(log, fmt.Sprintf("%s: GET Range bytes=0-99 -> %d", phase, resp.StatusCode))
			mu.Unlock()
			return resp.StatusCode
		}

		bmc := newFakeBMC()
		defer bmc.Close()
		bmc.onInsert = func(img string) {
			mu.Lock()
			image = img
			mu.Unlock()
			rangeGet("BMC at InsertMedia")
		}
		bmc.onReset = func() { rangeGet("BMC at Reset (firmware starts reading CD)") }

		e := echo.New()
		body := fmt.Sprintf(`{"endpoint":%q,"username":"u","password":"p","verifySSL":false}`, bmc.URL())
		req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/art-1/deploy/redfish", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("art-1")
		Expect(h.DeployRedfish(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusAccepted))

		// Wait for the async deploy to reach a terminal state.
		var status string
		Eventually(func() string {
			deployments.mu.Lock()
			defer deployments.mu.Unlock()
			status = deployments.deps[0].Status
			return status
		}, 30*time.Second, 200*time.Millisecond).ShouldNot(Equal(store.DeployActive))
		deployments.mu.Lock()
		GinkgoWriter.Printf("deployment status=%s message=%q\n", deployments.deps[0].Status, deployments.deps[0].Message)
		deployments.mu.Unlock()
		time.Sleep(200 * time.Millisecond) // let deferred Revoke run

		// Installer kernel/initrd/squashfs reads continue after the Reset task completes.
		code := rangeGet("installer boot after Deploy returned")
		for _, l := range log {
			GinkgoWriter.Println(l)
		}
		Expect(code).To(Equal(http.StatusPartialContent), "BMC can no longer read the virtual CD mid-boot")
	})
})
