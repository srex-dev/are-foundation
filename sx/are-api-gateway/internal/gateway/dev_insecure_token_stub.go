//go:build !dev

package gateway

// DevBuildSupportsInsecureTestToken is false unless the binary is built with -tags dev.
func DevBuildSupportsInsecureTestToken() bool { return false }

func insecureBypassTestTokenJWKS(v *JWKSValidator, token string) (string, bool) {
	_ = v
	_ = token
	return "", false
}

func insecureBypassTestTokenStatic(v *StaticValidator, token string) (string, bool) {
	_ = v
	_ = token
	return "", false
}
