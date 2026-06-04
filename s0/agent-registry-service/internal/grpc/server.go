package grpcserver

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"

	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/domain"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/registryerr"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/service"
	registryv1 "github.com/srex-dev/are-foundation/s0/agent-registry-service/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the AgentRegistryService gRPC contract.
type Server struct {
	registryv1.UnimplementedAgentRegistryServiceServer
	registry *service.Registry
}

// New returns a gRPC server adapter for the registry service.
func New(registry *service.Registry) *Server {
	return &Server{registry: registry}
}

// RegisterAgent creates a new agent and returns the persisted record.
func (s *Server) RegisterAgent(ctx context.Context, req *registryv1.RegisterAgentRequest) (*registryv1.RegisterAgentResponse, error) {
	hasAdmission := len(req.GetAdmissionConstraintsJson()) > 0 ||
		req.GetAdmittedPolicyId() != "" ||
		req.GetAdmittedPolicyVer() != "" ||
		len(req.GetAdmittedScopePatterns()) > 0 ||
		len(req.GetAdmittedBehavioralCaps()) > 0 ||
		len(req.GetAdmissionSignature()) > 0 ||
		req.GetIssuingAuthority() != "" ||
		req.GetEnvelopeId() != ""

	var admission *domain.AdmissionSnapshot
	if hasAdmission {
		admission = &domain.AdmissionSnapshot{
			PolicyID:         req.GetAdmittedPolicyId(),
			PolicyVer:        req.GetAdmittedPolicyVer(),
			Constraints:      map[string]any{},
			ScopePatterns:    append([]string(nil), req.GetAdmittedScopePatterns()...),
			EnvelopeID:       req.GetEnvelopeId(),
			IssuingAuthority: req.GetIssuingAuthority(),
			Signature:        append([]byte(nil), req.GetAdmissionSignature()...),
		}
		if caps := req.GetAdmittedBehavioralCaps(); len(caps) > 0 {
			admission.BehavioralCaps = maps.Clone(caps)
		}
		if len(req.GetAdmissionConstraintsJson()) > 0 {
			if err := json.Unmarshal(req.GetAdmissionConstraintsJson(), &admission.Constraints); err != nil {
				return nil, status.Error(codes.InvalidArgument, "admission_constraints_json must be a JSON object")
			}
		}
	}
	agent, err := s.registry.RegisterWithAdmission(ctx, req.GetAgentType(), req.GetOwnerId(), req.GetExternalId(), req.GetMetadata(), admission)
	if err != nil {
		return nil, mapError(err)
	}
	return &registryv1.RegisterAgentResponse{Agent: toProtoAgent(agent)}, nil
}

// GetAgent fetches one agent by id.
func (s *Server) GetAgent(ctx context.Context, req *registryv1.GetAgentRequest) (*registryv1.GetAgentResponse, error) {
	agent, err := s.registry.Get(ctx, req.GetAgentId())
	if err != nil {
		return nil, mapError(err)
	}
	return &registryv1.GetAgentResponse{Agent: toProtoAgent(agent)}, nil
}

// GetAdmissionEnvelope returns the persisted admission envelope for an agent.
func (s *Server) GetAdmissionEnvelope(ctx context.Context, req *registryv1.GetAdmissionEnvelopeRequest) (*registryv1.GetAdmissionEnvelopeResponse, error) {
	env, err := s.registry.GetAdmissionEnvelope(ctx, req.GetAgentId())
	if err != nil {
		return nil, mapError(err)
	}
	return &registryv1.GetAdmissionEnvelopeResponse{Envelope: toProtoAdmissionEnvelope(env)}, nil
}

