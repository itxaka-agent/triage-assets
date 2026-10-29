package uki

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/extensions"
	"github.com/kairos-io/AuroraBoot/pkg/utils"
	"github.com/kairos-io/kairos/v4/sdk/types/logger"
)

// fakePath builds a PATH dir that links only the given tools from the real PATH.
func fakePath(t *testing.T, tools ...string) string {
	d := t.TempDir()
	for _, tool := range tools {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("host tool %s missing: %v", tool, err)
		}
		if err := os.Symlink(real, filepath.Join(d, tool)); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestF8DepsMissingMformatPasses(t *testing.T) {
	t.Setenv("PATH", fakePath(t, "dd", "mkfs.msdos", "mmd", "mcopy", "xorriso"))
	err := checkBuildUKIDeps("amd64")
	t.Logf("checkBuildUKIDeps without mformat on PATH -> err=%v", err)
	if err == nil {
		_, lerr := exec.LookPath("mformat")
		t.Logf("but mformat lookup: %v", lerr)
		t.Errorf("BUG: dependency check passed although mformat (run by createImgWithSize) is missing")
	}
}

func TestF8DepsMissingXorrisoFailsForAllTypes(t *testing.T) {
	t.Setenv("PATH", fakePath(t, "dd", "mkfs.msdos", "mmd", "mcopy", "mformat"))
	err := checkBuildUKIDeps("amd64")
	t.Logf("checkBuildUKIDeps without xorriso (signature has no outputType) -> err=%v", err)
	if err != nil {
		t.Errorf("BUG: xorriso demanded even though only ISO output runs it: %v", err)
	}
}

func TestF8DepsMissingMkfsMsdosFails(t *testing.T) {
	t.Setenv("PATH", fakePath(t, "dd", "mmd", "mcopy", "mformat", "xorriso"))
	err := checkBuildUKIDeps("amd64")
	t.Logf("checkBuildUKIDeps without mkfs.msdos -> err=%v", err)
	if err != nil {
		t.Errorf("BUG: mkfs.msdos demanded though nothing in build-uki runs it: %v", err)
	}
}

func fakeSource(t *testing.T) (string, string) {
	src := t.TempDir()
	keys := t.TempDir()
	must := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(strings.Repeat("x", 4096)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(src, "BOOTX64.EFI"))
	must(filepath.Join(src, "loader.conf"))
	must(filepath.Join(src, "entry.efi"))
	must(filepath.Join(src, "entries", "entry.conf"))
	for _, k := range []string{"PK.auth", "KEK.auth", "db.auth"} {
		must(filepath.Join(keys, k))
	}
	return src, keys
}

func TestF8IsoMissingOutputDir(t *testing.T) {
	materializeExtensions = func(context.Context, []string, []extensions.Request, string, string, bool) ([]string, error) { return nil, nil }
	src, keys := fakeSource(t)
	out := filepath.Join(t.TempDir(), "does", "not", "exist")
	log := logger.NewKairosLogger("f8", "info", false)
	err := createISO(nil, src, out, "", keys, "out", "", []utils.BootEntry{{FileName: "entry"}}, log, "amd64", nil, nil, false)
	t.Logf("createISO into missing dir -> err=%v", err)
	if err != nil {
		t.Errorf("BUG: iso output into non-existent -d fails at xorriso step")
	}
	_, statErr := os.Stat(out)
	t.Logf("output dir after createISO: %v", statErr)
}

func TestF8DefaultMissingOutputDir(t *testing.T) {
	src, keys := fakeSource(t)
	out := filepath.Join(t.TempDir(), "does", "not", "exist")
	log := logger.NewKairosLogger("f8", "info", false)
	err := createArtifact(src, out, keys, []utils.BootEntry{{FileName: "entry"}}, log, "amd64")
	t.Logf("createArtifact (default uki type) into missing dir -> err=%v", err)
	if err != nil {
		t.Errorf("default type failed: %v", err)
	}
}
