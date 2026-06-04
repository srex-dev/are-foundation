package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/domain"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/registryerr"
)

type dbPool interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Repository uses PostgreSQL as source of truth.
type Repository struct {
	pool dbPool
}

// New creates a repository using an existing pgx pool.
func New(pool dbPool) *Repository {
	return &Repository{pool: pool}
}

// RegisterAgentWithOutbox inserts agent, optional admission envelope, and outbox event atomically.
func (r *Repository) RegisterAgentWithOutbox(ctx context.Context, a domain.Agent, envelope *domain.AdmissionEnvelope, eventType string, payload []byte) (domain.Agent, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Agent{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metadataJSON, err := json.Marshal(a.Metadata)
	if err != nil {
		return domain.Agent{}, fmt.Errorf("marshal metadata: %w", err)
	}
	admissionJSON, err := json.Marshal(a.AdmissionConstraints)
	if err != nil {
		return domain.Agent{}, fmt.Errorf("marshal admission_constraints: %w", err)
	}
	var admittedPolicyID any
	var admittedPolicyVer any
	if a.AdmittedPolicyID != "" {
		admittedPolicyID = a.AdmittedPolicyID
	}
	if a.AdmittedPolicyVer != "" {
		admittedPolicyVer = a.AdmittedPolicyVer
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO agents (
			agent_id, agent_type, owner_id, external_id, status, metadata, registration_ts, updated_ts,
			admission_constraints, admitted_policy_id, admitted_policy_ver, admission_ts
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, a.AgentID, a.AgentType, a.OwnerID, nullable(a.ExternalID), a.Status, metadataJSON, a.RegistrationTS, a.UpdatedTS,
		admissionJSON, admittedPolicyID, admittedPolicyVer, a.AdmissionTS)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.Agent{}, fmt.Errorf(
				"%w: external_id + owner_id already registered",
				registryerr.ErrAlreadyExists,
			)
		}
		return domain.Agent{}, err
	}

	if envelope != nil {
		scopesJSON, err := json.Marshal(envelope.AdmittedScopes)
		if err != nil {
			return domain.Agent{}, fmt.Errorf("marshal admitted_scopes: %w", err)
		}
		capsJSON, err := json.Marshal(envelope.AdmittedBehavioralCaps)
		if err != nil {
			return domain.Agent{}, fmt.Errorf("marshal admitted_behavioral_caps: %w", err)
		}
		sig := envelope.Signature
		if sig == nil {
			sig = []byte{}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO admission_envelopes (
				envelope_id, agent_id, policy_id, policy_ver,
				admitted_scopes, admitted_behavioral_caps, admitted_ts, issuing_authority, signature
			) VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,$8,$9)
		`, envelope.EnvelopeID, envelope.AgentID, nullable(envelope.PolicyID), nullable(envelope.PolicyVer),
			scopesJSON, capsJSON, envelope.AdmittedTS, envelope.IssuingAuthority, sig)
		if err != nil {
			return domain.Agent{}, err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO agent_status_history (agent_id, previous_status, new_status, reason, changed_by)
		VALUES ($1,$2,$3,$4,$5)
	`, a.AgentID, nil, a.Status, "initial registration", "system")
	if err != nil {
		return domain.Agent{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO agent_lifecycle_outbox (agent_id, event_type, payload, status)
		VALUES ($1,$2,$3,'PENDING')
	`, a.AgentID, eventType, payload)
	if err != nil {
		return domain.Agent{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Agent{}, err
	}
	return a, nil
}

// GetAgent returns one row by id.
func (r *Repository) GetAgent(ctx context.Context, agentID string) (domain.Agent, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT agent_id, agent_type, owner_id, COALESCE(external_id,''), status, COALESCE(passport_id::text,''), metadata, registration_ts, updated_ts,
			COALESCE(admission_constraints, '{}'::jsonb), admitted_policy_id, admitted_policy_ver, admission_ts
		FROM agents WHERE agent_id = $1
	`, agentID)
	var a domain.Agent
	var metadata []byte
	var admission []byte
	var admittedPID *string
	var admittedPVer *string
	if err := row.Scan(&a.AgentID, &a.AgentType, &a.OwnerID, &a.ExternalID, &a.Status, &a.PassportID, &metadata, &a.RegistrationTS, &a.UpdatedTS,
		&admission, &admittedPID, &admittedPVer, &a.AdmissionTS); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Agent{}, fmt.Errorf("%w: agent_id %s", registryerr.ErrNotFound, agentID)
		}
		return domain.Agent{}, err
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &a.Metadata); err != nil {
			return domain.Agent{}, fmt.Errorf("invalid agent metadata JSON for %s: %w", agentID, err)
		}
	}
	if len(admission) > 0 {
		if err := json.Unmarshal(admission, &a.AdmissionConstraints); err != nil {
			return domain.Agent{}, fmt.Errorf("invalid admission_constraints JSON for %s: %w", agentID, err)
		}
	} else {
		a.AdmissionConstraints = map[string]any{}
	}
	if admittedPID != nil {
		a.AdmittedPolicyID = *admittedPID
	}
	if admittedPVer != nil {
		a.AdmittedPolicyVer = *admittedPVer
	}
	return a, nil
}

