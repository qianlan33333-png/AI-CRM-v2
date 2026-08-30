package app

import (
	"context"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- SHA-1 is required by the WeCom JSSDK signature protocol.
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	defaultAgentConfigRefreshBefore = 5 * time.Minute
	agentConfigNonceBytes           = 16
)

var (
	ErrJSSDKDisabled    = errors.New("sidebar jssdk disabled")
	ErrJSSDKInvalid     = errors.New("sidebar jssdk input invalid")
	ErrJSSDKUnavailable = errors.New("sidebar jssdk unavailable")
)

// AgentConfigTicketProvider fetches the two read-only tickets required by the
// WeCom two-stage initialization: wx.config, then wx.agentConfig.
type AgentConfigTicketProvider interface {
	FetchConfigTicket(context.Context) (AgentConfigTicket, error)
	FetchAgentConfigTicket(context.Context) (AgentConfigTicket, error)
}

type AgentConfigTicket struct {
	Value     string
	ExpiresAt time.Time
}

type JSSDKServiceConfig struct {
	Enabled      bool
	CorpID       string
	AgentID      int64
	AllowedHosts []string
}

type JSSDKOptions struct {
	Clock         func() time.Time
	Random        io.Reader
	RefreshBefore time.Duration
}

type AgentConfigSignature struct {
	SignatureType   string    `json:"signature_type"`
	CorpID          string    `json:"corp_id"`
	AgentID         int64     `json:"agent_id"`
	Nonce           string    `json:"nonce"`
	Timestamp       int64     `json:"timestamp"`
	Signature       string    `json:"signature"`
	URL             string    `json:"url"`
	TicketExpiresAt time.Time `json:"ticket_expires_at"`
}

type JSSDKConfig struct {
	CorpID      string               `json:"corp_id"`
	AgentID     int64                `json:"agent_id"`
	Config      AgentConfigSignature `json:"config"`
	AgentConfig AgentConfigSignature `json:"agent_config"`
}

type JSSDKService struct {
	enabled      bool
	corpID       string
	agentID      int64
	allowedHosts map[string]struct{}
	configCache  *agentConfigTicketCache
	agentCache   *agentConfigTicketCache
	now          func() time.Time
	random       io.Reader
}

