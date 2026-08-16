package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestGenerateCertificatesCreatesVerifiableTURNChain(t *testing.T) {
	directory := t.TempDir()
	options, err := parseOptions(directory, "turn.lab.example", "127.0.0.1", 24*time.Hour, false)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if err := generateCertificates(options, time.Now().UTC()); err != nil {
		t.Fatalf("generate certificates: %v", err)
	}

	ca := readCertificate(t, filepath.Join(directory, "ca-cert.pem"))
	server := readCertificate(t, filepath.Join(directory, "turn-cert.pem"))
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, err := server.Verify(x509.VerifyOptions{Roots: roots, DNSName: "turn.lab.example"}); err != nil {
		t.Fatalf("verify TURN certificate: %v", err)
	}
	if runtime.GOOS != "windows" {
		if mode := fileMode(t, filepath.Join(directory, "turn-key.pem")); mode.Perm() != 0o600 {
			t.Fatalf("private key mode = %o, want 600", mode.Perm())
		}
	}
}

func TestGenerateCertificatesRefusesOverwrite(t *testing.T) {
	directory := t.TempDir()
	options, err := parseOptions(directory, "turn.lab.example", "", time.Hour, false)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "turn-key.pem"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generateCertificates(options, time.Now().UTC()); err == nil {
		t.Fatal("expected overwrite refusal")
	}
}

func TestCleanCertificatesRemovesGeneratedFiles(t *testing.T) {
	directory := t.TempDir()
	options, err := parseOptions(directory, "turn.lab.example", "127.0.0.1", time.Hour, false)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if err := generateCertificates(options, time.Now().UTC()); err != nil {
		t.Fatalf("generate certificates: %v", err)
	}
	if err := cleanCertificates(directory); err != nil {
		t.Fatalf("clean certificates: %v", err)
	}
	for _, name := range []string{"ca-cert.pem", "turn-cert.pem", "turn-key.pem"} {
		path := filepath.Join(directory, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("file %s still exists after cleanCertificates", path)
		}
	}
}

func readCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatalf("no PEM block in %s", path)
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return certificate
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
