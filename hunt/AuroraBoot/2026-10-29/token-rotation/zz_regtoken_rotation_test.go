package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/server"
)

type capBuilder struct {
	mu  sync.Mutex
	cfg []string
}

func (f *capBuilder) Build(_ context.Context, opts builder.BuildOptions) (*builder.BuildStatus, error) {
	f.mu.Lock()
	f.cfg = append(f.cfg, opts.CloudConfig)
	f.mu.Unlock()
	return &builder.BuildStatus{ID: opts.ID, Phase: builder.BuildPending}, nil
}
func (f *capBuilder) Status(_ context.Context, _ string) (*builder.BuildStatus, error) {
	return nil, nil
}
func (f *capBuilder) List(_ context.Context) ([]*builder.BuildStatus, error) { return nil, nil }
func (f *capBuilder) Cancel(_ context.Context, _ string) error               { return nil }

func do(t *testing.T, method, url, auth, body string) (int, string) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestRegTokenRotationReachesBuilds(t *testing.T) {
	b := &capBuilder{}
	app := server.New(server.Config{
		NodeStore: &fakeNodeStore{}, CommandStore: &fakeCommandStore{}, GroupStore: &fakeGroupStore{},
		ArtifactStore: &fakeArtifactStore{}, Builder: b,
		AdminPassword: "admin-pass", RegToken: "startup-token", AuroraBootURL: "http://localhost:8080",
		DisableRateLimit: true,
	})
	ts := httptest.NewServer(app)
	defer ts.Close()

	code, body := do(t, "POST", ts.URL+"/api/v1/settings/registration-token/rotate", "admin-pass", "")
	t.Logf("rotate: %d %s", code, body)
	var r map[string]string
	_ = json.Unmarshal([]byte(body), &r)
	newTok := r["registrationToken"]

	code, body = do(t, "GET", ts.URL+"/api/v1/settings/registration-token", "admin-pass", "")
	t.Logf("GET token after rotate: %d %s", code, body)

	code, body = do(t, "POST", ts.URL+"/api/v1/artifacts", "admin-pass",
		`{"name":"t","baseImage":"quay.io/kairos/opensuse:leap-15.6-core-amd64-generic","variant":"core","outputs":{"iso":true}}`)
	t.Logf("create artifact: %d %s", code, body)
	if len(b.cfg) == 0 {
		t.Fatalf("builder not called")
	}
	cc := b.cfg[len(b.cfg)-1]
	for _, l := range strings.Split(cc, "\n") {
		if strings.Contains(l, "registration_token") {
			t.Logf("cloud-config line: %s", strings.TrimSpace(l))
		}
	}
	// Node boots from this ISO and registers with the embedded token.
	code, body = do(t, "POST", ts.URL+"/api/v1/nodes/register", "",
		`{"registrationToken":"startup-token","machineId":"m1","hostname":"h1"}`)
	t.Logf("register with embedded (old) token: %d %s", code, body)
	code2, body2 := do(t, "POST", ts.URL+"/api/v1/nodes/register", "",
		`{"registrationToken":"`+newTok+`","machineId":"m2","hostname":"h2"}`)
	t.Logf("register with rotated token: %d %s", code2, body2)

	if !strings.Contains(cc, newTok) {
		t.Errorf("BUG: build started after rotation embeds stale token; rotated=%s; new token present in cloud-config=%v; old token present=%v",
			newTok, strings.Contains(cc, newTok), strings.Contains(cc, "startup-token"))
	}
}

// Race: rotate concurrently with registration requests.
func TestRegTokenRotateRace(t *testing.T) {
	app := server.New(server.Config{
		NodeStore: &fakeNodeStore{}, CommandStore: &fakeCommandStore{}, GroupStore: &fakeGroupStore{},
		Builder: &capBuilder{}, AdminPassword: "admin-pass", RegToken: "startup-token",
		AuroraBootURL: "http://localhost:8080", DisableRateLimit: true,
	})
	ts := httptest.NewServer(app)
	defer ts.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); do(t, "POST", ts.URL+"/api/v1/settings/registration-token/rotate", "admin-pass", "") }()
		go func() {
			defer wg.Done()
			do(t, "POST", ts.URL+"/api/v1/nodes/register", "", `{"registrationToken":"x","machineId":"m","hostname":"h"}`)
		}()
	}
	wg.Wait()
}
