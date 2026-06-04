package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/domain"
	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/metrics"
	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/service"
	passportv1 "github.com/srex-dev/are-foundation/s0/passport-issuance-engine/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements PassportIssuanceService. RenewPassport and ListPassports remain unimplemented (forward-compatible embed).
type Server struct {
	passportv1.UnimplementedPassportIssuanceServiceServer
	svc *service.Service
}

// New constructs a gRPC API server backed by the domain service.
func New(svc *service.Service) *Server {
	return &Server{svc: svc}
}

func (s *Server) IssuePassport(ctx context.Context, req *passportv1.IssuePassportRequest) (*passportv1.IssuePassportResponse, error) {
	start := time.Now()
	result := "error"
	defer func() {
		metrics.IssuedTotal.WithLabelValues(result).Inc()
		metrics.ObserveDuration("issue", start, result)
	}()
	_ = ctx
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "nil request")
	}
	pType, err := parsePassportType(req.PassportType)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if len(req.RequestedScopes) == 0 {
		return nil, status.Error(codes.InvalidArgument, "requested_scopes required")
	}
	now := time.Now().UTC()
	scopes := make([]domain.Scope, 0, len(req.RequestedScopes))
	for _, sr := range req.RequestedScopes {
		if sr == nil {
			continue
		}
		scopes = append(scopes, scopeFromProto(sr, req.TtlSeconds, now))
	}
	if len(scopes) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no valid scopes")
	}
	p, err := s.svc.IssuePassport(service.IssueInput{
		AgentID:      req.AgentId,
		PassportType: pType,
		Scopes:       scopes,
		TTLSeconds:   req.TtlSeconds,
		IssuedBy:     req.IssuedBy,
		ForceReissue: req.ForceReissue,
		Reason:       req.Reason,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	result = "success"
	return &passportv1.IssuePassportResponse{Passport: passportToProto(p)}, nil
}

func (s *Server) GetPassport(ctx context.Context, req *passportv1.GetPassportRequest) (*passportv1.GetPassportResponse, error) {
	_ = ctx
	if req == nil || req.PassportId == "" {
		return nil, status.Error(codes.InvalidArgument, "passport_id required")
	}
	p, err := s.svc.GetPassport(req.PassportId)
	if err != nil {
		return nil, mapErr(err)
	}
	return &passportv1.GetPassportResponse{Passport: passportToProto(p)}, nil
}

func (s *Server) GetAgentPassport(ctx context.Context, req *passportv1.GetAgentPassportRequest) (*passportv1.GetAgentPassportResponse, error) {
	_ = ctx
	if req == nil || req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id required")
	}
	p, err := s.svc.GetAgentPassport(req.AgentId)
	if err != nil {
		return nil, mapErr(err)
	}
	return &passportv1.GetAgentPassportResponse{Passport: passportToProto(p)}, nil
}

func (s *Server) VerifyPassport(ctx context.Context, req *passportv1.VerifyPassportRequest) (*passportv1.VerifyPassportResponse, error) {
	start := time.Now()
	result := "error"
	metricReason := "error"
	defer func() {
		metrics.VerifyTotal.WithLabelValues(result, metricReason).Inc()
		metrics.ObserveDuration("verify", start, result)
	}()
	_ = ctx
	if req == nil || req.PassportId == "" {
		metricReason = "invalid_argument"
		return nil, status.Error(codes.InvalidArgument, "passport_id required")
	}
	ok, st, failureReason, err := s.svc.VerifyPassport(req.PassportId, req.Payload, req.Signature)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			result = "invalid"
			metricReason = "not_found"
			return &passportv1.VerifyPassportResponse{
				Valid:         false,
				PassportId:    req.PassportId,
				FailureReason: "not_found",
			}, nil
		}
		metricReason = "dependency_error"
		return nil, mapErr(err)
	}
	metricReason = failureReason
	if metricReason == "" {
		metricReason = "ok"
	}
	resp := &passportv1.VerifyPassportResponse{
		Valid:         ok,
		PassportId:    req.PassportId,
		Status:        string(st),
		FailureReason: failureReason,
		ExpiresTs:     0,
		ActiveScopes:  nil,
	}
	if !ok {
		result = "invalid"
		return resp, nil
	}
	p, err := s.svc.GetPassport(req.PassportId)
	if err == nil {
		resp.ExpiresTs = p.ExpiresTS.UnixMilli()
		resp.ActiveScopes = scopesToProto(p.ScopeSet)
	}
	result = "valid"
	return resp, nil
}

