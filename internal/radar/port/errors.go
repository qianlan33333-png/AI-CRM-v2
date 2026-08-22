package port

import "errors"

var (
	ErrInvalidArgument         = errors.New("invalid radar argument")
	ErrNotFound                = errors.New("radar link not found")
	ErrConflict                = errors.New("radar link conflict")
	ErrStateConflict           = errors.New("radar link state conflict")
	ErrIdempotencyConflict     = errors.New("radar idempotency conflict")
	ErrPublicCodeCollision     = errors.New("radar public code collision")
	ErrUnavailable             = errors.New("radar dependency unavailable")
	ErrIdempotencyStateInvalid = errors.New("radar idempotency state invalid")
)

// ValidationError is deliberately closed: callers may expose Field and Reason,
// but never the wrapped internal cause.
type ValidationError struct {
	Field  string
	Reason string
}

func (validation *ValidationError) Error() string { return ErrInvalidArgument.Error() }

func (validation *ValidationError) Unwrap() error { return ErrInvalidArgument }

func Invalid(field, reason string) error {
	return &ValidationError{Field: field, Reason: reason}
}
