package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunRequiresTLSFilesAndJWKS(t *testing.T) {
	err := run(context.Background(), Config{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for missing tls config")
	}
}

func TestRunRejectsMissingCertFiles(t *testing.T) {
	cfg := Config{
		CertFile: "missing-cert.pem",
		KeyFile:  "missing-key.pem",
		CAFile:   "missing-ca.pem",
		JWKSURL:  "https://example.test/jwks.json",
	}
	err := run(context.Background(), cfg, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for missing cert files")
	}
}

func TestLoadConfigFallbacks(t *testing.T) {
	t.Setenv("ARE_GW_HTTPS_PORT", "not-int")
	t.Setenv("ARE_GW_GRPC_PORT", "not-int")
	cfg := loadConfig()
	if cfg.HTTPSPort != 443 || cfg.GRPCPort != 9100 {
		t.Fatalf("unexpected fallback ports: %+v", cfg)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("ARE_GW_HTTPS_PORT", "9443")
	t.Setenv("ARE_GW_GRPC_PORT", "19100")
	t.Setenv("ARE_GW_METRICS_PORT", "18090")
	t.Setenv("ARE_GW_HEALTH_PORT", "18080")
	t.Setenv("ARE_GW_TLS_CERT_FILE", "cert.pem")
	t.Setenv("ARE_GW_TLS_KEY_FILE", "key.pem")
	t.Setenv("ARE_GW_TLS_CA_FILE", "ca.pem")
	t.Setenv("ARE_GW_JWKS_URL", "https://issuer.example/jwks.json")
	cfg := loadConfig()
	if cfg.HTTPSPort != 9443 || cfg.GRPCPort != 19100 || cfg.MetricsPort != 18090 || cfg.HealthPort != 18080 {
		t.Fatalf("unexpected env ports: %+v", cfg)
	}
	if cfg.JWKSURL == "" {
		t.Fatal("expected jwks url from env")
	}
}

func TestEnvIntValidValue(t *testing.T) {
	t.Setenv("ARE_GW_HEALTH_PORT", "18081")
	if got := envInt("ARE_GW_HEALTH_PORT", 8080); got != 18081 {
		t.Fatalf("expected 18081, got %d", got)
	}
}

func TestNewTLSConfigSuccess(t *testing.T) {
	certPath, keyPath, caPath := writeTLSFiles(t)
	cfg, err := newTLSConfig(certPath, keyPath, caPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinVersion != 0x0304 {
		t.Fatalf("expected TLS1.3, got %v", cfg.MinVersion)
	}
}

func TestNewTLSConfigInvalidCA(t *testing.T) {
	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "cert.pem")
	keyPath := filepath.Join(tmp, "key.pem")
	caPath := filepath.Join(tmp, "ca.pem")
	if err := os.WriteFile(certPath, []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caPath, []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := newTLSConfig(certPath, keyPath, caPath)
	if err == nil {
		t.Fatal("expected newTLSConfig error")
	}
}

func TestRunReturnsOnContextCancel(t *testing.T) {
	certPath, keyPath, caPath := writeTLSFiles(t)
	jwksURL := startJWKSServer(t)

	cfg := Config{
		HTTPSPort:    0,
		GRPCPort:     0,
		HealthPort:   0,
		MetricsPort:  0,
		CertFile:     certPath,
		KeyFile:      keyPath,
		CAFile:       caPath,
		JWKSURL:      jwksURL,
		PollInterval: 10 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	err := run(ctx, cfg, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
}

func TestRunRequiresJWKSURL(t *testing.T) {
	certPath, keyPath, caPath := writeTLSFiles(t)
	cfg := Config{
		HTTPSPort:    0,
		GRPCPort:     0,
		HealthPort:   0,
		MetricsPort:  0,
		CertFile:     certPath,
		KeyFile:      keyPath,
		CAFile:       caPath,
		PollInterval: 10 * time.Millisecond,
	}
	err := run(context.Background(), cfg, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected jwks url error")
	}
}

func TestRunRejectsInsecureConformanceWithoutDevUnsafeMode(t *testing.T) {
	certPath, keyPath, caPath := writeTLSFiles(t)
	jwksURL := startJWKSServer(t)
	cfg := Config{
		HTTPSPort:                 0,
		GRPCPort:                  0,
		HealthPort:                0,
		MetricsPort:               0,
		CertFile:                  certPath,
		KeyFile:                   keyPath,
		CAFile:                    caPath,
		JWKSURL:                   jwksURL,
		EnableInsecureConformance: true,
		PollInterval:              10 * time.Millisecond,
	}
	t.Setenv("ARE_GW_DEV_UNSAFE_MODE", "")
	err := run(context.Background(), cfg, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected insecure conformance without ARE_GW_DEV_UNSAFE_MODE to fail")
	}
}

func TestRunRejectsLiveUpstreamWithoutMandatoryEnv(t *testing.T) {
	t.Setenv("ARE_GW_UPSTREAM_MODE", "live")
	t.Setenv("ARE_GW_S0S1_HTTP_PROXY_BASE", "")
	certPath, keyPath, caPath := writeTLSFiles(t)
	jwksURL := startJWKSServer(t)
	cfg := Config{
		HTTPSPort:    0,
		GRPCPort:     0,
		HealthPort:   0,
		MetricsPort:  0,
		CertFile:     certPath,
		KeyFile:      keyPath,
		CAFile:       caPath,
		JWKSURL:      jwksURL,
		PollInterval: 10 * time.Millisecond,
	}
	err := run(context.Background(), cfg, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected ARE_GW_UPSTREAM_MODE=live validation error before listeners")
	}
}

func TestRunFailsOnInvalidGRPCPort(t *testing.T) {
	certPath, keyPath, caPath := writeTLSFiles(t)
	jwksURL := startJWKSServer(t)
	cfg := Config{
		HTTPSPort:    0,
		GRPCPort:     -1,
		HealthPort:   0,
		MetricsPort:  0,
		CertFile:     certPath,
		KeyFile:      keyPath,
		CAFile:       caPath,
		JWKSURL:      jwksURL,
		PollInterval: 10 * time.Millisecond,
	}
	err := run(context.Background(), cfg, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected grpc listen error")
	}
}

func writeTLSFiles(t *testing.T) (string, string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "are-api-gateway",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "cert.pem")
	keyPath := filepath.Join(tmp, "key.pem")
	caPath := filepath.Join(tmp, "ca.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath, caPath
}

func startJWKSServer(t *testing.T) string {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PublicKey(&privateKey.PublicKey)
	sum := sha256.Sum256(der)
	kid := base64.RawURLEncoding.EncodeToString(sum[:])
	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": kid,
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(server.Close)
	return server.URL
}
