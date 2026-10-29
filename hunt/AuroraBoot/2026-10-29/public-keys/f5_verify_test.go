package uki

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/utils"
	"github.com/kairos-io/kairos/v4/sdk/types/logger"
)

func setupSource(t *testing.T) string {
	src := t.TempDir()
	for _, f := range []string{"BOOTX64.EFI", "loader.conf", "default.efi"} {
		os.WriteFile(filepath.Join(src, f), []byte("x"), 0o644)
	}
	os.MkdirAll(filepath.Join(src, "entries"), 0o755)
	os.WriteFile(filepath.Join(src, "entries", "default.conf"), []byte("x"), 0o644)
	return src
}

func TestF5ImageFilesEmptyKeysDir(t *testing.T) {
	m, _ := imageFiles(setupSource(t), "", []utils.BootEntry{{FileName: "default"}}, "amd64")
	t.Logf("loader/keys/auto = %v", m["loader/keys/auto"])
}

func TestF5CreateArtifactNoKeys(t *testing.T) {
	src := setupSource(t)
	out := t.TempDir()
	cwd := t.TempDir()
	os.Chdir(cwd)
	log := logger.NewKairosLogger("t", "info", false)
	err := createArtifact(src, out, "", []utils.BootEntry{{FileName: "default"}}, log, "amd64")
	if err != nil {
		t.Fatalf("createArtifact with empty PublicKeysDir failed: %v", err)
	}
}

func TestF5CreateArtifactPicksCWD(t *testing.T) {
	src := setupSource(t)
	out := t.TempDir()
	cwd := t.TempDir()
	for _, f := range []string{"PK.auth", "KEK.auth", "db.auth"} {
		os.WriteFile(filepath.Join(cwd, f), []byte("stray-"+f), 0o644)
	}
	os.Chdir(cwd)
	log := logger.NewKairosLogger("t", "info", false)
	if err := createArtifact(src, out, "", []utils.BootEntry{{FileName: "default"}}, log, "amd64"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(out, "loader/keys/auto/PK.auth"))
	t.Logf("shipped loader/keys/auto/PK.auth from CWD: %q err=%v", b, err)
}
