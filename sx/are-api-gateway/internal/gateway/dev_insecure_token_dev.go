//go:build dev

package gateway

// DevBuildSupportsInsecureTestToken is true when built with -tags dev (local/CI tests only).
func DevBuildSupportsInsecureTestToken() bool { return true }

func insecureBypassTestTokenJWKS(v *JWKSValidator, token string) (string, bool) {
	if v.allowInsecureTestToken && token == "test-token" {
		return "test-subject", true
	}
	return "", false
}

func insecureBypassTestTokenStatic(v *StaticValidator, token string) (string, bool) {
	if v.allowInsecureTestToken && token == "test-token" {
		return "test-subject", true
	}
	return "", false
}
