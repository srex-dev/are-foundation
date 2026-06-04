package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SetOPA configures optional OPA Rego evaluation for handleEvaluatePolicy (iter-007).
// baseURL is the OPA base (e.g. http://localhost:8181), same as s0s1-rest-bff ARE_S0S1_OPA_URL.
// When empty, EvaluatePolicy uses the in-process stub only.
// client should include an appropriate Timeout; if nil, a 5s default is used.
func (s *StaticUpstreamClient) SetOPA(baseURL string, client *http.Client) {
	if s == nil {
		return
	}
	s.opaBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if s.opaBaseURL == "" {
		s.opaHTTP = nil
		return
	}
	if client != nil {
		s.opaHTTP = client
	} else {
		s.opaHTTP = &http.Client{Timeout: 5 * time.Second}
	}
}

func (s *StaticUpstreamClient) opaHTTPClient() *http.Client {
	if s != nil && s.opaHTTP != nil {
		return s.opaHTTP
	}
	return &http.Client{Timeout: 5 * time.Second}
}

// evaluatePolicyOPA calls POST {base}/v1/data/are/evaluatepolicy/decision with the same input envelope as the BFF.
func evaluatePolicyOPA(
	ctx context.Context,
	base string,
	hc *http.Client,
	decisionID, agentID, actionClass, resource string,
) (effect string, reason string, err error) {
	u, err := url.Parse(base + "/v1/data/are/evaluatepolicy/decision")
	if err != nil {
		return "", "", err
	}
	body, err := json.Marshal(map[string]any{
		"input": map[string]any{
			"decision_id":  decisionID,
			"agent_id":     agentID,
			"action_class": actionClass,
			"resource":     resource,
		},
	})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("opa: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var envelope struct {
		Result *struct {
			Effect string `json:"effect"`
			Reason string `json:"reason"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", "", err
	}
	if envelope.Result == nil || strings.TrimSpace(envelope.Result.Effect) == "" {
		return "", "", fmt.Errorf("opa: missing result.effect")
	}
	return strings.ToUpper(strings.TrimSpace(envelope.Result.Effect)), envelope.Result.Reason, nil
}