// ListAgents returns paginated agents filtered by status/type/owner.
func (s *Server) ListAgents(ctx context.Context, req *registryv1.ListAgentsRequest) (*registryv1.ListAgentsResponse, error) {
	agents, nextToken, totalCount, err := s.registry.List(
		ctx,
		req.GetStatus(),
		req.GetType(),
		req.GetOwnerId(),
		int(req.GetPageSize()),
		req.GetPageToken(),
	)
	if err != nil {
		return nil, mapError(err)
	}
	resp := &registryv1.ListAgentsResponse{
		Agents:        make([]*registryv1.Agent, 0, len(agents)),
		NextPageToken: nextToken,
		TotalCount:    int32(totalCount),
	}
	for _, a := range agents {
		resp.Agents = append(resp.Agents, toProtoAgent(a))
	}
	return resp, nil
}

// UpdateAgentStatus applies validated lifecycle transitions.
func (s *Server) UpdateAgentStatus(ctx context.Context, req *registryv1.UpdateAgentStatusRequest) (*registryv1.UpdateAgentStatusResponse, error) {
	agent, err := s.registry.UpdateStatus(ctx, req.GetAgentId(), strings.ToUpper(req.GetNewStatus()), req.GetReason(), req.GetUpdatedBy())
	if err != nil {
		return nil, mapError(err)
	}
	return &registryv1.UpdateAgentStatusResponse{Agent: toProtoAgent(agent)}, nil
}

// CheckAgent checks existence and status with lightweight response.
func (s *Server) CheckAgent(ctx context.Context, req *registryv1.CheckAgentRequest) (*registryv1.CheckAgentResponse, error) {
	agent, err := s.registry.Get(ctx, req.GetAgentId())
	if err != nil {
		if errors.Is(err, registryerr.ErrNotFound) {
			return &registryv1.CheckAgentResponse{
				AgentId: req.GetAgentId(),
				Status:  "",
				Exists:  false,
			}, nil
		}
		return nil, mapError(err)
	}
	return &registryv1.CheckAgentResponse{
		AgentId: agent.AgentID,
		Status:  agent.Status,
		Exists:  true,
	}, nil
}

func toProtoAgent(a domain.Agent) *registryv1.Agent {
	out := &registryv1.Agent{
		AgentId:           a.AgentID,
		AgentType:         a.AgentType,
		OwnerId:           a.OwnerID,
		ExternalId:        a.ExternalID,
		Status:            a.Status,
		PassportId:        a.PassportID,
		RegistrationTs:    a.RegistrationTS.UnixMilli(),
		UpdatedTs:         a.UpdatedTS.UnixMilli(),
		Metadata:          a.Metadata,
		AdmittedPolicyId:  a.AdmittedPolicyID,
		AdmittedPolicyVer: a.AdmittedPolicyVer,
		AdmissionTs:       a.AdmissionTS.UnixMilli(),
	}
	if len(a.AdmissionConstraints) > 0 {
		b, err := json.Marshal(a.AdmissionConstraints)
		if err == nil {
			out.AdmissionConstraintsJson = b
		}
	}
	return out
}

func toProtoAdmissionEnvelope(e *domain.AdmissionEnvelope) *registryv1.AdmissionEnvelope {
	if e == nil {
		return nil
	}
	out := &registryv1.AdmissionEnvelope{
		EnvelopeId:       e.EnvelopeID,
		AgentId:          e.AgentID,
		PolicyId:         e.PolicyID,
		PolicyVer:        e.PolicyVer,
		AdmittedScopes:   append([]string(nil), e.AdmittedScopes...),
		AdmittedTsMs:     e.AdmittedTS.UnixMilli(),
		IssuingAuthority: e.IssuingAuthority,
		Signature:        append([]byte(nil), e.Signature...),
	}
	if len(e.AdmittedBehavioralCaps) > 0 {
		out.AdmittedBehavioralCaps = maps.Clone(e.AdmittedBehavioralCaps)
	}
	return out
}

func mapError(err error) error {
	// Map only on typed sentinel errors (PA-4); never parse err.Error() for gRPC codes.
	switch {
	case errors.Is(err, registryerr.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, registryerr.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, registryerr.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, registryerr.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
