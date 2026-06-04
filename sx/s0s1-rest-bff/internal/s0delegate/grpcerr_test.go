package s0delegate

import (
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUpstreamHTTP(t *testing.T) {
	t.Parallel()
	if got := UpstreamHTTP(status.Error(codes.InvalidArgument, "bad")); got != http.StatusBadRequest {
		t.Fatalf("invalid arg: %d", got)
	}
	if got := UpstreamHTTP(status.Error(codes.NotFound, "nope")); got != http.StatusNotFound {
		t.Fatalf("not found: %d", got)
	}
	if got := UpstreamHTTP(status.Error(codes.Unavailable, "x")); got != http.StatusBadGateway {
		t.Fatalf("unavailable: %d", got)
	}
}
