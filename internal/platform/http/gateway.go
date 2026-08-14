package platformhttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	RequestIDHeader                = "X-Request-ID"
	maxBufferedResponseBytes       = 8 << 20
	DefaultMaxConcurrentPerAccount = 4
	DefaultRequestTimeout          = 10 * time.Second
)

var (
	ErrInvalidGateway = errors.New("invalid HTTP gateway")
	requestIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
	accountPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]{0,63}$`)
	fallbackSequence  atomic.Uint64
)

type requestIDKey struct{}
type accountIDKey struct{}

type requestStateKey struct{}

type requestState struct {
	requestID    string
	accountID    atomic.Value
	routePattern atomic.Value
}

type GatewayOptions struct {
	Logger                  *slog.Logger
	Clock                   func() time.Time
	NewRequestID            func() string
	RoutePattern            func(*http.Request) string
	MaxConcurrentPerAccount int
	RequestTimeout          time.Duration
}

type Gateway struct {
	logger                  *slog.Logger
	clock                   func() time.Time
	newRequestID            func() string
	routePattern            func(*http.Request) string
	maxConcurrentPerAccount int
	requestTimeout          time.Duration
	budgetMu                sync.Mutex
	inflightByAccount       map[string]int
}

func NewGateway(options GatewayOptions) (*Gateway, error) {
	if options.Logger == nil {
		return nil, ErrInvalidGateway
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.NewRequestID == nil {
		options.NewRequestID = newRequestID
	}
	if options.RoutePattern == nil {
		options.RoutePattern = routePattern
	}
	if options.MaxConcurrentPerAccount == 0 {
		options.MaxConcurrentPerAccount = DefaultMaxConcurrentPerAccount
	}
	if options.RequestTimeout == 0 {
		options.RequestTimeout = DefaultRequestTimeout
	}
	if options.MaxConcurrentPerAccount < 1 || options.RequestTimeout < 1 {
		return nil, ErrInvalidGateway
	}
	return &Gateway{
		logger: options.Logger, clock: options.Clock, newRequestID: options.NewRequestID,
		routePattern: options.RoutePattern, maxConcurrentPerAccount: options.MaxConcurrentPerAccount,
		requestTimeout: options.RequestTimeout, inflightByAccount: make(map[string]int),
	}, nil
}

func (gateway *Gateway) Wrap(next http.Handler) (http.Handler, error) {
	if gateway == nil || gateway.logger == nil || gateway.clock == nil || gateway.newRequestID == nil || gateway.routePattern == nil || next == nil {
		return nil, ErrInvalidGateway
	}
	tail, err := gateway.RecoveryErrorLog(next)
	if err != nil {
		return nil, err
	}
	return gateway.RequestIDMiddleware(tail)
}

// RequestIDMiddleware is the outermost process middleware. Authentication and
// request-budget middleware may be inserted between it and RecoveryErrorLog.
func (gateway *Gateway) RequestIDMiddleware(next http.Handler) (http.Handler, error) {
	if gateway == nil || gateway.logger == nil || gateway.clock == nil || gateway.newRequestID == nil || gateway.routePattern == nil || next == nil {
		return nil, ErrInvalidGateway
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		startedAt := gateway.clock()
		requestID := strings.TrimSpace(request.Header.Get(RequestIDHeader))
		if !requestIDPattern.MatchString(requestID) {
			requestID = gateway.newRequestID()
		}
		if !requestIDPattern.MatchString(requestID) {
			requestID = newRequestID()
		}
		state := &requestState{requestID: requestID}
		ctx := context.WithValue(request.Context(), requestIDKey{}, requestID)
		request = request.WithContext(context.WithValue(ctx, requestStateKey{}, state))
		response := &observedResponse{underlying: writer}
		response.Header().Set(RequestIDHeader, requestID)
		defer func() { gateway.logAccess(request, response.status, response.errorCode, startedAt) }()
		next.ServeHTTP(response, request)
	}), nil
}

// AccountBudgetMiddleware limits in-flight work after authentication has bound
// an opaque account identifier. It never queues excess requests in-process.
func (gateway *Gateway) AccountBudgetMiddleware(next http.Handler) (http.Handler, error) {
	if gateway == nil || gateway.maxConcurrentPerAccount < 1 || gateway.inflightByAccount == nil || next == nil {
		return nil, ErrInvalidGateway
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		accountID := AccountID(request.Context())
		if accountID == "anonymous" {
			WriteError(writer, request, NewError(CodeUnauthenticated, nil))
			return
		}
		if !gateway.acquireAccount(accountID) {
			WriteError(writer, request, NewError(CodeConcurrencyLimited, nil))
			return
		}
		defer gateway.releaseAccount(accountID)
		next.ServeHTTP(writer, request)
	}), nil
}

// RoutePatternMiddleware records the frozen route template after routing and
// before authentication can short-circuit. Raw request paths are never stored.
func (gateway *Gateway) RoutePatternMiddleware(pattern string, next http.Handler) (http.Handler, error) {
	if gateway == nil || pattern == "unmatched" || safeRoutePattern(pattern) != pattern || next == nil {
		return nil, ErrInvalidGateway
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		state, ok := request.Context().Value(requestStateKey{}).(*requestState)
		if !ok || state == nil {
			WriteError(writer, request, NewError(CodeInternal, nil))
			return
		}
		state.routePattern.Store(pattern)
		next.ServeHTTP(writer, request)
	}), nil
}

// TimeoutMiddleware binds the default request deadline. Domain handlers must
// honor context cancellation; RecoveryErrorLog normalizes an elapsed deadline.
func (gateway *Gateway) TimeoutMiddleware(next http.Handler) (http.Handler, error) {
	if gateway == nil || gateway.requestTimeout < 1 || next == nil {
		return nil, ErrInvalidGateway
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), gateway.requestTimeout)
		defer cancel()
		next.ServeHTTP(writer, request.WithContext(ctx))
	}), nil
}

// RecoveryErrorLog is the fixed tail of the HTTP middleware chain: panic
// recovery and unified errors. RequestIDMiddleware emits the access record
// after this tail or any earlier middleware returns. Responses are buffered,
// so the platform business API does not support streaming handlers.
func (gateway *Gateway) RecoveryErrorLog(next http.Handler) (http.Handler, error) {
	if gateway == nil || next == nil {
		return nil, ErrInvalidGateway
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		response := newBufferedResponse(writer)
		response.Header().Set(RequestIDHeader, RequestID(request.Context()))

		defer func() {
			if recover() != nil {
				response.reset()
				WriteError(response, request, NewError(CodeInternal, nil))
			} else if errors.Is(request.Context().Err(), context.DeadlineExceeded) {
				response.reset()
				WriteError(response, request, NewError(CodeDependencyUnavailable, nil))
			}
			response.finalize(request)
		}()
		next.ServeHTTP(response, request)
	}), nil
}

func (gateway *Gateway) acquireAccount(accountID string) bool {
	gateway.budgetMu.Lock()
	defer gateway.budgetMu.Unlock()
	if gateway.inflightByAccount[accountID] >= gateway.maxConcurrentPerAccount {
		return false
	}
	gateway.inflightByAccount[accountID]++
	return true
}

func (gateway *Gateway) releaseAccount(accountID string) {
	gateway.budgetMu.Lock()
	defer gateway.budgetMu.Unlock()
	remaining := gateway.inflightByAccount[accountID] - 1
	if remaining <= 0 {
		delete(gateway.inflightByAccount, accountID)
		return
	}
	gateway.inflightByAccount[accountID] = remaining
}

func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if state, ok := ctx.Value(requestStateKey{}).(*requestState); ok && state != nil && requestIDPattern.MatchString(state.requestID) {
		return state.requestID
	}
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	if requestIDPattern.MatchString(requestID) {
		return requestID
	}
	return ""
}

func ContextWithAccountID(ctx context.Context, accountID string) (context.Context, error) {
	if ctx == nil || !accountPattern.MatchString(accountID) {
		return nil, ErrInvalidGateway
	}
	if state, ok := ctx.Value(requestStateKey{}).(*requestState); ok && state != nil {
		state.accountID.Store(accountID)
	}
	return context.WithValue(ctx, accountIDKey{}, accountID), nil
}

func AccountID(ctx context.Context) string {
	if ctx == nil {
		return "anonymous"
	}
	accountID, _ := ctx.Value(accountIDKey{}).(string)
	if accountPattern.MatchString(accountID) {
		return accountID
	}
	if state, ok := ctx.Value(requestStateKey{}).(*requestState); ok && state != nil {
		accountID, _ = state.accountID.Load().(string)
		if accountPattern.MatchString(accountID) {
			return accountID
		}
	}
	return "anonymous"
}

func newRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	sequence := fallbackSequence.Add(1)
	return "fallback-" + strings.ToLower(strconvBase36(sequence))
}

func strconvBase36(value uint64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if value == 0 {
		return "0"
	}
	var encoded [13]byte
	position := len(encoded)
	for value > 0 {
		position--
		encoded[position] = digits[value%36]
		value /= 36
	}
	return string(encoded[position:])
}

func routePattern(request *http.Request) string {
	if request == nil {
		return "unmatched"
	}
	if request.Pattern != "" {
		return safeRoutePattern(request.Pattern)
	}
	if routeContext := chi.RouteContext(request.Context()); routeContext != nil {
		if pattern := routeContext.RoutePattern(); pattern != "" {
			return safeRoutePattern(pattern)
		}
	}
	return "unmatched"
}

func safeRoutePattern(pattern string) string {
	if len(pattern) == 0 || len(pattern) > 160 || pattern[0] != '/' || strings.ContainsAny(pattern, "?\r\n\t") {
		return "unmatched"
	}
	return pattern
}

func (gateway *Gateway) logAccess(request *http.Request, status int, errorCode ErrorCode, startedAt time.Time) {
	if status == 0 {
		status = http.StatusOK
	}
	level := slog.LevelInfo
	if status >= http.StatusInternalServerError {
		level = slog.LevelError
	} else if status >= http.StatusBadRequest {
		level = slog.LevelWarn
	}
	gateway.logger.LogAttrs(request.Context(), level, "http_access",
		slog.String("request_id", RequestID(request.Context())),
		slog.String("account", AccountID(request.Context())),
		slog.String("method", safeMethod(request.Method)),
		slog.String("path", gateway.observedRoutePattern(request)),
		slog.Int("status", status),
		slog.Int64("latency_ms", gateway.clock().Sub(startedAt).Milliseconds()),
		slog.String("err", string(errorCode)),
	)
}

func (gateway *Gateway) observedRoutePattern(request *http.Request) string {
	if request != nil {
		if state, ok := request.Context().Value(requestStateKey{}).(*requestState); ok && state != nil {
			pattern, _ := state.routePattern.Load().(string)
			if safeRoutePattern(pattern) == pattern {
				return pattern
			}
		}
	}
	return safeRoutePattern(gateway.routePattern(request))
}

func safeMethod(method string) string {
	if len(method) == 0 || len(method) > 16 {
		return "UNKNOWN"
	}
	for _, character := range method {
		if character < 'A' || character > 'Z' {
			return "UNKNOWN"
		}
	}
	return method
}

type bufferedResponse struct {
	underlying http.ResponseWriter
	header     http.Header
	body       bytes.Buffer
	status     int
	errorCode  ErrorCode
	overflow   bool
}

type observedResponse struct {
	underlying http.ResponseWriter
	status     int
	errorCode  ErrorCode
}

func (response *observedResponse) Header() http.Header { return response.underlying.Header() }

func (response *observedResponse) WriteHeader(status int) {
	if response.status != 0 {
		return
	}
	response.status = status
	response.underlying.WriteHeader(status)
}

func (response *observedResponse) Write(content []byte) (int, error) {
	if response.status == 0 {
		response.WriteHeader(http.StatusOK)
	}
	return response.underlying.Write(content)
}

func (response *observedResponse) markError(code ErrorCode) { response.errorCode = code }

func newBufferedResponse(underlying http.ResponseWriter) *bufferedResponse {
	return &bufferedResponse{underlying: underlying, header: make(http.Header)}
}

func (response *bufferedResponse) Header() http.Header { return response.header }

func (response *bufferedResponse) WriteHeader(status int) {
	if response.status != 0 {
		return
	}
	response.status = status
}

func (response *bufferedResponse) Write(content []byte) (int, error) {
	if response.status == 0 {
		response.status = http.StatusOK
	}
	if response.body.Len()+len(content) > maxBufferedResponseBytes {
		response.overflow = true
		return len(content), nil
	}
	return response.body.Write(content)
}

func (response *bufferedResponse) markError(code ErrorCode) { response.errorCode = code }

func (response *bufferedResponse) reset() {
	response.header = make(http.Header)
	response.body.Reset()
	response.status = 0
	response.errorCode = ""
	response.overflow = false
}

func (response *bufferedResponse) finalize(request *http.Request) {
	if response.overflow {
		response.reset()
		WriteError(response, request, NewError(CodeInternal, nil))
	}
	if response.status == 0 {
		response.status = http.StatusOK
	}
	// Browser compatibility routes legitimately use redirects. Error
	// normalization begins at 4xx; redirect target safety remains the owning
	// handler's responsibility.
	if response.status >= http.StatusBadRequest && response.errorCode == "" {
		code := defaultCodeForStatus(response.status)
		response.reset()
		WriteError(response, request, NewError(code, nil))
	}
	for key := range response.underlying.Header() {
		response.underlying.Header().Del(key)
	}
	for key, values := range response.header {
		for _, value := range values {
			response.underlying.Header().Add(key, value)
		}
	}
	if marker, ok := response.underlying.(errorMarker); ok && response.errorCode != "" {
		marker.markError(response.errorCode)
	}
	response.underlying.WriteHeader(response.status)
	if response.status != http.StatusNoContent {
		_, _ = response.underlying.Write(response.body.Bytes())
	}
}
