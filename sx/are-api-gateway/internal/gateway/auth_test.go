package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestStaticValidatorValidToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "caller-x",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	validator := NewStaticValidator(&privateKey.PublicKey)
	identity, err := validator.Validate(context.Background(), signed)
	if err != nil {
		t.Fatal(err)
	}
	if identity != "caller-x" {
		t.Fatalf("unexpected identity: %s", identity)
	}
}

func TestJWKSValidatorValidToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := computeKID(t, &privateKey.PublicKey)
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
	defer server.Close()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "caller-jwks",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = kid
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	validator, err := NewJWKSValidator(context.Background(), server.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := validator.Validate(context.Background(), signed)
	if err != nil {
		t.Fatal(err)
	}
	if identity != "caller-jwks" {
		t.Fatalf("unexpected identity: %s", identity)
	}
}

func TestJWKSValidatorRequiresURL(t *testing.T) {
	if _, err := NewJWKSValidator(context.Background(), "", false); err == nil {
		t.Fatal("expected url error")
	}
}

func TestJWKSValidatorInvalidEndpoint(t *testing.T) {
	validator, err := NewJWKSValidator(context.Background(), "http://127.0.0.1:1/jwks", false)
	if err != nil {
		return
	}
	if _, err := validator.Validate(context.Background(), "invalid-token"); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestStaticValidatorWrongSigningMethod(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "caller-x",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	validator := NewStaticValidator(&privateKey.PublicKey)
	if _, err := validator.Validate(context.Background(), signed); err == nil {
		t.Fatal("expected signing method error")
	}
}

func TestStaticValidatorMissingSubject(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	validator := NewStaticValidator(&privateKey.PublicKey)
	if _, err := validator.Validate(context.Background(), signed); err == nil {
		t.Fatal("expected missing subject error")
	}
}

func computeKID(t *testing.T, key *rsa.PublicKey) string {
	t.Helper()
	der := x509.MarshalPKCS1PublicKey(key)
	hashed := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(hashed[:])
}
