package secureboot

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyF4_ExistingKeysDirKeepsPCRKey(t *testing.T) {
	fleet := filepath.Join(t.TempDir(), "fleet")
	if err := GenerateKeySet(Options{Name: "fleet", OutputDir: fleet, SkipMicrosoftCerts: true}); err != nil {
		t.Fatalf("initial: %v", err)
	}
	orig, err := os.ReadFile(filepath.Join(fleet, "tpm2-pcr-private.pem"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "rebuilt")
	if err := GenerateKeySet(Options{ExistingKeysDir: fleet, OutputDir: out, SkipMicrosoftCerts: true}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	for _, f := range []string{"PK.key", "KEK.key", "db.key"} {
		a, _ := os.ReadFile(filepath.Join(fleet, f))
		b, _ := os.ReadFile(filepath.Join(out, f))
		t.Logf("%s identical: %v", f, bytes.Equal(a, b))
	}
	got, err := os.ReadFile(filepath.Join(out, "tpm2-pcr-private.pem"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tpm2-pcr-private.pem identical: %v", bytes.Equal(orig, got))
	if !bytes.Equal(orig, got) {
		t.Fatalf("tpm2-pcr-private.pem in ExistingKeysDir was ignored; a new PCR policy key was minted")
	}
}
