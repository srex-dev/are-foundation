package domain

import (
	"fmt"
	"strings"
)

const (
	maxAgentMetadataKeys   = 50
	maxAgentMetadataKeyLen = 64
	maxAgentMetadataValLen = 512
)

// ValidateAgentMetadata enforces bounded map size and key/value lengths for registration.
func ValidateAgentMetadata(metadata map[string]string) error {
	if len(metadata) == 0 {
		return nil
	}
	if len(metadata) > maxAgentMetadataKeys {
		return fmt.Errorf("metadata: at most %d keys allowed, got %d", maxAgentMetadataKeys, len(metadata))
	}
	for k, v := range metadata {
		if len(k) > maxAgentMetadataKeyLen {
			return fmt.Errorf("metadata key length %d exceeds max %d", len(k), maxAgentMetadataKeyLen)
		}
		if len(v) > maxAgentMetadataValLen {
			return fmt.Errorf("metadata value length for key %q exceeds max %d", k, maxAgentMetadataValLen)
		}
	}
	return nil
}

var validTypes = map[string]bool{
	"AUTONOMOUS": true,
	"ASSISTED":   true,
	"SYSTEM":     true,
	"EPHEMERAL":  true,
}

// ValidateAgentType validates and normalizes user-provided type.
func ValidateAgentType(v string) (string, error) {
	t := strings.ToUpper(strings.TrimSpace(v))
	if !validTypes[t] {
		return "", fmt.Errorf("invalid agent_type %q; valid: AUTONOMOUS|ASSISTED|SYSTEM|EPHEMERAL", v)
	}
	return t, nil
}
