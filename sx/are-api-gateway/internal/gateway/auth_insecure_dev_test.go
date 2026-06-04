//go:build dev

package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
)

func TestStaticValidatorInsecureTestTokenFlag(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	on := NewStaticValidatorWithInsecureTestToken(&privateKey.PublicKey, true)
	id, err := on.Validate(context.Background(), "test-token")
	if err != nil || id != "test-subject" {
		t.Fatalf("got %q err=%v", id, err)
	}
	off := NewStaticValidatorWithInsecureTestToken(&privateKey.PublicKey, false)
	if _, err := off.Validate(context.Background(), "test-token"); err == nil {
		t.Fatal("expected error when insecure test-token disabled")
	}
}
