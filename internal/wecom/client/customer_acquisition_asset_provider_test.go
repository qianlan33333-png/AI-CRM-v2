package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestCustomerAcquisitionAssetProviderUsesOpaqueCorrelationAndRejectsLinkConflict(t *testing.T) {
	var qrState string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/externalcontact/add_contact_way":
			var body struct {
				State string `json:"state"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			qrState = body.State
			_, _ = writer.Write([]byte(`{"errcode":0,"config_id":"config-safe","qr_code":"https://work.weixin.qq.com/q/config-safe"}`))
		case "/cgi-bin/externalcontact/customer_acquisition/create_link":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if _, leaked := body["customer_channel"]; leaked {
				t.Fatal("customer_channel must not be sent in create_link JSON")
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"link_id":"link-safe","url":"https://work.weixin.qq.com/ca/link-safe?customer_channel=conflict"}`))
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), staticTokenProvider{token: AccessToken{value: "token-safe"}})
	provider, err := NewCustomerAcquisitionAssetProvider(client)
	if err != nil {
		t.Fatal(err)
	}
	request := acquisitionAssetProviderRequest(contactport.AcquisitionAssetQRCode)
	if _, err = provider.PublishAcquisitionAsset(context.Background(), request); err != nil || qrState != request.CorrelationKey {
		t.Fatalf("state=%q err=%v", qrState, err)
	}
	request.Kind = contactport.AcquisitionAssetLink
	request.Snapshot.SceneValue = ""
	result, err := provider.PublishAcquisitionAsset(context.Background(), request)
	if !errors.Is(err, ErrWriteOutcomeUnknown) || !result.BusinessEndpointDispatched || !result.RealExternalCallExecuted || result.Outcome == contactport.AcquisitionAssetProviderFinalFailed {
		t.Fatalf("conflict result=%+v err=%v", result, err)
	}
}

func TestCustomerAcquisitionAssetProviderTokenFailureIsNotBusinessDispatch(t *testing.T) {
	var httpCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { httpCalls.Add(1) }))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), failingAcquisitionTokenProvider{})
	provider, err := NewCustomerAcquisitionAssetProvider(client)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.PublishAcquisitionAsset(context.Background(), acquisitionAssetProviderRequest(contactport.AcquisitionAssetQRCode))
	if !errors.Is(err, ErrBusinessWriteNotDispatched) || result.BusinessEndpointDispatched || result.RealExternalCallExecuted || result.Outcome == contactport.AcquisitionAssetProviderFinalFailed || result.ReceiptDigest != ([32]byte{}) || httpCalls.Load() != 0 {
		t.Fatalf("result=%+v http_calls=%d err=%v", result, httpCalls.Load(), err)
	}
}

func TestCustomerAcquisitionLinkReceiptUsesFinalCorrelatedURL(t *testing.T) {
	key := "ch02_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	base := "https://work.weixin.qq.com/ca/link-safe?source=provider"
	finalURL, err := appendCustomerChannel(base, key)
	if err != nil || finalURL != "https://work.weixin.qq.com/ca/link-safe?customer_channel="+key+"&source=provider" {
		t.Fatalf("final_url=%q err=%v", finalURL, err)
	}
	requestDigest := [32]byte{1}
	finalReceipt := acquisitionAssetProviderSuccess(requestDigest, "link-safe", finalURL).ReceiptDigest
	baseReceipt := acquisitionAssetProviderSuccess(requestDigest, "link-safe", base).ReceiptDigest
	if finalReceipt == baseReceipt || finalReceipt == ([32]byte{}) {
		t.Fatalf("final receipt did not bind correlated URL")
	}
}

type failingAcquisitionTokenProvider struct{}

func (failingAcquisitionTokenProvider) Token(context.Context) (AccessToken, error) {
	return AccessToken{}, apiError(40013, "token grant rejected")
}

