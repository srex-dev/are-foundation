package registryerr

import "errors"

// Sentinel errors for stable gRPC code mapping via errors.Is.
// Wrap with fmt.Errorf("...: %w", ErrHere) from repository/service layers.
var (
	ErrNotFound           = errors.New("NOT_FOUND")
	ErrAlreadyExists      = errors.New("ALREADY_EXISTS")
	ErrInvalidArgument    = errors.New("INVALID_ARGUMENT")
	ErrFailedPrecondition = errors.New("FAILED_PRECONDITION")
)
