package policyfixtures

import (
	"strings"
	"testing"
)

func TestMustKnownVectors(t *testing.T) {
	for _, name := range []string{
		"demo_forbidden_deny",
		"demo_read_allow",
		"dependency_timeout",
		"contract_shape_generic_read",
	} {
		v := Must(name)
		if v.Name != name {
			t.Fatalf("%s: name field mismatch", name)
		}
	}
}

func TestPolicyEvaluationBodyOverrideAgent(t *testing.T) {
	v := Must("demo_forbidden_deny")
	b, err := v.PolicyEvaluationBody("agt-override")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"agent_id":"agt-override"`) {
		t.Fatalf("body: %s", b)
	}
}

func TestStubVectorsNonEmpty(t *testing.T) {
	vs, err := StubVectors()
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) < 4 {
		t.Fatalf("expected at least 4 vectors, got %d", len(vs))
	}
}
