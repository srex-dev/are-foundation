package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	out := filepath.Join("deploy", "compose", "tls")
	must(os.MkdirAll(out, 0o700))

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ARE Foundation Local CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	must(err)

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "s0s1-rest-bff"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(2, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost", "are-api-gateway", "s0s1-rest-bff"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	must(err)

	writeCert(filepath.Join(out, "ca.crt"), caDER)
	writeKey(filepath.Join(out, "ca.key"), caKey)
	writeCert(filepath.Join(out, "foundation.crt"), leafDER)
	writeKey(filepath.Join(out, "foundation.key"), leafKey)
}

func writeCert(path string, der []byte) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	must(err)
	defer f.Close()
	must(pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func writeKey(path string, key *rsa.PrivateKey) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	must(err)
	defer f.Close()
	must(pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