func TestCustomerAcquisitionAssetProviderPublishesOnlyFrozenContactShapes(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		switch request.URL.Path {
		case "/cgi-bin/externalcontact/add_contact_way":
			_, _ = writer.Write([]byte(`{"errcode":0,"config_id":"config-safe","qr_code":"https://work.weixin.qq.com/q/config-safe"}`))
		case "/cgi-bin/externalcontact/customer_acquisition/create_link":
			_, _ = writer.Write([]byte(`{"errcode":0,"link_id":"link-safe","url":"https://work.weixin.qq.com/ca/link-safe"}`))
		default:
			t.Fatalf("unexpected provider path %q", request.URL.Path)
		}
	}))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), staticTokenProvider{token: AccessToken{value: "token-safe"}})
	provider, err := NewCustomerAcquisitionAssetProvider(client)
	if err != nil {
		t.Fatal(err)
	}

	qrcode := acquisitionAssetProviderRequest(contactport.AcquisitionAssetQRCode)
	result, err := provider.PublishAcquisitionAsset(context.Background(), qrcode)
	if err != nil || result.Outcome != contactport.AcquisitionAssetProviderExecuted || result.ReceiptDigest == ([32]byte{}) ||
		result.AssetReferenceDigest == ([32]byte{}) || result.ProviderAssetID != "config-safe" ||
		result.AssetURL != "https://work.weixin.qq.com/q/config-safe" || !result.RealExternalCallExecuted {
		t.Fatalf("qrcode result=%+v err=%v", result, err)
	}
	link := acquisitionAssetProviderRequest(contactport.AcquisitionAssetLink)
	link.Snapshot.SceneValue = ""
	result, err = provider.PublishAcquisitionAsset(context.Background(), link)
	if err != nil || result.Outcome != contactport.AcquisitionAssetProviderExecuted || result.ReceiptDigest == ([32]byte{}) ||
		result.AssetReferenceDigest == ([32]byte{}) || result.ProviderAssetID != "link-safe" ||
		result.AssetURL != "https://work.weixin.qq.com/ca/link-safe?customer_channel="+link.CorrelationKey ||
		!result.RealExternalCallExecuted || calls.Load() != 2 {
		t.Fatalf("link result=%+v calls=%d err=%v", result, calls.Load(), err)
	}
}

func TestCustomerAcquisitionAssetProviderPreservesKnownAndUnknownWriteOutcomes(t *testing.T) {
	for _, test := range []struct {
		name        string
		response    string
		status      int
		wantOutcome contactport.AcquisitionAssetProviderOutcome
		wantReal    bool
	}{
		{name: "provider rejection", response: `{"errcode":40058,"errmsg":"rejected"}`, status: http.StatusOK, wantOutcome: contactport.AcquisitionAssetProviderFinalFailed, wantReal: true},
		{name: "malformed response", response: `not-json`, status: http.StatusOK, wantOutcome: contactport.AcquisitionAssetProviderOutcomeUnknown},
		{name: "gateway failure", response: `{}`, status: http.StatusBadGateway, wantOutcome: contactport.AcquisitionAssetProviderOutcomeUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.response))
			}))
			defer server.Close()
			client := testCustomerAcquisitionClient(t, server.URL, server.Client(), staticTokenProvider{token: AccessToken{value: "token-safe"}})
			provider, err := NewCustomerAcquisitionAssetProvider(client)
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.PublishAcquisitionAsset(context.Background(), acquisitionAssetProviderRequest(contactport.AcquisitionAssetQRCode))
			if err != nil || result.Outcome != test.wantOutcome || result.ReceiptDigest == ([32]byte{}) || result.RealExternalCallExecuted != test.wantReal || result.AssetReferenceDigest != ([32]byte{}) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestCustomerAcquisitionAssetProviderRejectsBeforeTokenOrHTTP(t *testing.T) {
	var tokenCalls atomic.Int32
	providerClient, err := NewCustomerAcquisitionClient(CustomerAcquisitionClientConfig{
		BaseURL: "https://qyapi.weixin.qq.com", HTTPClient: http.DefaultClient,
		TokenProvider: acquisitionAssetTokenProvider{calls: &tokenCalls},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewCustomerAcquisitionAssetProvider(providerClient)
	if err != nil {
		t.Fatal(err)
	}
	invalid := acquisitionAssetProviderRequest(contactport.AcquisitionAssetQRCode)
	invalid.Snapshot.ChannelName = ""
	if _, err = provider.PublishAcquisitionAsset(context.Background(), invalid); !errors.Is(err, ErrInvalidConfig) || tokenCalls.Load() != 0 {
		t.Fatalf("invalid request err=%v token_calls=%d", err, tokenCalls.Load())
	}
	if provider, err = NewCustomerAcquisitionAssetProvider(nil); provider != nil || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil provider=%v err=%v", provider, err)
	}
}

func acquisitionAssetProviderRequest(kind contactport.AcquisitionAssetKind) contactport.AcquisitionAssetPublishRequest {
	return contactport.AcquisitionAssetPublishRequest{
		EffectID: "eer_41", CorpID: "corp-safe", CorrelationKey: "ch02_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", AssetVersion: 2, Supersedes: 1, Kind: kind,
		Snapshot: contactport.AcquisitionAssetSnapshot{
			ChannelID: 7, ChannelRevision: 9, ChannelCode: "channel-safe", ChannelName: "CH02 asset",
			ChannelStatus: "active", SceneValue: "scene-safe", AssigneeWeComUserIDs: []string{"staff-a"},
		},
		SnapshotDigest: [32]byte{1},
	}
}

type acquisitionAssetTokenProvider struct{ calls *atomic.Int32 }

func (provider acquisitionAssetTokenProvider) Token(context.Context) (AccessToken, error) {
	provider.calls.Add(1)
	return AccessToken{value: "must-not-be-requested"}, nil
}
