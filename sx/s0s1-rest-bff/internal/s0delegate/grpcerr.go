package s0delegate

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UpstreamHTTP maps a gRPC error to an HTTP status for the BFF.
func UpstreamHTTP(err error) (httpStatus int) {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusBadGateway
	}
	switch st.Code() {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists, codes.Aborted:
		return http.StatusConflict
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unavailable, codes.ResourceExhausted:
		return http.StatusBadGateway
	default:
		return http.StatusBadGateway
	}
}

// UpstreamIsRetryable is true for errors where clients may retry.
func UpstreamIsRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return true
	}
	switch st.Code() {
	case codes.DeadlineExceeded, codes.Unavailable, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}
