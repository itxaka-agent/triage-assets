package uki

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/constants"
	sdkLogger "github.com/kairos-io/kairos/v4/sdk/types/logger"
)

// Mirrors Build(): entries := append(getUkiCmdline(...), getUkiSingleCmdlines(...)...)
func TestF9SingleEfiCollision(t *testing.T) {
	log := sdkLogger.NewKairosLogger("t", "info", false)
	singles := []string{"Debug: rd.debug", "debug: rd.break", "Norole: console=ttyS0"}
	entries := append(
		getUkiCmdline(constants.UkiCmdline, "", "Kairos", nil, false),
		getUkiSingleCmdlines(constants.UkiCmdline, "Kairos", singles, log)...,
	)
	for _, e := range entries {
		t.Logf("entry title=%q filename=%q cmdline=%q", e.Title, e.FileName, e.Cmdline)
	}

	src := t.TempDir()
	// Simulate ukify per entry exactly where Build writes it: OutUKIPath = src/<FileName>.efi
	for _, e := range entries {
		if err := os.WriteFile(filepath.Join(src, e.FileName+".efi"), []byte("UKI for: "+e.Cmdline), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := createConfFiles(constants.UkiCmdline, src, e.Cmdline, e.Title, e.FileName, "v1", "0", false, true); err != nil {
			t.Fatal(err)
		}
	}
	efis, _ := filepath.Glob(filepath.Join(src, "*.efi"))
	confs, _ := filepath.Glob(filepath.Join(src, "entries", "*.conf"))
	t.Logf("%d entries -> %d efi files on disk, %d conf files", len(entries), len(efis), len(confs))
	for _, f := range efis {
		b, _ := os.ReadFile(f)
		t.Logf("  %s: %s", filepath.Base(f), b)
	}
	for _, f := range confs {
		b, _ := os.ReadFile(f)
		t.Logf("  entries/%s:\n%s", filepath.Base(f), b)
	}

	// files imageFiles needs besides the entries
	keys := t.TempDir()
	for _, f := range []string{"PK.auth", "KEK.auth", "db.auth"} {
		os.WriteFile(filepath.Join(keys, f), []byte("x"), 0o644)
	}
	os.WriteFile(filepath.Join(src, "BOOTX64.EFI"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(src, "loader.conf"), []byte("x"), 0o644)

	fm, err := imageFiles(src, keys, entries, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("imageFiles EFI/kairos = %v", fm["EFI/kairos"])

	// Real ISO path: createISO -> sumFileSizes, createImgWithSize, createImgDirs, copyFilesToImg
	img := filepath.Join(t.TempDir(), "efiboot.img")
	size, err := sumFileSizes(fm)
	if err != nil {
		t.Fatal(err)
	}
	if err := createImgWithSize(img, size+1); err != nil {
		t.Fatal(err)
	}
	if err := createImgDirs(img, fm); err != nil {
		t.Fatal(err)
	}
	err = copyFilesToImg(img, fm)
	t.Logf("copyFilesToImg err = %v", err)

	if len(efis) != len(entries) || err != nil {
		t.Errorf("COLLISION: %d boot entries but %d distinct EFI files; ISO copy err=%v", len(entries), len(efis), err)
	}
}
