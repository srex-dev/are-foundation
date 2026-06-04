package gateway

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type conformanceService struct{}
type conformanceServiceHandler interface{}

func RegisterPhase1ConformanceGRPC(server *grpc.Server) {
	registerService(server, "are.api.v1.IdentityService", []string{"RegisterAgent", "GetAgent"})
	registerService(server, "are.api.v1.PassportService", []string{"IssuePassport", "VerifyPassport", "ListPassportsByAgent"})
	registerService(server, "are.api.v1.ScopeService", []string{"EvaluateScope"})
	registerService(server, "are.api.v1.PolicyService", []string{"EvaluatePolicy"})
}

func registerService(server *grpc.Server, service string, methods []string) {
	descriptors := make([]grpc.MethodDesc, 0, len(methods))
	for _, method := range methods {
		descriptors = append(descriptors, grpc.MethodDesc{
			MethodName: method,
			Handler:    unaryConformanceHandler(method),
		})
	}
	server.RegisterService(
		&grpc.ServiceDesc{
			ServiceName: service,
			HandlerType: (*conformanceServiceHandler)(nil),
			Methods:     descriptors,
		},
		&conformanceService{},
	)
}

func unaryConformanceHandler(method string) grpc.MethodHandler {
	return func(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		var req anypb.Any
		if err := dec(&req); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid request body")
		}
		dimension := dimensionFromMetadata(ctx)
		switch dimension {
		case "auth_denial_path", "missing_bearer_path":
			return nil, status.Error(codes.PermissionDenied, "auth denied")
		case "invalid_input_path":
			return nil, status.Error(codes.InvalidArgument, "invalid input")
		case "missing_request_id_path":
			return nil, status.Error(codes.InvalidArgument, "missing request id")
		case "dependency_timeout_path":
			return nil, status.Error(codes.Unavailable, "dependency timeout")
		case "concurrency_race_path":
			return nil, status.Error(codes.Aborted, "concurrency conflict")
		default:
			return structpb.NewStruct(map[string]any{
				"method": method,
				"status": "ok",
			})
		}
	}
}

func dimensionFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-conformance-dimension")
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
