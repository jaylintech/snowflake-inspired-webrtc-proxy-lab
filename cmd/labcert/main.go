package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type certificateOptions struct {
	OutputDirectory string
	DNSNames        []string
	IPAddresses     []net.IP
	ValidFor        time.Duration
	Force           bool
}

func main() {
	outputDirectory := flag.String("out", "testbed/private/turn", "output directory for ignored lab certificate files")
	dnsNames := flag.String("dns", "turn.lab.example", "comma-separated DNS names for the TURN server certificate")
	ipAddresses := flag.String("ip", "127.0.0.1", "comma-separated IP addresses for the TURN server certificate")
	validFor := flag.Duration("valid-for", 7*24*time.Hour, "certificate validity duration")
	force := flag.Bool("force", false, "overwrite existing generated files")
	flag.Parse()

	options, err := parseOptions(*outputDirectory, *dnsNames, *ipAddresses, *validFor, *force)
	if err != nil {
		log.Fatal(err)
	}
	if err := generateCertificates(options, time.Now().UTC()); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote temporary TURN CA and server certificate under %s", options.OutputDirectory)
	log.Printf("trust ca-cert.pem only in the owned lab, then remove that trust after testing")
	if runtime.GOOS == "windows" {
		log.Printf("Windows does not enforce POSIX mode bits; restrict the output directory ACL to the current lab user")
	}
}

func parseOptions(outputDirectory, dnsCSV, ipCSV string, validFor time.Duration, force bool) (certificateOptions, error) {
	if strings.TrimSpace(outputDirectory) == "" {
		return certificateOptions{}, fmt.Errorf("output directory is required")
	}
	if validFor < time.Hour || validFor > 30*24*time.Hour {
		return certificateOptions{}, fmt.Errorf("valid-for must be between 1 hour and 30 days")
	}

	options := certificateOptions{OutputDirectory: outputDirectory, ValidFor: validFor, Force: force}
	for _, name := range splitCSV(dnsCSV) {
		if strings.ContainsAny(name, "/\\") {
			return certificateOptions{}, fmt.Errorf("invalid DNS name %q", name)
		}
		options.DNSNames = append(options.DNSNames, name)
	}
	for _, raw := range splitCSV(ipCSV) {
		parsed := net.ParseIP(raw)
		if parsed == nil {
			return certificateOptions{}, fmt.Errorf("invalid IP address %q", raw)
		}
		options.IPAddresses = append(options.IPAddresses, parsed)
	}
	if len(options.DNSNames) == 0 && len(options.IPAddresses) == 0 {
		return certificateOptions{}, fmt.Errorf("at least one DNS name or IP address is required")
	}
	return options, nil
}

func generateCertificates(options certificateOptions, now time.Time) error {
	caKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return fmt.Errorf("generate server key: %w", err)
	}

	caSerial, err := randomSerial()
	if err != nil {
		return err
	}
	serverSerial, err := randomSerial()
	if err != nil {
		return err
	}

	notBefore := now.Add(-5 * time.Minute)
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "Snowflake Part 2 Temporary Lab CA"},
		NotBefore:             notBefore,
		NotAfter:              now.Add(options.ValidFor),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create CA certificate: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: "Snowflake Part 2 TURN"},
		NotBefore:    notBefore,
		NotAfter:     now.Add(options.ValidFor),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     options.DNSNames,
		IPAddresses:  options.IPAddresses,
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create server certificate: %w", err)
	}

	files := map[string]struct {
		mode os.FileMode
		data []byte
	}{
		"ca-cert.pem":   {mode: 0o644, data: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})},
		"turn-cert.pem": {mode: 0o644, data: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})},
		"turn-key.pem":  {mode: 0o600, data: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})},
	}

	if err := os.MkdirAll(options.OutputDirectory, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	for name := range files {
		path := filepath.Join(options.OutputDirectory, name)
		if !options.Force {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("refusing to overwrite %s; use -force for a new lab set", path)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}
	for name, file := range files {
		path := filepath.Join(options.OutputDirectory, name)
		if err := os.WriteFile(path, file.data, file.mode); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		if err := os.Chmod(path, file.mode); err != nil {
			return fmt.Errorf("set permissions on %s: %w", path, err)
		}
	}
	return nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	return serial, nil
}

func splitCSV(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}
