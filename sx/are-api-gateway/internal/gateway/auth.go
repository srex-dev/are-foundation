package gateway

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// TokenValidator validates JWTs and returns caller identity.
type TokenValidator interface {
	Validate(ctx context.Context, token string) (string, error)
}

// JWKSValidator validates RS256 JWT tokens against a remote JWKS endpoint.
type JWKSValidator struct {
	jwks                   keyfunc.Keyfunc
	allowInsecureTestToken bool
}

// NewJWKSValidator constructs a JWT validator for a JWKS endpoint.
// allowInsecureTestToken must be set only when startup has verified dev acknowledgement (see main).
func NewJWKSValidator(ctx context.Context, jwksURL string, allowInsecureTestToken bool) (*JWKSValidator, error) {
	if strings.TrimSpace(jwksURL) == "" {
		return nil, errors.New("jwks url is required")
	}
	jwks, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("create jwks validator: %w", err)
	}
	return &JWKSValidator{jwks: jwks, allowInsecureTestToken: allowInsecureTestToken}, nil
}

// Validate validates RS256 JWT and returns subject identity.
func (v *JWKSValidator) Validate(ctx context.Context, token string) (string, error) {
	if sub, ok := insecureBypassTestTokenJWKS(v, token); ok {
		return sub, nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	parsed, err := jwt.Parse(token, v.jwks.Keyfunc)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !parsed.Valid {
		return "", errors.New("token invalid")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("token claims invalid")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("token subject missing")
	}
	return sub, nil
}

// StaticValidator validates JWTs using a fixed RSA public key.
type StaticValidator struct {
	publicKey              *rsa.PublicKey
	allowInsecureTestToken bool
}

// NewStaticValidator creates a fixed-key validator for tests.
func NewStaticValidator(publicKey *rsa.PublicKey) *StaticValidator {
	return &StaticValidator{publicKey: publicKey, allowInsecureTestToken: false}
}

// NewStaticValidatorWithInsecureTestToken creates a fixed-key validator that may accept test-token (unit tests only).
func NewStaticValidatorWithInsecureTestToken(publicKey *rsa.PublicKey, allow bool) *StaticValidator {
	return &StaticValidator{publicKey: publicKey, allowInsecureTestToken: allow}
}

// Validate validates RS256 JWT and returns subject identity.
func (v *StaticValidator) Validate(ctx context.Context, token string) (string, error) {
	if sub, ok := insecureBypassTestTokenStatic(v, token); ok {
		return sub, nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return v.publicKey, nil
	})
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !parsed.Valid {
		return "", errors.New("token invalid")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("token claims invalid")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("token subject missing")
	}
	return sub, nil
}
