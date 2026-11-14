package servertls

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
)

func TestNewConfig_GeneratesSelfSignedCertificate(t *testing.T) {
	config, err := NewConfig("", "")
	if err != nil {
		t.Fatalf("NewConfig() returned error: %v", err)
	}
	if config.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected minimum TLS version TLS 1.2, got %v", config.MinVersion)
	}
	if len(config.Certificates) != 1 {
		t.Fatalf("expected one certificate, got %d", len(config.Certificates))
	}

	certDER := config.Certificates[0].Certificate[0]
	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	if err := leaf.VerifyHostname("localhost"); err != nil {
		t.Fatalf("generated certificate does not cover localhost: %v", err)
	}
	if err := leaf.VerifyHostname("127.0.0.1"); err != nil {
		t.Fatalf("generated certificate does not cover loopback IP: %v", err)
	}
}

func TestNewConfig_RequiresCertificateAndKeyTogether(t *testing.T) {
	if _, err := NewConfig("cert.pem", ""); err == nil {
		t.Fatal("expected error when only a certificate path is supplied")
	}
	if _, err := NewConfig("", "key.pem"); err == nil {
		t.Fatal("expected error when only a key path is supplied")
	}
}
