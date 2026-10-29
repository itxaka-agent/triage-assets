package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// hookStore wraps the real GORM-backed ArtifactStore. After the handler's
// GetByID returns its snapshot, afterGet runs once: this places a builder
// phase write deterministically inside the window between the handler's read
// and its Update.
type hookStore struct {
	store.ArtifactStore
	once     sync.Once
	afterGet func()
}

func (h *hookStore) GetByID(ctx context.Context, id string) (*store.ArtifactRecord, error) {
	rec, err := h.ArtifactStore.GetByID(ctx, id)
	if h.afterGet != nil {
		h.once.Do(h.afterGet)
	}
	return rec, err
}

func newRealStore(t *testing.T) store.ArtifactStore {
	s, err := gormstore.New(filepath.Join(t.TempDir(), "ab.db"))
	if err != nil {
		t.Fatal(err)
	}
	return &gormstore.ArtifactStoreAdapter{S: s}
}

func patch(t *testing.T, h *handlers.ArtifactHandler, id, body string) int {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/artifacts/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	if err := h.Update(c); err != nil {
		t.Fatal(err)
	}
	return rec.Code
}

// Deterministic interleaving: builder (operator watchCRPhase-style
// column-scoped writes) moves the build to Ready, sets files and clears the
// upload token while PATCH holds its snapshot.
func TestPatchWritesBackStaleSnapshot(t *testing.T) {
	ctx := context.Background()
	real := newRealStore(t)
	if err := real.Create(ctx, &store.ArtifactRecord{ID: "a1", Name: "old", Phase: store.ArtifactBuilding, Message: "building...", UploadToken: "tokenhash"}); err != nil {
		t.Fatal(err)
	}
	hs := &hookStore{ArtifactStore: real, afterGet: func() {
		_ = real.UpdatePhaseMessage(ctx, "a1", store.ArtifactReady, "")
		_ = real.UpdateFiles(ctx, "a1", []string{"kairos.iso"})
		_ = real.ClearUploadToken(ctx, "a1")
	}}
	h := handlers.NewArtifactHandler(&fakeBuilder{}, hs, nil, nil, nil, nil, t.TempDir(), "reg", "http://x")

	code := patch(t, h, "a1", `{"name":"new"}`)
	got, _ := real.GetByID(ctx, "a1")
	t.Logf("PATCH code=%d; after: name=%q phase=%q message=%q files=%v uploadToken=%q",
		code, got.Name, got.Phase, got.Message, got.ArtifactFiles, got.UploadToken)
	if got.Phase != store.ArtifactReady {
		t.Errorf("phase reverted: want %q, got %q", store.ArtifactReady, got.Phase)
	}
	if len(got.ArtifactFiles) != 1 {
		t.Errorf("artifact files reverted: got %v", got.ArtifactFiles)
	}
	if got.UploadToken != "" {
		t.Errorf("cleared upload token restored: got %q", got.UploadToken)
	}
}

// Non-deterministic: no hook. A writer flips phase Building<->Ready via
// UpdatePhaseMessage while PATCH requests run concurrently; count PATCHes
// whose response/DB state shows the phase it snapshotted, not the latest.
func TestPatchRaceUnhooked(t *testing.T) {
	ctx := context.Background()
	real := newRealStore(t)
	_ = real.Create(ctx, &store.ArtifactRecord{ID: "a1", Name: "old", Phase: store.ArtifactBuilding})
	h := handlers.NewArtifactHandler(&fakeBuilder{}, real, nil, nil, nil, nil, t.TempDir(), "reg", "http://x")

	var lost int64
	const N = 300
	for i := 0; i < N; i++ {
		_ = real.UpdatePhaseMessage(ctx, "a1", store.ArtifactBuilding, "b")
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); patch(t, h, "a1", `{"saved":true}`) }()
		go func() { defer wg.Done(); _ = real.UpdatePhaseMessage(ctx, "a1", store.ArtifactReady, "") }()
		wg.Wait()
		got, _ := real.GetByID(ctx, "a1")
		if got.Phase != store.ArtifactReady {
			atomic.AddInt64(&lost, 1)
		}
	}
	t.Logf("unhooked: %d/%d iterations ended with phase reverted to Building after the Ready write completed", lost, N)
}
