package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

func call(h echo.HandlerFunc, method, id, body string) *httptest.ResponseRecorder {
	e := echo.New()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	_ = h(c)
	return rec
}

func TestVerifyOrphanBundleBlocksExtensionDelete(t *testing.T) {
	ctx := context.Background()
	s, err := gormstore.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	as := &gormstore.ArtifactStoreAdapter{S: s}
	es := &gormstore.ExtensionStoreAdapter{S: s}
	bs := &gormstore.ArtifactExtensionBundleStoreAdapter{S: s}

	art := &store.ArtifactRecord{ID: "art-1", Phase: store.ArtifactReady, Arch: "amd64"}
	if err := as.Create(ctx, art); err != nil {
		t.Fatal(err)
	}
	ext := &store.ExtensionRecord{ID: "ext-1", Name: "myext", Type: "sysext", Phase: "Ready", Arch: "amd64", Version: "1"}
	if err := es.Create(ctx, ext); err != nil {
		t.Fatal(err)
	}

	ah := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, es, bs, t.TempDir(), "", "")
	eh := handlers.NewExtensionHandler(&fakeExtensionBuilder{}, es, bs, nil, nil, t.TempDir())

	r := call(ah.SetBundleExtensions, http.MethodPut, "art-1", `[{"extensionName":"myext","extensionType":"sysext"}]`)
	t.Logf("PUT /artifacts/art-1/bundle-extensions -> %d %s", r.Code, strings.TrimSpace(r.Body.String()))

	r = call(ah.Delete, http.MethodDelete, "art-1", "")
	t.Logf("DELETE /artifacts/art-1 -> %d", r.Code)
	if _, err := as.GetByID(ctx, "art-1"); err == nil {
		t.Fatal("artifact still present")
	}

	rows, _ := bs.ListForArtifact(ctx, "art-1")
	t.Logf("bundle rows left for deleted art-1: %d", len(rows))

	r = call(eh.Delete, http.MethodDelete, "ext-1", "")
	t.Logf("DELETE /extensions/ext-1 -> %d %s", r.Code, strings.TrimSpace(r.Body.String()))

	r = call(ah.SetBundleExtensions, http.MethodPut, "art-1", `[]`)
	t.Logf("PUT /artifacts/art-1/bundle-extensions [] -> %d %s", r.Code, strings.TrimSpace(r.Body.String()))

	// DeleteByPhase path
	art2 := &store.ArtifactRecord{ID: "art-2", Phase: store.ArtifactError, Arch: "amd64"}
	_ = as.Create(ctx, art2)
	_ = bs.ReplaceForArtifact(ctx, "art-2", []store.ArtifactExtensionBundle{{ExtensionName: "myext", ExtensionType: "sysext"}})
	_ = as.DeleteByPhase(ctx, store.ArtifactError)
	rows, _ = bs.ListForArtifact(ctx, "art-2")
	t.Logf("bundle rows left after DeleteByPhase(Error) for art-2: %d", len(rows))

	if len(rows) > 0 || r.Code == http.StatusNotFound {
		t.Errorf("BUG: orphan bundle rows survive artifact deletion and block extension delete")
	}
}
