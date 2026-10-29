package uki_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/kairos-io/AuroraBoot/pkg/uki"
)

// Writing to /dev/full: every write fails with ENOSPC. Data is small so the
// encoder buffers it all and only writes on Close.
func TestZstdFileDevFull(t *testing.T) {
	src := filepath.Join(t.TempDir(), "in.cpio")
	if err := os.WriteFile(src, bytes.Repeat([]byte("kairos-initrd-"), 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	err := uki.ZstdFile(src, "/dev/full")
	t.Logf("ZstdFile(src, /dev/full) returned: %v", err)
	if err == nil {
		t.Errorf("BUG: ZstdFile returned nil although every write to /dev/full fails with ENOSPC")
	}
}

// Sanity: a normal target round-trips, so the function is otherwise fine.
func TestZstdFileRoundTrip(t *testing.T) {
	d := t.TempDir()
	payload := bytes.Repeat([]byte("kairos-initrd-"), 4096)
	src := filepath.Join(d, "in.cpio")
	dst := filepath.Join(d, "initrd")
	os.WriteFile(src, payload, 0o644)
	if err := uki.ZstdFile(src, dst); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst)
	dec, _ := zstd.NewReader(nil)
	out, err := dec.DecodeAll(b, nil)
	if err != nil || !bytes.Equal(out, payload) {
		t.Fatalf("roundtrip failed: %v", err)
	}
	t.Logf("roundtrip ok: %d -> %d bytes", len(payload), len(b))
}
