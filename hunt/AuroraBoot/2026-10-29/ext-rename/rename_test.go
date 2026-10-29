package huntrepro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/server"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

type stubExtBuilder struct{}

func (stubExtBuilder) Build(context.Context, builder.ExtensionBuildOptions) (*builder.ExtensionBuildStatus, error) {
	return nil, fmt.Errorf("stub")
}
func (stubExtBuilder) Status(context.Context, string) (*builder.ExtensionBuildStatus, error) {
	return nil, fmt.Errorf("stub")
}
func (stubExtBuilder) List(context.Context) ([]*builder.ExtensionBuildStatus, error) { return nil, nil }
func (stubExtBuilder) Cancel(context.Context, string) error                        { return nil }

const pw = "admin-pw"

func call(t *testing.T, base, method, path string, body any) (int, string) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, rdr)
	req.Header.Set("Authorization", "Bearer "+pw)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	t.Logf("%s %s -> %d %s", method, path, resp.StatusCode, bytes.TrimSpace(out))
	return resp.StatusCode, string(out)
}

func TestRenameBundledExtension(t *testing.T) {
	dir := t.TempDir()
	st, err := gormstore.New(filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	extStore := &gormstore.ExtensionStoreAdapter{S: st}
	artStore := &gormstore.ArtifactStoreAdapter{S: st}
	// Seed a Ready sysext and a Ready artifact of the same arch, as a finished
	// build would leave them.
	if err := extStore.Create(ctx, &store.ExtensionRecord{ID: "ext1", Name: "myext", Type: "sysext", Phase: "Ready",
		Arch: "amd64", Version: "v1", RawFilename: "myext.raw", DownloadToken: "tok"}); err != nil {
		t.Fatal(err)
	}
	if err := artStore.Create(ctx, &store.ArtifactRecord{ID: "art1", Phase: "Ready", Arch: "amd64"}); err != nil {
		t.Fatal(err)
	}

	e := server.New(server.Config{
		NodeStore:                    &gormstore.NodeStoreAdapter{S: st},
		CommandStore:                 &gormstore.CommandStoreAdapter{S: st},
		GroupStore:                   &gormstore.GroupStoreAdapter{S: st},
		ArtifactStore:                artStore,
		SecureBootKeySetStore:        &gormstore.SecureBootKeySetStoreAdapter{S: st},
		ExtensionStore:               extStore,
		ArtifactExtensionBundleStore: &gormstore.ArtifactExtensionBundleStoreAdapter{S: st},
		NodeExtensionStore:           &gormstore.NodeExtensionStoreAdapter{S: st},
		ExtensionBuilder:             stubExtBuilder{},
		DeploymentStore:              &gormstore.DeploymentStoreAdapter{S: st},
		BMCTargetStore:               &gormstore.BMCTargetStoreAdapter{S: st},
		SettingsStore:                &gormstore.SettingsStoreAdapter{S: st},
		AdminPassword:                pw,
		RegToken:                     "reg",
		AuroraBootURL:                "http://ab.example",
		ArtifactsDir:                 dir,
		KeysDir:                      dir,
		DisableRateLimit:             true,
	})
	ts := httptest.NewServer(e)
	defer ts.Close()
	b := ts.URL

	code, _ := call(t, b, "PUT", "/api/v1/artifacts/art1/bundle-extensions",
		[]map[string]any{{"extensionName": "myext", "extensionType": "sysext"}})
	if code != 200 {
		t.Fatalf("set bundle: %d", code)
	}
	if code, _ = call(t, b, "POST", "/api/v1/artifacts/art1/bundle-resolve", nil); code != 200 {
		t.Fatalf("resolve before rename: %d", code)
	}
	if code, _ = call(t, b, "DELETE", "/api/v1/extensions/ext1", nil); code != 409 {
		t.Fatalf("delete guard: expected 409, got %d", code)
	}

	// Rename the bundled extension: no reference check.
	code, _ = call(t, b, "PATCH", "/api/v1/extensions/ext1", map[string]string{"name": "renamed"})
	renameOK := code == 200

	call(t, b, "GET", "/api/v1/artifacts/art1/bundle-extensions", nil)
	resolveCode, _ := call(t, b, "POST", "/api/v1/artifacts/art1/bundle-resolve", nil)
	delCode, _ := call(t, b, "DELETE", "/api/v1/extensions/ext1", nil)
	resolveCode2, _ := call(t, b, "POST", "/api/v1/artifacts/art1/bundle-resolve", nil)

	t.Logf("SUMMARY renameAccepted=%v resolveAfterRename=%d deleteAfterRename=%d resolveAfterDelete=%d",
		renameOK, resolveCode, delCode, resolveCode2)
	if renameOK && resolveCode == 400 {
		t.Errorf("BUG: rename of bundled extension accepted (200) and broke bundle-resolve (400); delete guard then bypassed (%d)", delCode)
	}
}
