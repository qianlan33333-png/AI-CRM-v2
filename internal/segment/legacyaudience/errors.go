package legacyaudience

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid AI Audience input")
	ErrNotFound            = errors.New("AI Audience resource not found")
	ErrConflict            = errors.New("AI Audience state conflict")
	ErrVersionConflict     = errors.New("AI Audience version conflict")
	ErrIdempotencyConflict = errors.New("AI Audience idempotency conflict")
	ErrGroupNotEmpty       = errors.New("AI Audience package group is not empty")
	ErrArchived            = errors.New("AI Audience package is archived")
	ErrUnavailable         = errors.New("AI Audience dependency unavailable")

	ErrUnauthenticated = errors.New("AI Audience authentication required")
	ErrForbidden       = errors.New("AI Audience permission denied")
	ErrCSRFInvalid     = errors.New("AI Audience CSRF validation failed")
)
