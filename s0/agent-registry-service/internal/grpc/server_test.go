package grpcserver

import (
	"fmt"
	"testing"

	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/registryerr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMapErrorTypedSentinels(t *testing.T) {
	tests := []struct {
		err  error
		code codes.Code
	}{
		{fmt.Errorf("%w: agent_id a", registryerr.ErrNotFound), codes.NotFound},
		{fmt.Errorf("%w: dup", registryerr.ErrAlreadyExists), codes.AlreadyExists},
		{fmt.Errorf("%w: bad input", registryerr.ErrInvalidArgument), codes.InvalidArgument},
		{fmt.Errorf("%w: transition", registryerr.ErrFailedPrecondition), codes.FailedPrecondition},
	}
	for _, tc := range tests {
		st, ok := status.FromError(mapError(tc.err))
		if !ok {
			t.Fatalf("expected gRPC status for %v", tc.err)
		}
		if st.Code() != tc.code {
			t.Fatalf("for %v: got code %v want %v", tc.err, st.Code(), tc.code)
		}
	}
}

func TestMapErrorUnknownIsInternal(t *testing.T) {
	st, ok := status.FromError(mapError(fmt.Errorf("some datastore failure")))
	if !ok {
		t.Fatal("expected status")
	}
	if st.Code() != codes.Internal {
		t.Fatalf("got %v want Internal", st.Code())
	}
	if st.Message() != "internal error" {
		t.Fatalf("unexpected message leak: %q", st.Message())
	}
}
