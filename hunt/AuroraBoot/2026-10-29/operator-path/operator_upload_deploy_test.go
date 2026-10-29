package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/isoserve"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

var _ = Describe("HUNT f2: operator-uploaded ISO via local serve", func() {
	It("deploys an artifact whose ISO arrived through Upload", func() {
		e := echo.New()
		artifactsDir := GinkgoT().TempDir()
		const id, tok = "op-build", "tok123"
		as := &fakeArtifactStore{records: []*store.ArtifactRecord{
			{ID: id, Phase: store.ArtifactBuilding, UploadToken: sha256Hex(tok)},
		}}
		ah := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, nil, nil, artifactsDir, "reg", "http://localhost:8080")

		req := httptest.NewRequest(http.MethodPut, "/api/v1/artifacts/"+id+"/upload/kairos.iso", bytes.NewReader([]byte("ISO")))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id", "*")
		c.SetParamValues(id, "kairos.iso")
		Expect(ah.Upload(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusCreated))

		r, _ := as.GetByID(context.Background(), id)
		GinkgoWriter.Printf("ArtifactFiles after Upload: %q\n", r.ArtifactFiles)
		_, statErr := os.Stat(filepath.Join(artifactsDir, id, "kairos.iso"))
		GinkgoWriter.Printf("on-disk file exists: %v\n", statErr == nil)
		_ = as.UpdatePhaseMessage(context.Background(), id, store.ArtifactReady, "")

		settings := newFakeSettingsStore()
		settings.values[handlers.SettingLocalServeEnabled] = "true"
		serve := isoserve.New(isoserve.Config{BaseURL: "http://10.0.0.5:8090"})
		dh := handlers.NewDeployHandler(as, &fakeDeploymentStore{}, &fakeBMCTargetStore{}, nil, artifactsDir, serve, nil).WithSettings(settings)

		dreq := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/"+id+"/deploy/redfish",
			strings.NewReader(`{"endpoint":"http://10.0.0.9","username":"u","password":"p"}`))
		dreq.Header.Set("Content-Type", "application/json")
		drec := httptest.NewRecorder()
		dc := e.NewContext(dreq, drec)
		dc.SetParamNames("id")
		dc.SetParamValues(id)
		Expect(dh.DeployRedfish(dc)).To(Succeed())
		cwd, _ := os.Getwd()
		GinkgoWriter.Printf("cwd=%s\ndeploy status=%d body=%s\n", cwd, drec.Code, drec.Body.String())
		Expect(drec.Code).To(Equal(http.StatusAccepted))
	})
})
