package hunttest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/server"
)

func do(t *testing.T, srv *httptest.Server, method, path, body string) (int, string) {
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-pass")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

func TestDuplicateGroupName(t *testing.T) {
	s, err := gormstore.New(filepath.Join(t.TempDir(), "ab.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := server.New(server.Config{
		NodeStore:     &gormstore.NodeStoreAdapter{S: s},
		CommandStore:  &gormstore.CommandStoreAdapter{S: s},
		GroupStore:    &gormstore.GroupStoreAdapter{S: s},
		AdminPassword: "admin-pass",
		RegToken:      "reg-token",
		AuroraBootURL: "http://localhost:8080",
	})
	srv := httptest.NewServer(app)
	defer srv.Close()

	c1, b1 := do(t, srv, "POST", "/api/v1/groups", `{"name":"prod"}`)
	t.Logf("POST prod #1 -> %d %s", c1, b1)
	c2, b2 := do(t, srv, "POST", "/api/v1/groups", `{"name":"prod"}`)
	t.Logf("POST prod #2 -> %d %s", c2, b2)
	c3, b3 := do(t, srv, "POST", "/api/v1/groups", `{"name":"staging"}`)
	t.Logf("POST staging -> %d %s", c3, b3)
	id := b3[strings.Index(b3, `"id":"`)+6:]
	id = id[:strings.Index(id, `"`)]
	c4, b4 := do(t, srv, "PUT", "/api/v1/groups/"+id, `{"name":"prod"}`)
	t.Logf("PUT staging->prod -> %d %s", c4, b4)
	_, b5 := do(t, srv, "GET", "/api/v1/groups", "")
	t.Logf("GET groups -> %s", b5)

	if c2 != http.StatusConflict {
		t.Errorf("duplicate create: got %d, want 409", c2)
	}
	if c4 != http.StatusConflict {
		t.Errorf("rename to existing: got %d, want 409", c4)
	}
}
