package domain

import (
	"fmt"
	"testing"
)

func TestValidateAgentTypeValidValues(t *testing.T) {
	got, err := ValidateAgentType(" autonomous ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "AUTONOMOUS" {
		t.Fatalf("expected AUTONOMOUS, got %s", got)
	}
}

func TestValidateAgentTypeInvalid(t *testing.T) {
	_, err := ValidateAgentType("unknown")
	if err == nil {
		t.Fatal("expected invalid type error")
	}
}

func TestValidateAgentMetadataWithinLimits(t *testing.T) {
	meta := map[string]string{"k": "v"}
	if err := ValidateAgentMetadata(meta); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateAgentMetadataTooManyKeys(t *testing.T) {
	meta := map[string]string{}
	for i := 0; i < 51; i++ {
		meta[fmt.Sprintf("meta-key-%02d", i)] = "x"
	}
	if err := ValidateAgentMetadata(meta); err == nil {
		t.Fatal("expected error for too many keys")
	}
}
