package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentConfigUsesTwoDeterministicSignaturesAndStripsFragment(t *testing.T) {
	now := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	provider := &agentConfigProviderFake{
		configTicket: AgentConfigTicket{Value: "config-ticket-fixture", ExpiresAt: now.Add(2 * time.Hour)},
		agentTicket:  AgentConfigTicket{Value: "agent-ticket-fixture", ExpiresAt: now.Add(2 * time.Hour)},
	}
	service, err := NewJSSDKService(JSSDKServiceConfig{
		Enabled: true, CorpID: "corp-1", AgentID: 73, AllowedHosts: []string{"CRM.EXAMPLE.TEST"},
	}, provider, JSSDKOptions{Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x11}, 32))})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.AgentConfig(context.Background(), "https://crm.example.test/sidebar?tab=profile%2Fone#client-fragment")
	if err != nil {
		t.Fatal(err)
	}
	if result.CorpID != "corp-1" || result.AgentID != 73 || result.Config.SignatureType != "config" || result.AgentConfig.SignatureType != "agent_config" ||
		result.Config.Nonce != "EREREREREREREREREREREQ" || result.AgentConfig.Nonce != "EREREREREREREREREREREQ" || result.Config.Timestamp != 1787652000 ||
		result.Config.URL != "https://crm.example.test/sidebar?tab=profile%2Fone" || result.AgentConfig.URL != result.Config.URL ||
		!result.Config.TicketExpiresAt.Equal(now.Add(2*time.Hour)) || !result.AgentConfig.TicketExpiresAt.Equal(now.Add(2*time.Hour)) || provider.configCalls.Load() != 1 || provider.agentCalls.Load() != 1 {
		t.Fatalf("AgentConfig()=%+v provider_calls=%d/%d", result, provider.configCalls.Load(), provider.agentCalls.Load())
	}
	wantCanonical := "jsapi_ticket=config-ticket-fixture&noncestr=EREREREREREREREREREREQ&timestamp=1787652000&url=https://crm.example.test/sidebar?tab=profile%2Fone"
	if canonical := canonicalJSSDKString("config-ticket-fixture", result.Config.Nonce, result.Config.Timestamp, result.Config.URL); canonical != wantCanonical {
		t.Fatalf("canonical string = %q", canonical)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("ticket-fixture")) {
		t.Fatalf("Config result leaked provider ticket: %s", encoded)
	}
}

func TestAgentConfigRejectsUnsafeOrUnallowedURLsBeforeProvider(t *testing.T) {
	provider := &agentConfigProviderFake{}
	service := newJSSDKTestService(t, provider, time.Now)
	for _, raw := range []string{
		"", "/sidebar", "http://crm.example.test/sidebar", "https://evil.example/sidebar",
		"https://crm.example.test.evil/sidebar", "https://user@crm.example.test/sidebar",
		"https://crm.example.test:444/sidebar", "https://crm.example.test/back\\slash",
		" https://crm.example.test/sidebar", "https://crm.example.test/side bar",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := service.AgentConfig(context.Background(), raw); !errors.Is(err, ErrJSSDKInvalid) {
				t.Fatalf("AgentConfig(%q) error = %v", raw, err)
			}
		})
	}
	if provider.configCalls.Load() != 0 || provider.agentCalls.Load() != 0 {
		t.Fatalf("unsafe URL provider calls = %d/%d", provider.configCalls.Load(), provider.agentCalls.Load())
	}
}

