package main

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

// 首次调用生成证书；再次调用沿用已有证书，不重新生成
func TestEnsureSelfSignedCertCreatesOnceAndReuses(t *testing.T) {
	dir := t.TempDir()
	cert, key := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")

	if err := ensureSelfSignedCert(cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := tls.LoadX509KeyPair(cert, key); err != nil {
		t.Fatalf("生成的证书无法加载: %v", err)
	}
	before, _ := os.ReadFile(cert)

	if err := ensureSelfSignedCert(cert, key); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(cert)
	if string(before) != string(after) {
		t.Fatal("已有证书不应被重新生成")
	}
}
