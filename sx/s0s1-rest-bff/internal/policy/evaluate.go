package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Evaluator decides policy effect for EvaluatePolicy REST payloads.
type Evaluator interface {
	Evaluate(ctx context.Context, decisionID, agentID, actionClass, resource string) (effect string, reason string, err error)
}

// Stub matches sx/are-api-gateway StaticUpstreamClient.handleEvaluatePolicy (no OPA).
type Stub struct{}

func (Stub) Evaluate(_ context.Context, _, _, actionClass, resource string) (effect string, reason string, err error) {
	if strings.Contains(strings.ToLower(resource), "timeout") {
		return "", "", ErrDependencyTimeout
	}
	effect = "ALLOW"
	reason = "policy-evaluated"
	switch actionClass {
	case "demo.forbidden_action":
		effect = "DENY"
		reason = "forbidden action class for demo"
	case "demo.read":
		effect = "ALLOW"
		reason = "read scope allowed"
	default:
		if strings.Contains(strings.ToUpper(actionClass), "DENY") {
			effect = "DENY"
		}
	}
	return effect, reason, nil
}

// ErrDependencyTimeout signals the synthetic timeout resource (504 from API).
var ErrDependencyTimeout = errors.New("policy dependency timeout")

// OPAClient calls OPA's Data API: POST {base}/v1/data/are/evaluatepolicy/decision
// with body {"input":{...}}. Rego: policy/are_evaluatepolicy.rego.
type OPAClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c *OPAClient) Evaluate(ctx context.Context, decisionID, agentID, actionClass, resource string) (effect string, reason string, err error) {
	if strings.Contains(strings.ToLower(resource), "timeout") {
		return "", "", ErrDependencyTimeout
	}
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return "", "", fmt.Errorf("opa: empty base url")
	}
	u, err := url.Parse(base + "/v1/data/are/evaluatepolicy/decision")
	if err != nil {
		return "", "", fmt.Errorf("opa: parse url: %w", err)
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
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("opa: request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("opa: read body: %w", err)
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
		return "", "", fmt.Errorf("opa: decode: %w", err)
	}
	if envelope.Result == nil || strings.TrimSpace(envelope.Result.Effect) == "" {
		return "", "", fmt.Errorf("opa: missing result.effect")
	}
	return strings.ToUpper(strings.TrimSpace(envelope.Result.Effect)), envelope.Result.Reason, nil
}

// NewOPAClient returns an Evaluator backed by OPA. baseURL is e.g. http://opa:8181.
func NewOPAClient(baseURL string, timeout time.Duration) *OPAClient {
	t := timeout
	if t <= 0 {
		t = 5 * time.Second
	}
	return &OPAClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: t,
		},
	}
}