func TestJSSDKDisabledNeverCallsProvider(t *testing.T) {
	service, err := NewJSSDKService(JSSDKServiceConfig{}, nil, JSSDKOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.AgentConfig(context.Background(), "https://crm.example.test/sidebar"); !errors.Is(err, ErrJSSDKDisabled) {
		t.Fatalf("disabled AgentConfig() error = %v", err)
	}
}

func TestAgentConfigTicketCacheCoalescesConcurrentMisses(t *testing.T) {
	now := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	provider := &blockingAgentConfigProvider{
		started: make(chan struct{}), release: make(chan struct{}),
		ticket: AgentConfigTicket{Value: "coalesced-ticket", ExpiresAt: now.Add(2 * time.Hour)},
	}
	cache := newAgentConfigTicketCache(provider, func() time.Time { return now }, 5*time.Minute)
	const callers = 32
	results := make(chan AgentConfigTicket, callers)
	errorsChannel := make(chan error, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			ticket, err := cache.get(context.Background())
			results <- ticket
			errorsChannel <- err
		}()
	}
	<-provider.started
	close(provider.release)
	wait.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	for ticket := range results {
		if ticket.Value != "coalesced-ticket" || !ticket.ExpiresAt.Equal(now.Add(2*time.Hour)) {
			t.Fatalf("coalesced ticket = %+v", ticket)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d", provider.calls.Load())
	}
}

func TestAgentConfigTicketCacheRefreshesEarlyAndFailsClosed(t *testing.T) {
	start := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	now := start
	provider := &sequenceAgentConfigProvider{results: []agentConfigProviderResult{
		{ticket: AgentConfigTicket{Value: "ticket-1", ExpiresAt: start.Add(10 * time.Minute)}},
		{err: errors.New("provider unavailable")},
		{ticket: AgentConfigTicket{Value: "ticket-2", ExpiresAt: start.Add(2 * time.Hour)}},
	}}
	cache := newAgentConfigTicketCache(provider, func() time.Time { return now }, 2*time.Minute)
	first, err := cache.get(context.Background())
	if err != nil || first.Value != "ticket-1" {
		t.Fatalf("first ticket/error = %+v/%v", first, err)
	}
	now = start.Add(7 * time.Minute)
	stillCached, err := cache.get(context.Background())
	if err != nil || stillCached.Value != "ticket-1" || provider.calls != 1 {
		t.Fatalf("cached ticket/error/calls = %+v/%v/%d", stillCached, err, provider.calls)
	}
	now = start.Add(8 * time.Minute)
	if _, err = cache.get(context.Background()); !errors.Is(err, ErrJSSDKUnavailable) || provider.calls != 2 {
		t.Fatalf("failed refresh error/calls = %v/%d", err, provider.calls)
	}
	refreshed, err := cache.get(context.Background())
	if err != nil || refreshed.Value != "ticket-2" || provider.calls != 3 {
		t.Fatalf("refreshed ticket/error/calls = %+v/%v/%d", refreshed, err, provider.calls)
	}

	expired := &agentConfigProviderFake{ticket: AgentConfigTicket{Value: "expired", ExpiresAt: now}}
	if _, err = newAgentConfigTicketCache(expired, func() time.Time { return now }, time.Minute).get(context.Background()); !errors.Is(err, ErrJSSDKUnavailable) {
		t.Fatalf("expired provider ticket error = %v", err)
	}
	clockCalls := atomic.Int32{}
	tooSlow := &agentConfigProviderFake{ticket: AgentConfigTicket{Value: "expired-during-fetch", ExpiresAt: now.Add(90 * time.Second)}}
	if _, err = newAgentConfigTicketCache(tooSlow, func() time.Time {
		if clockCalls.Add(1) == 1 {
			return now
		}
		return now.Add(2 * time.Minute)
	}, time.Minute).get(context.Background()); !errors.Is(err, ErrJSSDKUnavailable) {
		t.Fatalf("ticket expired during fetch error = %v", err)
	}
}

func TestJSSDKEnabledConfigurationFailsClosed(t *testing.T) {
	provider := &agentConfigProviderFake{}
	for _, config := range []JSSDKServiceConfig{
		{Enabled: true, CorpID: "corp-1", AgentID: 1},
		{Enabled: true, CorpID: "corp 1", AgentID: 1, AllowedHosts: []string{"crm.example.test"}},
		{Enabled: true, CorpID: "corp-1", AllowedHosts: []string{"crm.example.test"}},
		{Enabled: true, CorpID: "corp-1", AgentID: 1, AllowedHosts: []string{"crm.example.test", "CRM.EXAMPLE.TEST"}},
		{Enabled: true, CorpID: "corp-1", AgentID: 1, AllowedHosts: []string{"crm.example.test:443"}},
	} {
		if service, err := NewJSSDKService(config, provider, JSSDKOptions{}); !errors.Is(err, ErrJSSDKUnavailable) || service != nil {
			t.Fatalf("config=%+v service/error=%v/%v", config, service, err)
		}
	}
	if service, err := NewJSSDKService(JSSDKServiceConfig{Enabled: true, CorpID: "corp-1", AgentID: 1, AllowedHosts: []string{"crm.example.test"}}, nil, JSSDKOptions{}); !errors.Is(err, ErrJSSDKUnavailable) || service != nil {
		t.Fatalf("nil provider service/error=%v/%v", service, err)
	}
	var typedNil *agentConfigProviderFake
	if service, err := NewJSSDKService(JSSDKServiceConfig{Enabled: true, CorpID: "corp-1", AgentID: 1, AllowedHosts: []string{"crm.example.test"}}, typedNil, JSSDKOptions{}); !errors.Is(err, ErrJSSDKUnavailable) || service != nil {
		t.Fatalf("typed nil provider service/error=%v/%v", service, err)
	}
	var typedNilRandom *bytes.Reader
	if service, err := NewJSSDKService(JSSDKServiceConfig{Enabled: true, CorpID: "corp-1", AgentID: 1, AllowedHosts: []string{"crm.example.test"}}, provider, JSSDKOptions{Random: typedNilRandom}); !errors.Is(err, ErrJSSDKUnavailable) || service != nil {
		t.Fatalf("typed nil random service/error=%v/%v", service, err)
	}
}

type agentConfigProviderFake struct {
	ticket       AgentConfigTicket
	configTicket AgentConfigTicket
	agentTicket  AgentConfigTicket
	err          error
	configCalls  atomic.Int32
	agentCalls   atomic.Int32
}

func (provider *agentConfigProviderFake) FetchConfigTicket(context.Context) (AgentConfigTicket, error) {
	provider.configCalls.Add(1)
	if provider.configTicket.Value != "" {
		return provider.configTicket, provider.err
	}
	return provider.ticket, provider.err
}

func (provider *agentConfigProviderFake) FetchAgentConfigTicket(context.Context) (AgentConfigTicket, error) {
	provider.agentCalls.Add(1)
	if provider.agentTicket.Value != "" {
		return provider.agentTicket, provider.err
	}
	return provider.ticket, provider.err
}

type blockingAgentConfigProvider struct {
	started chan struct{}
	release chan struct{}
	ticket  AgentConfigTicket
	calls   atomic.Int32
}

func (provider *blockingAgentConfigProvider) FetchConfigTicket(context.Context) (AgentConfigTicket, error) {
	return provider.FetchAgentConfigTicket(context.Background())
}

func (provider *blockingAgentConfigProvider) FetchAgentConfigTicket(context.Context) (AgentConfigTicket, error) {
	if provider.calls.Add(1) == 1 {
		close(provider.started)
	}
	<-provider.release
	return provider.ticket, nil
}

type agentConfigProviderResult struct {
	ticket AgentConfigTicket
	err    error
}

type sequenceAgentConfigProvider struct {
	results []agentConfigProviderResult
	calls   int
}

func (provider *sequenceAgentConfigProvider) FetchConfigTicket(ctx context.Context) (AgentConfigTicket, error) {
	return provider.FetchAgentConfigTicket(ctx)
}

func (provider *sequenceAgentConfigProvider) FetchAgentConfigTicket(context.Context) (AgentConfigTicket, error) {
	result := provider.results[provider.calls]
	provider.calls++
	return result.ticket, result.err
}

func newJSSDKTestService(t *testing.T, provider AgentConfigTicketProvider, clock func() time.Time) *JSSDKService {
	t.Helper()
	service, err := NewJSSDKService(JSSDKServiceConfig{
		Enabled: true, CorpID: "corp-1", AgentID: 73, AllowedHosts: []string{"crm.example.test"},
	}, provider, JSSDKOptions{Clock: clock, Random: bytes.NewReader(bytes.Repeat([]byte{1}, 1024))})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
