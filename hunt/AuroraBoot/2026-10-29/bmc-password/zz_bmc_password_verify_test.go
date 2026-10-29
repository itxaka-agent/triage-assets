package handlers_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/internal/secrets"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/labstack/echo/v4"
)

func nonLoopbackIP(t *testing.T) string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			return ipn.IP.String()
		}
	}
	t.Fatal("no non-loopback ipv4")
	return ""
}

func TestVerifyBMCPasswordPersist(t *testing.T) {
	// fake BMC on a non-loopback address so ValidateMediaURL accepts it
	var mu sync.Mutex
	var seen []string
	ln, err := net.Listen("tcp", nonLoopbackIP(t)+":0")
	if err != nil {
		t.Fatal(err)
	}
	bmc := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		u, p, ok := r.BasicAuth()
		mu.Lock()
		if ok {
			seen = append(seen, fmt.Sprintf("%s %s basic user=%q pass=%q", r.Method, r.URL.Path, u, p))
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "Sessions") {
			seen = append(seen, fmt.Sprintf("%s %s session-body=%s", r.Method, r.URL.Path, string(b)))
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/redfish/v1/" || r.URL.Path == "/redfish/v1" {
			_, _ = w.Write([]byte(`{"@odata.id":"/redfish/v1/","Id":"RootService","RedfishVersion":"1.6.0","SessionService":{"@odata.id":"/redfish/v1/SessionService"},"Links":{"Sessions":{"@odata.id":"/redfish/v1/SessionService/Sessions"}},"Systems":{"@odata.id":"/redfish/v1/Systems"}}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad creds"}}`))
	}))
	bmc.Listener = ln
	bmc.Start()
	defer bmc.Close()
	t.Logf("fake BMC at %s", bmc.URL)

	// real gorm store (sqlite) with the cipher the web server wires
	st, err := gormstore.New(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c, err := secrets.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	st.WithCipher(c)
	bmcStore := &gormstore.BMCTargetStoreAdapter{S: st}

	h := handlers.NewDeployHandler(nil, nil, bmcStore, nil, "", nil, nil)
	e := echo.New()
	g := e.Group("/api/v1")
	g.POST("/bmc-targets", h.CreateBMCTarget)
	g.GET("/bmc-targets", h.ListBMCTargets)
	g.PUT("/bmc-targets/:id", h.UpdateBMCTarget)
	g.POST("/bmc-targets/:id/inspect", h.InspectHardware)
	srv := httptest.NewServer(e)
	defer srv.Close()

	do := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// Same body shape as ui/src/api/deployments.ts createBMCTarget
	createBody := fmt.Sprintf(`{"name":"bmc1","endpoint":%q,"vendor":"generic","username":"admin","password":"S3cret-create","verifySSL":false}`, bmc.URL)
	code, body := do("POST", "/api/v1/bmc-targets", createBody)
	t.Logf("POST create -> %d %s", code, body)
	var created map[string]any
	_ = json.Unmarshal([]byte(body), &created)
	id, _ := created["id"].(string)

	got, err := bmcStore.GetByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("store after POST: Password=%q", got.Password)
	if got.Password != "S3cret-create" {
		t.Errorf("BUG: password sent on POST not stored (got %q)", got.Password)
	}

	code, body = do("POST", "/api/v1/bmc-targets/"+id+"/inspect", "")
	t.Logf("POST inspect -> %d %s", code, body)

	// PUT with a new password (updateBMCTarget body shape)
	putBody := fmt.Sprintf(`{"name":"bmc1","endpoint":%q,"vendor":"generic","username":"admin","password":"S3cret-update","verifySSL":false}`, bmc.URL)
	code, body = do("PUT", "/api/v1/bmc-targets/"+id, putBody)
	t.Logf("PUT update -> %d %s", code, body)
	got, _ = bmcStore.GetByID(t.Context(), id)
	t.Logf("store after PUT: Password=%q", got.Password)
	if got.Password != "S3cret-update" {
		t.Errorf("BUG: password sent on PUT not stored (got %q)", got.Password)
	}

	mu.Lock()
	for _, s := range seen {
		t.Logf("BMC saw: %s", s)
	}
	mu.Unlock()

	// Control: store layer itself keeps a password when given one directly.
	got.Password = "direct"
	_ = bmcStore.Update(t.Context(), got)
	got2, _ := bmcStore.GetByID(t.Context(), id)
	t.Logf("control: store after direct Update(Password=direct): Password=%q", got2.Password)

	if got.Password == "" || got2.Password != "direct" {
		t.Log("unexpected control")
	}
	final, _ := bmcStore.GetByID(t.Context(), id)
	_ = final
	if strings.TrimSpace(fmt.Sprint(seen)) != "" {
		t.Log("done")
	}
}
