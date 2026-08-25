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

func TestJSSDKConfigUsesDeterministicCanonicalSignatureAndStripsFragment(t *testing.T) {
	now := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	provider := &jssdkProviderFake{ticket: JSSDKTicket{Value: "ticket-fixture", ExpiresAt: now.Add(2 * time.Hour)}}
	service, err := NewJSSDKService(JSSDKServiceConfig{
		Enabled: true, CorpID: "corp-1", AgentID: 73, AllowedHosts: []string{"CRM.EXAMPLE.TEST"},
	}, provider, JSSDKOptions{Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x11}, 16))})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Config(context.Background(), "https://crm.example.test/sidebar?tab=profile%2Fone#client-fragment")
	if err != nil {
		t.Fatal(err)
	}
	if result.CorpID != "corp-1" || result.AgentID != 73 || result.Nonce != "EREREREREREREREREREREQ" || result.Timestamp != 1787652000 ||
		result.URL != "https://crm.example.test/sidebar?tab=profile%2Fone" || result.Signature != "0296c52a3932efcf6ed973f72cd247830ed8bf31" ||
		!result.TicketExpiresAt.Equal(now.Add(2*time.Hour)) || provider.calls.Load() != 1 {
		t.Fatalf("Config()=%+v provider_calls=%d", result, provider.calls.Load())
	}
	wantCanonical := "jsapi_ticket=ticket-fixture&noncestr=EREREREREREREREREREREQ&timestamp=1787652000&url=https://crm.example.test/sidebar?tab=profile%2Fone"
	if canonical := canonicalJSSDKString("ticket-fixture", result.Nonce, result.Timestamp, result.URL); canonical != wantCanonical {
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

func TestJSSDKConfigRejectsUnsafeOrUnallowedURLsBeforeProvider(t *testing.T) {
	provider := &jssdkProviderFake{}
	service := newJSSDKTestService(t, provider, time.Now)
	for _, raw := range []string{
		"", "/sidebar", "http://crm.example.test/sidebar", "https://evil.example/sidebar",
		"https://crm.example.test.evil/sidebar", "https://user@crm.example.test/sidebar",
		"https://crm.example.test:444/sidebar", "https://crm.example.test/back\\slash",
		" https://crm.example.test/sidebar", "https://crm.example.test/side bar",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := service.Config(context.Background(), raw); !errors.Is(err, ErrJSSDKInvalid) {
				t.Fatalf("Config(%q) error = %v", raw, err)
			}
		})
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("unsafe URL provider calls = %d", provider.calls.Load())
	}
}

func TestJSSDKDisabledNeverCallsProvider(t *testing.T) {
	service, err := NewJSSDKService(JSSDKServiceConfig{}, nil, JSSDKOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Config(context.Background(), "https://crm.example.test/sidebar"); !errors.Is(err, ErrJSSDKDisabled) {
		t.Fatalf("disabled Config() error = %v", err)
	}
}

func TestJSSDKTicketCacheCoalescesConcurrentMisses(t *testing.T) {
	now := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	provider := &blockingJSSDKProvider{
		started: make(chan struct{}), release: make(chan struct{}),
		ticket: JSSDKTicket{Value: "coalesced-ticket", ExpiresAt: now.Add(2 * time.Hour)},
	}
	cache := newJSSDKTicketCache(provider, func() time.Time { return now }, 5*time.Minute)
	const callers = 32
	results := make(chan JSSDKTicket, callers)
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

func TestJSSDKTicketCacheRefreshesEarlyAndFailsClosed(t *testing.T) {
	start := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	now := start
	provider := &sequenceJSSDKProvider{results: []jssdkProviderResult{
		{ticket: JSSDKTicket{Value: "ticket-1", ExpiresAt: start.Add(10 * time.Minute)}},
		{err: errors.New("provider unavailable")},
		{ticket: JSSDKTicket{Value: "ticket-2", ExpiresAt: start.Add(2 * time.Hour)}},
	}}
	cache := newJSSDKTicketCache(provider, func() time.Time { return now }, 2*time.Minute)
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

	expired := &jssdkProviderFake{ticket: JSSDKTicket{Value: "expired", ExpiresAt: now}}
	if _, err = newJSSDKTicketCache(expired, func() time.Time { return now }, time.Minute).get(context.Background()); !errors.Is(err, ErrJSSDKUnavailable) {
		t.Fatalf("expired provider ticket error = %v", err)
	}
	clockCalls := atomic.Int32{}
	tooSlow := &jssdkProviderFake{ticket: JSSDKTicket{Value: "expired-during-fetch", ExpiresAt: now.Add(90 * time.Second)}}
	if _, err = newJSSDKTicketCache(tooSlow, func() time.Time {
		if clockCalls.Add(1) == 1 {
			return now
		}
		return now.Add(2 * time.Minute)
	}, time.Minute).get(context.Background()); !errors.Is(err, ErrJSSDKUnavailable) {
		t.Fatalf("ticket expired during fetch error = %v", err)
	}
}

func TestJSSDKEnabledConfigurationFailsClosed(t *testing.T) {
	provider := &jssdkProviderFake{}
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
	var typedNil *jssdkProviderFake
	if service, err := NewJSSDKService(JSSDKServiceConfig{Enabled: true, CorpID: "corp-1", AgentID: 1, AllowedHosts: []string{"crm.example.test"}}, typedNil, JSSDKOptions{}); !errors.Is(err, ErrJSSDKUnavailable) || service != nil {
		t.Fatalf("typed nil provider service/error=%v/%v", service, err)
	}
	var typedNilRandom *bytes.Reader
	if service, err := NewJSSDKService(JSSDKServiceConfig{Enabled: true, CorpID: "corp-1", AgentID: 1, AllowedHosts: []string{"crm.example.test"}}, provider, JSSDKOptions{Random: typedNilRandom}); !errors.Is(err, ErrJSSDKUnavailable) || service != nil {
		t.Fatalf("typed nil random service/error=%v/%v", service, err)
	}
}

type jssdkProviderFake struct {
	ticket JSSDKTicket
	err    error
	calls  atomic.Int32
}

func (provider *jssdkProviderFake) FetchJSSDKTicket(context.Context) (JSSDKTicket, error) {
	provider.calls.Add(1)
	return provider.ticket, provider.err
}

type blockingJSSDKProvider struct {
	started chan struct{}
	release chan struct{}
	ticket  JSSDKTicket
	calls   atomic.Int32
}

func (provider *blockingJSSDKProvider) FetchJSSDKTicket(context.Context) (JSSDKTicket, error) {
	if provider.calls.Add(1) == 1 {
		close(provider.started)
	}
	<-provider.release
	return provider.ticket, nil
}

type jssdkProviderResult struct {
	ticket JSSDKTicket
	err    error
}

type sequenceJSSDKProvider struct {
	results []jssdkProviderResult
	calls   int
}

func (provider *sequenceJSSDKProvider) FetchJSSDKTicket(context.Context) (JSSDKTicket, error) {
	result := provider.results[provider.calls]
	provider.calls++
	return result.ticket, result.err
}

func newJSSDKTestService(t *testing.T, provider JSSDKTicketProvider, clock func() time.Time) *JSSDKService {
	t.Helper()
	service, err := NewJSSDKService(JSSDKServiceConfig{
		Enabled: true, CorpID: "corp-1", AgentID: 73, AllowedHosts: []string{"crm.example.test"},
	}, provider, JSSDKOptions{Clock: clock, Random: bytes.NewReader(bytes.Repeat([]byte{1}, 1024))})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