// GetAdmissionEnvelope returns the persisted admission envelope for an agent, if any.
func (r *Repository) GetAdmissionEnvelope(ctx context.Context, agentID string) (*domain.AdmissionEnvelope, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT envelope_id, agent_id, policy_id, policy_ver,
			COALESCE(admitted_scopes, '[]'::jsonb), COALESCE(admitted_behavioral_caps, '{}'::jsonb),
			admitted_ts, issuing_authority, COALESCE(signature, ''::bytea)
		FROM admission_envelopes WHERE agent_id = $1
	`, agentID)
	var env domain.AdmissionEnvelope
	var scopesJSON, capsJSON []byte
	var policyID, policyVer *string
	if err := row.Scan(&env.EnvelopeID, &env.AgentID, &policyID, &policyVer, &scopesJSON, &capsJSON, &env.AdmittedTS, &env.IssuingAuthority, &env.Signature); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: admission envelope for agent_id %s", registryerr.ErrNotFound, agentID)
		}
		return nil, err
	}
	if policyID != nil {
		env.PolicyID = *policyID
	}
	if policyVer != nil {
		env.PolicyVer = *policyVer
	}
	if len(scopesJSON) > 0 {
		if err := json.Unmarshal(scopesJSON, &env.AdmittedScopes); err != nil {
			return nil, fmt.Errorf("invalid admitted_scopes JSON: %w", err)
		}
	}
	if env.AdmittedScopes == nil {
		env.AdmittedScopes = []string{}
	}
	if len(capsJSON) > 0 {
		if err := json.Unmarshal(capsJSON, &env.AdmittedBehavioralCaps); err != nil {
			return nil, fmt.Errorf("invalid admitted_behavioral_caps JSON: %w", err)
		}
	}
	if env.AdmittedBehavioralCaps == nil {
		env.AdmittedBehavioralCaps = map[string]float64{}
	}
	return &env, nil
}

// ListAgents returns default list excluding deregistered when no status filter is set.
func (r *Repository) ListAgents(ctx context.Context, status, agentType, ownerID string, limit int, token string) ([]domain.Agent, string, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	args := []any{}
	clauses := []string{}
	add := func(c string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(c, len(args)))
	}

	if status != "" {
		add("status = $%d", status)
	} else {
		clauses = append(clauses, "status != 'DEREGISTERED'")
	}
	if agentType != "" {
		add("agent_type = $%d", strings.ToUpper(agentType))
	}
	if ownerID != "" {
		add("owner_id = $%d", ownerID)
	}
	if token != "" {
		add("agent_id > $%d", token)
	}
	args = append(args, limit+1)

	q := `SELECT agent_id, agent_type, owner_id, COALESCE(external_id,''), status, COALESCE(passport_id::text,''), metadata, registration_ts, updated_ts,
		COALESCE(admission_constraints, '{}'::jsonb), admitted_policy_id, admitted_policy_ver, admission_ts FROM agents`
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	q += fmt.Sprintf(" ORDER BY agent_id LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, "", 0, err
	}
	defer rows.Close()

	var result []domain.Agent
	var next string
	for rows.Next() {
		var a domain.Agent
		var metadata []byte
		var admission []byte
		var admittedPID *string
		var admittedPVer *string
		if err := rows.Scan(&a.AgentID, &a.AgentType, &a.OwnerID, &a.ExternalID, &a.Status, &a.PassportID, &metadata, &a.RegistrationTS, &a.UpdatedTS,
			&admission, &admittedPID, &admittedPVer, &a.AdmissionTS); err != nil {
			return nil, "", 0, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &a.Metadata); err != nil {
				return nil, "", 0, fmt.Errorf("invalid agent metadata JSON in list row %s: %w", a.AgentID, err)
			}
		}
		if len(admission) > 0 {
			if err := json.Unmarshal(admission, &a.AdmissionConstraints); err != nil {
				return nil, "", 0, fmt.Errorf("invalid admission_constraints JSON in list row %s: %w", a.AgentID, err)
			}
		} else {
			a.AdmissionConstraints = map[string]any{}
		}
		if admittedPID != nil {
			a.AdmittedPolicyID = *admittedPID
		}
		if admittedPVer != nil {
			a.AdmittedPolicyVer = *admittedPVer
		}
		result = append(result, a)
	}
	if len(result) > limit {
		next = result[limit].AgentID
		result = result[:limit]
	}
	return result, next, len(result), nil
}

// UpdateAgentStatus updates status and appends status history.
func (r *Repository) UpdateAgentStatus(ctx context.Context, agentID, newStatus, reason, changedBy string) (domain.Agent, string, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Agent{}, "", err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var prev string
	err = tx.QueryRow(ctx, `SELECT status FROM agents WHERE agent_id=$1 FOR UPDATE`, agentID).Scan(&prev)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Agent{}, "", fmt.Errorf("%w: agent_id %s", registryerr.ErrNotFound, agentID)
		}
		return domain.Agent{}, "", err
	}
	if !domain.CanTransition(prev, newStatus) {
		return domain.Agent{}, prev, fmt.Errorf("%w: invalid transition from %s to %s", registryerr.ErrFailedPrecondition, prev, newStatus)
	}
	if prev != newStatus {
		_, err = tx.Exec(ctx, `UPDATE agents SET status=$1, updated_ts=$2 WHERE agent_id=$3`, newStatus, time.Now().UTC(), agentID)
		if err != nil {
			return domain.Agent{}, prev, err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO agent_status_history(agent_id, previous_status, new_status, reason, changed_by)
			VALUES($1,$2,$3,$4,$5)
		`, agentID, prev, newStatus, reason, changedBy)
		if err != nil {
			return domain.Agent{}, prev, err
		}
		eventType := "AGENT_STATUS_CHANGED"
		if newStatus == domain.StatusDeregistered {
			eventType = "AGENT_DEREGISTERED"
		}
		payload, err := json.Marshal(map[string]any{
			"event_id":        uuid.NewString(),
			"event_type":      eventType,
			"agent_id":        agentID,
			"agent_type":      "",
			"owner_id":        "",
			"previous_status": prev,
			"new_status":      newStatus,
			"reason":          reason,
			"emitted_ts":      time.Now().UTC().UnixMilli(),
			"schema_version":  "1.0.0",
		})
		if err != nil {
			return domain.Agent{}, prev, err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO agent_lifecycle_outbox (agent_id, event_type, payload, status)
			VALUES($1,$2,$3,'PENDING')
		`, agentID, eventType, payload)
		if err != nil {
			return domain.Agent{}, prev, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Agent{}, prev, err
	}
	agent, err := r.GetAgent(ctx, agentID)
	return agent, prev, err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
