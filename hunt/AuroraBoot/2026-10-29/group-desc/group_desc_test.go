package huntrepro

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

func TestClearGroupDescription(t *testing.T) {
	s, err := gormstore.New(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	gs := &gormstore.GroupStoreAdapter{S: s}
	g := &store.NodeGroup{Name: "prod", Description: "old desc"}
	if err := gs.Create(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	h := handlers.NewGroupHandler(gs)
	e.PUT("/api/v1/groups/:id", h.Update)

	// Same body the UI sends when the user empties the description field.
	body := `{"name":"prod","description":""}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/"+g.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	t.Logf("PUT status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))

	var resp store.NodeGroup
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	got, err := gs.GetByID(context.Background(), g.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stored description after PUT: %q", got.Description)
	if rec.Code != http.StatusOK || got.Description != "" || resp.Description != "" {
		t.Fatalf("description not cleared: status=%d resp=%q stored=%q", rec.Code, resp.Description, got.Description)
	}
}