func (s *Server) RevokePassport(ctx context.Context, req *passportv1.RevokePassportRequest) (*passportv1.RevokePassportResponse, error) {
	start := time.Now()
	result := "error"
	defer func() {
		metrics.RevokedTotal.WithLabelValues(result).Inc()
		metrics.ObserveDuration("revoke", start, result)
	}()
	_ = ctx
	if req == nil || req.PassportId == "" || req.Reason == "" || req.RevokedBy == "" {
		return nil, status.Error(codes.InvalidArgument, "passport_id, reason, revoked_by required")
	}
	p, err := s.svc.RevokePassport(req.PassportId, req.Reason, req.RevokedBy)
	if err != nil {
		return nil, mapErr(err)
	}
	result = "success"
	var revokedTS int64
	if p.RevokedTS != nil {
		revokedTS = p.RevokedTS.UnixMilli()
	}
	return &passportv1.RevokePassportResponse{
		PassportId: p.PassportID,
		Status:     string(p.Status),
		RevokedTs:  revokedTS,
	}, nil
}

func mapErr(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, service.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, service.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func parsePassportType(raw string) (domain.PassportType, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", string(domain.TypeStandard):
		return domain.TypeStandard, nil
	case string(domain.TypeProvisional):
		return domain.TypeProvisional, nil
	default:
		return "", errors.New("invalid passport_type")
	}
}

func scopeFromProto(sr *passportv1.ScopeRequest, passportTTL int64, now time.Time) domain.Scope {
	ttl := sr.ScopeTtlSeconds
	if ttl <= 0 {
		ttl = passportTTL
	}
	if ttl <= 0 {
		ttl = 3600
	}
	ctx := sr.ContextConstraints
	if ctx == nil {
		ctx = map[string]string{}
	}
	return domain.Scope{
		ScopeID:            uuid.NewString(),
		ActionClass:        sr.ActionClass,
		ResourcePattern:    sr.ResourcePattern,
		ContextConstraints: ctx,
		GrantedTS:          now,
		ExpiresTS:          now.Add(time.Duration(ttl) * time.Second),
	}
}

func passportToProto(p domain.Passport) *passportv1.Passport {
	var revoked int64
	if p.RevokedTS != nil {
		revoked = p.RevokedTS.UnixMilli()
	}
	return &passportv1.Passport{
		PassportId:          p.PassportID,
		AgentId:             p.AgentID,
		PassportType:        string(p.PassportType),
		ScopeSet:            scopesToProto(p.ScopeSet),
		Status:              string(p.Status),
		IssuedBy:            p.IssuedBy,
		CredentialId:        p.CredentialID,
		PublicKeyPem:        p.PublicKeyPEM,
		Signature:           p.Signature,
		IssuedTs:            p.IssuedTS.UnixMilli(),
		ExpiresTs:           p.ExpiresTS.UnixMilli(),
		RevokedTs:           revoked,
		RevocationReason:    p.RevocationReason,
		SupersededBy:        p.SupersededBy,
		PolicyIdAtIssuance:  p.PolicyIDAtIssuance,
		PolicyVerAtIssuance: p.PolicyVerAtIssuance,
	}
}

func scopesToProto(scopes []domain.Scope) []*passportv1.PassportScope {
	out := make([]*passportv1.PassportScope, 0, len(scopes))
	for _, s := range scopes {
		cc := s.ContextConstraints
		if cc == nil {
			cc = map[string]string{}
		}
		out = append(out, &passportv1.PassportScope{
			ScopeId:            s.ScopeID,
			ActionClass:        s.ActionClass,
			ResourcePattern:    s.ResourcePattern,
			ContextConstraints: cc,
			GrantedTs:          s.GrantedTS.UnixMilli(),
			ExpiresTs:          s.ExpiresTS.UnixMilli(),
			IsEscalation:       s.IsEscalation,
		})
	}
	return out
}
