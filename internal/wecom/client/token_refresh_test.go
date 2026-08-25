package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCachingTokenProviderRefreshesAndInvalidatesOldCache(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch calls.Add(1) {
		case 1:
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"old-token","expires_in":600}`))
		case 2:
			_, _ = writer.Write([]byte(`{"errcode":40001,"errmsg":"corp-secret-must-not-leak"}`))
		default:
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"fresh-token","expires_in":600}`))
		}
	}))
	defer server.Close()
	provider := testProvider(t, server.URL, server.Client())
	if token, err := provider.Token(context.Background()); err != nil || token.Value() != "old-token" {
		t.Fatalf("Token()=%v err=%v", token, err)
	}
	if _, err := provider.RefreshToken(context.Background()); !errors.Is(err, ErrUpstream) || strings.Contains(err.Error(), "corp-secret-must-not-leak") {
		t.Fatalf("RefreshToken() err=%v", err)
	}
	if token, err := provider.Token(context.Background()); err != nil || token.Value() != "fresh-token" || calls.Load() != 3 {
		t.Fatalf("Token after failed refresh=%v err=%v calls=%d", token, err, calls.Load())
	}
}

func TestCachingTokenProviderRefreshIsSingleFlight(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) != 1 {
			t.Fatalf("unexpected concurrent gettoken request")
		}
		close(started)
		<-release
		_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"fresh-token","expires_in":600}`))
	}))
	defer server.Close()
	provider := testProvider(t, server.URL, server.Client())

	const workers = 12
	ready := make(chan struct{}, workers)
	goSignal := make(chan struct{})
	var group sync.WaitGroup
	errorsByWorker := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			ready <- struct{}{}
			<-goSignal
			token, err := provider.RefreshToken(context.Background())
			if err != nil || token.Value() != "fresh-token" {
				errorsByWorker <- err
			}
		}()
	}
	for range workers {
		<-ready
	}
	close(goSignal)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first token refresh did not start")
	}
	// Let all callers observe the in-flight grant before releasing it.
	time.Sleep(20 * time.Millisecond)
	close(release)
	group.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		t.Fatalf("RefreshToken() err=%v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("gettoken calls=%d, want one shared refresh", calls.Load())
	}
}

func TestCachingTokenProviderRefreshTransportErrorRedactsCredentials(t *testing.T) {
	secret := "corp-secret-must-not-leak"
	credentials, err := NewCredentials("wx-corp", secret)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewTokenProvider(TokenProviderConfig{
		BaseURL:     "https://qyapi.weixin.qq.com",
		Credentials: credentials,
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Get", URL: "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpsecret=" + secret, Err: errors.New("disconnect")}
		})},
		Now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.RefreshToken(context.Background())
	if !errors.Is(err, ErrTransport) || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "corpsecret") {
		t.Fatalf("RefreshToken() err=%v", err)
	}
}
