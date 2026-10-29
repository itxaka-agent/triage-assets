package secureboot

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/foxboron/go-uefi/efi/signature"
	efiutil "github.com/foxboron/go-uefi/efi/util"
)

func f3Cert(t *testing.T, cn string) []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// TestF3_CustomCertDirDropsSHA256 builds a firmware-style db export with one
// X509 cert and one SHA256 hash entry, runs GenerateKeySet with it as
// CustomCertDir, and checks whether the hash survives in db.esl.
func TestF3_CustomCertDirDropsSHA256(t *testing.T) {
	owner := *efiutil.StringToGUID("77fa9abd-0359-4d32-bd60-28f4e78f784b")
	fwDir := t.TempDir()

	dbCert := f3Cert(t, "firmware-db-cert")
	hash := sha256.Sum256([]byte("some-efi-binary"))

	db := signature.NewSignatureDatabase()
	if err := db.Append(signature.CERT_X509_GUID, owner, dbCert); err != nil {
		t.Fatal(err)
	}
	if err := db.Append(signature.CERT_SHA256_GUID, owner, hash[:]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fwDir, "db"), db.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	kek := signature.NewSignatureDatabase()
	if err := kek.Append(signature.CERT_X509_GUID, owner, f3Cert(t, "firmware-kek-cert")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fwDir, "KEK"), kek.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sanity: the input really carries the hash list.
	in, _ := signature.ReadSignatureDatabase(bytes.NewReader(db.Bytes()))
	t.Logf("input db: %d signature lists", len(in))
	for _, sl := range in {
		t.Logf("  input list type=%s entries=%d", sl.SignatureType.Format(), len(sl.Signatures))
	}

	out := t.TempDir()
	err := GenerateKeySet(Options{
		Name:               "f3",
		OutputDir:          out,
		SkipMicrosoftCerts: true,
		CustomCertDir:      fwDir,
	})
	t.Logf("GenerateKeySet err = %v", err)
	if err != nil {
		t.Fatalf("unexpected error")
	}

	b, err := os.ReadFile(filepath.Join(out, "db.esl"))
	if err != nil {
		t.Fatal(err)
	}
	sigdb, err := signature.ReadSignatureDatabase(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	foundCert, foundHash := false, false
	for _, sl := range sigdb {
		t.Logf("  output db.esl list type=%s entries=%d", sl.SignatureType.Format(), len(sl.Signatures))
		for _, s := range sl.Signatures {
			if bytes.Equal(s.Data, dbCert) {
				foundCert = true
			}
			if bytes.Equal(s.Data, hash[:]) {
				foundHash = true
			}
		}
	}
	t.Logf("custom cert carried over: %v", foundCert)
	t.Logf("custom SHA256 hash carried over: %v", foundHash)
	if !foundHash {
		t.Errorf("SHA256 entry from firmware db is missing from db.esl, yet GenerateKeySet returned nil")
	}
}
