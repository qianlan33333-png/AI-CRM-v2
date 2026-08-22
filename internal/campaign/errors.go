package campaign

import "errors"

var (
	ErrInvalidArgument     = errors.New("invalid campaign argument")
	ErrNotFound            = errors.New("campaign not found")
	ErrConflict            = errors.New("campaign version conflict")
	ErrStateConflict       = errors.New("campaign state conflict")
	ErrIdempotencyConflict = errors.New("campaign idempotency conflict")
	ErrUnavailable         = errors.New("campaign dependency unavailable")
)

type ValidationError struct{ Field, Reason string }

func (err *ValidationError) Error() string { return ErrInvalidArgument.Error() }
func (err *ValidationError) Unwrap() error { return ErrInvalidArgument }
func Invalid(field, reason string) error   { return &ValidationError{Field: field, Reason: reason} }
