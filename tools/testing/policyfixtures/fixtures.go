// Package policyfixtures exposes shared EvaluatePolicy stub test vectors for the
// API gateway StaticUpstreamClient and the s0s1-rest-bff policy Stub (iter-002).
package policyfixtures

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed vectors.json
var vectorsJSON []byte

var (
	loadOnce sync.Once
	doc      stubDoc
	loadErr  error
)

type stubDoc struct {
	Version int      `json:"version"`
	Comment string   `json:"comment"`
	Vectors []Vector `json:"vectors"`
}

// Vector is one POST /v1/policy/evaluations scenario for the in-process stub.
type Vector struct {
	Name                    string `json:"name"`
	DecisionID              string `json:"decision_id"`
	AgentID                 string `json:"agent_id"`
	ActionClass             string `json:"action_class"`
	Resource                string `json:"resource"`
	WantHTTPStatus          int    `json:"want_http_status"`
	WantEffect              string `json:"want_effect,omitempty"`
	WantReason              string `json:"want_reason,omitempty"`
	WantReasonSubstring     string `json:"want_reason_substring,omitempty"`
	WantErrorCode           string `json:"want_error_code,omitempty"`
	ExpectDependencyTimeout bool   `json:"expect_dependency_timeout"`
}

func load() {
	loadOnce.Do(func() {
		loadErr = json.Unmarshal(vectorsJSON, &doc)
		if loadErr != nil {
			return
		}
		if doc.Version != 1 {
			loadErr = fmt.Errorf("policyfixtures: unsupported version %d", doc.Version)
		}
	})
}

// StubVectors returns all stub vectors (for table-driven tests).
func StubVectors() ([]Vector, error) {
	load()
	if loadErr != nil {
		return nil, loadErr
	}
	out := make([]Vector, len(doc.Vectors))
	copy(out, doc.Vectors)
	return out, nil
}

// ByName returns a vector by its stable name, if present.
func ByName(name string) (Vector, bool) {
	load()
	if loadErr != nil {
		return Vector{}, false
	}
	for i := range doc.Vectors {
		if doc.Vectors[i].Name == name {
			return doc.Vectors[i], true
		}
	}
	return Vector{}, false
}

// Must is for tests: panics if name is missing or fixtures failed to load.
func Must(name string) Vector {
	load()
	if loadErr != nil {
		panic(loadErr)
	}
	v, ok := ByName(name)
	if !ok {
		panic("policyfixtures: missing vector " + name)
	}
	return v
}

// PolicyEvaluationBody builds the JSON body for POST /v1/policy/evaluations.
// If agentID is empty, the vector's AgentID is used.
func (v Vector) PolicyEvaluationBody(agentID string) ([]byte, error) {
	aid := agentID
	if aid == "" {
		aid = v.AgentID
	}
	return json.Marshal(map[string]string{
		"decision_id":  v.DecisionID,
		"agent_id":     aid,
		"action_class": v.ActionClass,
		"resource":     v.Resource,
	})
}
