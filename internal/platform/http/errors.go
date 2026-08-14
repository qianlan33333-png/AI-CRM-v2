package platformhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
)

type ErrorCode string

const (
	CodeMalformedRequest      ErrorCode = "MALFORMED_REQUEST"
	CodeCursorInvalid         ErrorCode = "CURSOR_INVALID"
	CodeUnauthenticated       ErrorCode = "UNAUTHENTICATED"
	CodeUnauthorized          ErrorCode = "UNAUTHORIZED"
	CodeNotFound              ErrorCode = "NOT_FOUND"
	CodeConflict              ErrorCode = "CONFLICT"
	CodeValidationFailed      ErrorCode = "VALIDATION_FAILED"
	CodeConcurrencyLimited    ErrorCode = "CONCURRENCY_LIMITED"
	CodeInternal              ErrorCode = "INTERNAL_ERROR"
	CodeDependencyUnavailable ErrorCode = "DEPENDENCY_UNAVAILABLE"
)

var (
	ErrInvalidErrorContract = errors.New("invalid HTTP error contract")
	fieldPattern            = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\[\]-]{0,63}$`)
	reasonPattern           = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

type errorSpec struct {
	status  int
	message string
}

var errorSpecs = map[ErrorCode]errorSpec{
	CodeMalformedRequest:      {status: http.StatusBadRequest, message: "The request is malformed."},
	CodeCursorInvalid:         {status: http.StatusBadRequest, message: "The cursor is invalid."},
	CodeUnauthenticated:       {status: http.StatusUnauthorized, message: "Authentication is required."},
	CodeUnauthorized:          {status: http.StatusForbidden, message: "Permission is denied."},
	CodeNotFound:              {status: http.StatusNotFound, message: "The resource was not found."},
	CodeConflict:              {status: http.StatusConflict, message: "The request conflicts with the current state."},
	CodeValidationFailed:      {status: http.StatusUnprocessableEntity, message: "Validation failed."},
	CodeConcurrencyLimited:    {status: http.StatusTooManyRequests, message: "Too many concurrent requests."},
	CodeInternal:              {status: http.StatusInternalServerError, message: "An internal error occurred."},
	CodeDependencyUnavailable: {status: http.StatusServiceUnavailable, message: "A required dependency is unavailable."},
}

type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type HTTPError struct {
	code    ErrorCode
	cause   error
	details []FieldError
}

func NewError(code ErrorCode, cause error, details ...FieldError) *HTTPError {
	if _, exists := errorSpecs[code]; !exists || !validDetails(details) {
		return &HTTPError{code: CodeInternal, cause: errors.Join(ErrInvalidErrorContract, cause)}
	}
	return &HTTPError{code: code, cause: cause, details: append([]FieldError(nil), details...)}
}

func (httpError *HTTPError) Error() string {
	if httpError == nil {
		return string(CodeInternal)
	}
	return string(httpError.code)
}

func (httpError *HTTPError) Unwrap() error {
	if httpError == nil {
		return nil
	}
	return httpError.cause
}

func (httpError *HTTPError) Code() ErrorCode {
	if httpError == nil {
		return CodeInternal
	}
	if _, exists := errorSpecs[httpError.code]; !exists {
		return CodeInternal
	}
	return httpError.code
}

func ErrorCodeOf(err error) ErrorCode {
	var httpError *HTTPError
	if errors.As(err, &httpError) {
		return httpError.Code()
	}
	return CodeInternal
}

type errorResponse struct {
	Code      ErrorCode    `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id"`
	Details   []FieldError `json:"details,omitempty"`
}

type errorMarker interface {
	markError(ErrorCode)
}

// MarkCompatibilityError preserves an explicitly frozen compatibility body
// while retaining the gateway's error classification and access-log code.
// Callers must write a bounded, non-secret JSON body immediately afterwards.
func MarkCompatibilityError(writer http.ResponseWriter, code ErrorCode) {
	if marker, ok := writer.(errorMarker); ok {
		marker.markError(code)
	}
}

func WriteError(writer http.ResponseWriter, request *http.Request, err error) {
	if writer == nil || request == nil {
		return
	}
	code, details := classifyError(err)
	spec := errorSpecs[code]
	requestID := RequestID(request.Context())
	if requestID == "" {
		requestID = newRequestID()
	}
	if marker, ok := writer.(errorMarker); ok {
		marker.markError(code)
	}
	writer.Header().Set(RequestIDHeader, requestID)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(spec.status)
	_ = json.NewEncoder(writer).Encode(errorResponse{
		Code: code, Message: spec.message, RequestID: requestID, Details: details,
	})
}

func RequestErrorHandler(writer http.ResponseWriter, request *http.Request, err error) {
	WriteError(writer, request, NewError(CodeMalformedRequest, err))
}

func ResponseErrorHandler(writer http.ResponseWriter, request *http.Request, err error) {
	WriteError(writer, request, err)
}

func classifyError(err error) (ErrorCode, []FieldError) {
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError == nil {
		return CodeInternal, nil
	}
	code := httpError.Code()
	if code == CodeInternal || !validDetails(httpError.details) {
		return CodeInternal, nil
	}
	return code, append([]FieldError(nil), httpError.details...)
}

func validDetails(details []FieldError) bool {
	for _, detail := range details {
		if !fieldPattern.MatchString(detail.Field) || !reasonPattern.MatchString(detail.Reason) {
			return false
		}
	}
	return true
}

func defaultCodeForStatus(status int) ErrorCode {
	switch status {
	case http.StatusBadRequest:
		return CodeMalformedRequest
	case http.StatusUnauthorized:
		return CodeUnauthenticated
	case http.StatusForbidden:
		return CodeUnauthorized
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	case http.StatusUnprocessableEntity:
		return CodeValidationFailed
	case http.StatusTooManyRequests:
		return CodeConcurrencyLimited
	case http.StatusServiceUnavailable:
		return CodeDependencyUnavailable
	default:
		return CodeInternal
	}
}
