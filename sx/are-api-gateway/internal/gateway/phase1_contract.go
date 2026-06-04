package gateway

import "strings"

type routeSpec struct {
	Service       string
	Method        string
	SuccessStatus int
}

func matchPhase1Route(httpMethod string, path string) (routeSpec, bool) {
	switch {
	case httpMethod == "POST" && path == "/v1/identity/agents":
		return routeSpec{Service: "ARE-A-S0-001", Method: "RegisterAgent", SuccessStatus: 201}, true
	case httpMethod == "GET" && strings.HasPrefix(path, "/v1/identity/agents/"):
		return routeSpec{Service: "ARE-A-S0-001", Method: "GetAgent", SuccessStatus: 200}, true
	case httpMethod == "POST" && path == "/v1/passports":
		return routeSpec{Service: "ARE-A-S0-005", Method: "IssuePassport", SuccessStatus: 201}, true
	case httpMethod == "GET" && strings.HasPrefix(path, "/v1/passports/by-agent/"):
		return routeSpec{Service: "ARE-A-S0-005", Method: "ListPassportsByAgent", SuccessStatus: 200}, true
	case httpMethod == "POST" && path == "/v1/passports:verify":
		return routeSpec{Service: "ARE-A-S0-005", Method: "VerifyPassport", SuccessStatus: 200}, true
	case httpMethod == "POST" && path == "/v1/enforcement/scope:evaluate":
		return routeSpec{Service: "ARE-A-S1-002", Method: "EvaluateScope", SuccessStatus: 200}, true
	case httpMethod == "POST" && path == "/v1/policy/evaluations":
		return routeSpec{Service: "ARE-A-S1-001", Method: "EvaluatePolicy", SuccessStatus: 200}, true
	case httpMethod == "GET" && path == "/v1/meta/deployment":
		return routeSpec{Service: "ARE-A-S0-META", Method: "GetDeploymentMeta", SuccessStatus: 200}, true
	default:
		return routeSpec{}, false
	}
}

func isFoundationAPIRoute(httpMethod string, path string) bool {
	_, ok := matchPhase1Route(httpMethod, path)
	return ok
}

func requiresIdempotencyKey(httpMethod string, path string) bool {
	if httpMethod != "POST" {
		return false
	}
	switch path {
	case "/v1/identity/agents", "/v1/passports", "/v1/policy/evaluations", "/v1/enforcement/scope:evaluate", "/v1/passports:verify":
		return true
	default:
		return false
	}
}

// IsS0S1RESTProxyRoute reports whether the request maps to the foundation S0/S1 REST BFF.
func IsS0S1RESTProxyRoute(httpMethod, path string) bool {
	return isFoundationAPIRoute(httpMethod, path)
}