func NewJSSDKService(config JSSDKServiceConfig, provider AgentConfigTicketProvider, options JSSDKOptions) (*JSSDKService, error) {
	if !config.Enabled {
		return &JSSDKService{}, nil
	}
	if nilJSSDKDependency(provider) || !validOAuthSubject(config.CorpID) || config.AgentID < 1 {
		return nil, ErrJSSDKUnavailable
	}
	allowedHosts, ok := normalizeJSSDKHosts(config.AllowedHosts)
	if !ok {
		return nil, ErrJSSDKUnavailable
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if nilJSSDKDependency(options.Random) {
		return nil, ErrJSSDKUnavailable
	}
	if options.RefreshBefore == 0 {
		options.RefreshBefore = defaultAgentConfigRefreshBefore
	}
	if options.RefreshBefore < time.Minute || options.RefreshBefore > 15*time.Minute {
		return nil, ErrJSSDKUnavailable
	}
	return &JSSDKService{
		enabled: true, corpID: config.CorpID, agentID: config.AgentID, allowedHosts: allowedHosts,
		configCache: newConfigTicketCache(provider, options.Clock, options.RefreshBefore),
		agentCache:  newAgentConfigTicketCache(provider, options.Clock, options.RefreshBefore),
		now:         options.Clock, random: options.Random,
	}, nil
}

func (service *JSSDKService) AgentConfig(ctx context.Context, rawURL string) (JSSDKConfig, error) {
	if service == nil || !service.enabled {
		return JSSDKConfig{}, ErrJSSDKDisabled
	}
	signedURL, err := service.validateURL(rawURL)
	if err != nil {
		return JSSDKConfig{}, err
	}
	configTicket, err := service.configCache.get(ctx)
	if err != nil {
		return JSSDKConfig{}, err
	}
	agentTicket, err := service.agentCache.get(ctx)
	if err != nil {
		return JSSDKConfig{}, err
	}
	config, err := service.sign("config", configTicket, signedURL)
	if err != nil {
		return JSSDKConfig{}, err
	}
	agentConfig, err := service.sign("agent_config", agentTicket, signedURL)
	if err != nil {
		return JSSDKConfig{}, err
	}
	return JSSDKConfig{CorpID: service.corpID, AgentID: service.agentID, Config: config, AgentConfig: agentConfig}, nil
}

func (service *JSSDKService) sign(signatureType string, ticket AgentConfigTicket, signedURL string) (AgentConfigSignature, error) {
	now := service.now().UTC()
	if now.IsZero() || now.Unix() <= 0 || !ticket.ExpiresAt.After(now) {
		return AgentConfigSignature{}, ErrJSSDKUnavailable
	}
	nonceBytes := make([]byte, agentConfigNonceBytes)
	if _, err := io.ReadFull(service.random, nonceBytes); err != nil {
		return AgentConfigSignature{}, ErrJSSDKUnavailable
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	timestamp := now.Unix()
	canonical := canonicalJSSDKString(ticket.Value, nonce, timestamp, signedURL)
	// WeCom requires SHA-1 for this canonical JSSDK protocol string.
	digest := sha1.Sum([]byte(canonical)) // #nosec G401 -- provider-mandated protocol digest, not a password hash.
	return AgentConfigSignature{
		SignatureType: signatureType, CorpID: service.corpID, AgentID: service.agentID, Nonce: nonce, Timestamp: timestamp,
		Signature: hex.EncodeToString(digest[:]), URL: signedURL, TicketExpiresAt: ticket.ExpiresAt.UTC(),
	}, nil
}

func (service *JSSDKService) validateURL(raw string) (string, error) {
	if len(raw) < 1 || len(raw) > 4096 || strings.TrimSpace(raw) != raw || !utf8.ValidString(raw) || strings.ContainsRune(raw, '\\') {
		return "", ErrJSSDKInvalid
	}
	for _, character := range raw {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return "", ErrJSSDKInvalid
		}
	}
	if fragment := strings.IndexByte(raw, '#'); fragment >= 0 {
		raw = raw[:fragment]
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", ErrJSSDKInvalid
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", ErrJSSDKInvalid
	}
	host := strings.ToLower(parsed.Hostname())
	if _, ok := service.allowedHosts[host]; !ok {
		return "", ErrJSSDKInvalid
	}
	return raw, nil
}

func canonicalJSSDKString(ticket, nonce string, timestamp int64, signedURL string) string {
	return "jsapi_ticket=" + ticket + "&noncestr=" + nonce + "&timestamp=" + strconv.FormatInt(timestamp, 10) + "&url=" + signedURL
}

type agentConfigTicketCache struct {
	provider      AgentConfigTicketProvider
	now           func() time.Time
	refreshBefore time.Duration

	mu       sync.Mutex
	ticket   AgentConfigTicket
	inFlight *agentConfigTicketFlight
}

type agentConfigTicketFlight struct {
	done   chan struct{}
	ticket AgentConfigTicket
	err    error
}

func newAgentConfigTicketCache(provider AgentConfigTicketProvider, now func() time.Time, refreshBefore time.Duration) *agentConfigTicketCache {
	return &agentConfigTicketCache{provider: agentTicketFetcher{provider}, now: now, refreshBefore: refreshBefore}
}

func newConfigTicketCache(provider AgentConfigTicketProvider, now func() time.Time, refreshBefore time.Duration) *agentConfigTicketCache {
	return &agentConfigTicketCache{provider: configTicketFetcher{provider}, now: now, refreshBefore: refreshBefore}
}

type agentTicketFetcher struct{ AgentConfigTicketProvider }

func (fetcher agentTicketFetcher) FetchAgentConfigTicket(ctx context.Context) (AgentConfigTicket, error) {
	return fetcher.AgentConfigTicketProvider.FetchAgentConfigTicket(ctx)
}

type configTicketFetcher struct{ AgentConfigTicketProvider }

func (fetcher configTicketFetcher) FetchAgentConfigTicket(ctx context.Context) (AgentConfigTicket, error) {
	return fetcher.AgentConfigTicketProvider.FetchConfigTicket(ctx)
}

func (cache *agentConfigTicketCache) get(ctx context.Context) (AgentConfigTicket, error) {
	if cache == nil || ctx == nil {
		return AgentConfigTicket{}, ErrJSSDKUnavailable
	}
	now := cache.now().UTC()
	cache.mu.Lock()
	if validCachedAgentConfigTicket(cache.ticket, now, cache.refreshBefore) {
		ticket := cache.ticket
		cache.mu.Unlock()
		return ticket, nil
	}
	if cache.inFlight != nil {
		flight := cache.inFlight
		cache.mu.Unlock()
		select {
		case <-flight.done:
			if flight.err == nil && !flight.ticket.ExpiresAt.After(cache.now().UTC()) {
				return AgentConfigTicket{}, ErrJSSDKUnavailable
			}
			return flight.ticket, flight.err
		case <-ctx.Done():
			return AgentConfigTicket{}, errors.Join(ErrJSSDKUnavailable, ctx.Err())
		}
	}
	flight := &agentConfigTicketFlight{done: make(chan struct{})}
	cache.inFlight = flight
	cache.mu.Unlock()

	ticket, err := cache.provider.FetchAgentConfigTicket(ctx)
	if err != nil || !validFetchedAgentConfigTicket(ticket, cache.now().UTC()) {
		flight.err = ErrJSSDKUnavailable
	} else {
		flight.ticket = AgentConfigTicket{Value: ticket.Value, ExpiresAt: ticket.ExpiresAt.UTC()}
	}
	cache.mu.Lock()
	if flight.err == nil {
		cache.ticket = flight.ticket
	}
	cache.inFlight = nil
	close(flight.done)
	cache.mu.Unlock()
	return flight.ticket, flight.err
}

func validCachedAgentConfigTicket(ticket AgentConfigTicket, now time.Time, refreshBefore time.Duration) bool {
	return validAgentConfigTicketValue(ticket.Value) && ticket.ExpiresAt.After(now.Add(refreshBefore))
}

func validFetchedAgentConfigTicket(ticket AgentConfigTicket, now time.Time) bool {
	return validAgentConfigTicketValue(ticket.Value) && ticket.ExpiresAt.After(now.Add(time.Minute)) && !ticket.ExpiresAt.After(now.Add(24*time.Hour))
}

func validAgentConfigTicketValue(value string) bool {
	if len(value) < 1 || len(value) > 2048 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func nilJSSDKDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface) && reflected.IsNil()
}

func normalizeJSSDKHosts(values []string) (map[string]struct{}, bool) {
	if len(values) < 1 || len(values) > 16 {
		return nil, false
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		host := strings.ToLower(value)
		if !validJSSDKHost(host) {
			return nil, false
		}
		if _, duplicate := result[host]; duplicate {
			return nil, false
		}
		result[host] = struct{}{}
	}
	return result, true
}

func validJSSDKHost(value string) bool {
	if len(value) < 1 || len(value) > 253 || strings.TrimSpace(value) != value || strings.HasSuffix(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return false
			}
		}
	}
	return true
}
