package api

import "testing"

func TestAgentRecFromRegisterResponseBody(t *testing.T) {
	t.Parallel()
	body := []byte(`{"agent":{"agent_id":"a-1","agent_type":"T","owner_id":"O","status":"ACTIVE","metadata":{"k":"v"}}}`)
	rec, err := agentRecFromRegisterResponseBody(body, "fallbackT", "fallbackO")
	if err != nil {
		t.Fatal(err)
	}
	if rec.AgentType != "T" || rec.OwnerID != "O" || rec.Status != "ACTIVE" {
		t.Fatalf("rec: %+v", rec)
	}
	if rec.Metadata == nil || rec.Metadata["k"] != "v" {
		t.Fatalf("metadata: %v", rec.Metadata)
	}
	empty, err := agentRecFromRegisterResponseBody([]byte(`{}`), "ft", "fo")
	if err != nil {
		t.Fatal(err)
	}
	if empty.AgentType != "ft" || empty.OwnerID != "fo" {
		t.Fatalf("fallback: %+v", empty)
	}
}
