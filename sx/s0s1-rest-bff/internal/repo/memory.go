package repo

import (
	"context"
	"sync"
)

// Memory is an in-process Repository (tests and no-database compose).
type Memory struct {
	mu           sync.Mutex
	agents       map[string]AgentRec
	registerIdem map[string][]byte
	passportIdem map[string][]byte
	passports    map[string][]byte
	byAgent      map[string][]string
}

func NewMemory() *Memory {
	return &Memory{
		agents:       make(map[string]AgentRec),
		registerIdem: make(map[string][]byte),
		passportIdem: make(map[string][]byte),
		passports:    make(map[string][]byte),
		byAgent:      make(map[string][]string),
	}
}

func (m *Memory) GetRegisterIdem(_ context.Context, idemKey string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.registerIdem[idemKey]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), b...), true, nil
}

func (m *Memory) FinishRegister(_ context.Context, idemKey, agentID string, responseBody []byte, rec AgentRec) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if prev, ok := m.registerIdem[idemKey]; ok {
		return append([]byte(nil), prev...), nil
	}
	m.agents[agentID] = rec
	m.registerIdem[idemKey] = append([]byte(nil), responseBody...)
	return append([]byte(nil), responseBody...), nil
}

func (m *Memory) GetAgent(_ context.Context, agentID string) (AgentRec, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.agents[agentID]
	return rec, ok, nil
}

func (m *Memory) GetAdmissionEnvelope(_ context.Context, agentID string) (map[string]any, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.agents[agentID]
	if !ok {
		return nil, false, nil
	}
	if len(rec.AdmissionEnvelope) == 0 {
		return nil, false, nil
	}
	out := make(map[string]any, len(rec.AdmissionEnvelope))
	for k, v := range rec.AdmissionEnvelope {
		out[k] = v
	}
	return out, true, nil
}

func (m *Memory) GetPassportIdem(_ context.Context, idemKey string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.passportIdem[idemKey]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), b...), true, nil
}

func (m *Memory) FinishPassport(_ context.Context, idemKey, agentID string, responseBody []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if prev, ok := m.passportIdem[idemKey]; ok {
		return append([]byte(nil), prev...), nil
	}
	if _, ok := m.agents[agentID]; !ok {
		return nil, ErrAgentNotFound
	}
	m.passportIdem[idemKey] = append([]byte(nil), responseBody...)
	if passportID := passportIDFromBody(responseBody); passportID != "" {
		m.passports[passportID] = append([]byte(nil), responseBody...)
		m.byAgent[agentID] = append(m.byAgent[agentID], passportID)
	}
	return append([]byte(nil), responseBody...), nil
}

func (m *Memory) ListPassportBodiesByAgent(_ context.Context, agentID string) ([][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := append([]string(nil), m.byAgent[agentID]...)
	out := make([][]byte, 0, len(ids))
	for _, id := range ids {
		if body, ok := m.passports[id]; ok {
			out = append(out, append([]byte(nil), body...))
		}
	}
	return out, nil
}

func (m *Memory) GetPassportBody(_ context.Context, passportID string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.passports[passportID]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), body...), true, nil
}
