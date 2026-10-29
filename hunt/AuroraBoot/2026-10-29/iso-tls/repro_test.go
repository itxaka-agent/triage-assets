package isoserve

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func probe(t *testing.T, s *Server) {
	addr := s.Addr().String()
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Logf("TCP dial: refused/failed: %v", err)
		return
	}
	t.Logf("TCP dial to %s: CONNECTED (listener still open)", addr)
	c.SetDeadline(time.Now().Add(3 * time.Second))
	tc := tls.Client(c, &tls.Config{InsecureSkipVerify: true})
	start := time.Now()
	err = tc.Handshake()
	t.Logf("TLS handshake after %v: err=%v", time.Since(start).Round(time.Millisecond), err)
	c2, _ := net.DialTimeout("tcp", addr, 2*time.Second)
	c2.SetDeadline(time.Now().Add(3 * time.Second))
	c2.Write([]byte("GET /redfish/iso/x/y HTTP/1.0\r\n\r\n"))
	buf := make([]byte, 64)
	start = time.Now()
	n, err := c2.Read(buf)
	t.Logf("plain HTTP read after %v: n=%d err=%v", time.Since(start).Round(time.Millisecond), n, err)
	c2.Close()
	c.Close()
}

func TestMissingCert(t *testing.T) {
	s := New(Config{BaseURL: "https://127.0.0.1", BindAddr: "127.0.0.1:0", CertFile: "/nonexistent/cert.pem", KeyFile: "/nonexistent/key.pem"})
	err := s.Start(context.Background())
	t.Logf("Start() with missing cert/key returned: %v", err)
	time.Sleep(200 * time.Millisecond)
	probe(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	t.Logf("Shutdown: %v", s.Shutdown(ctx))
}

func TestGarbageCert(t *testing.T) {
	d := t.TempDir()
	cf, kf := filepath.Join(d, "c.pem"), filepath.Join(d, "k.pem")
	os.WriteFile(cf, []byte("not a cert"), 0o600)
	os.WriteFile(kf, []byte("not a key"), 0o600)
	s := New(Config{BaseURL: "https://127.0.0.1", BindAddr: "127.0.0.1:0", CertFile: cf, KeyFile: kf})
	err := s.Start(context.Background())
	t.Logf("Start() with garbage cert/key returned: %v", err)
	time.Sleep(200 * time.Millisecond)
	probe(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	t.Logf("Shutdown: %v", s.Shutdown(ctx))
}
