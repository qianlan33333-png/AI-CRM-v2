package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCustomerAcquisitionClientCreatesPrivateMessageTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/cgi-bin/externalcontact/add_msg_template" || request.URL.Query().Get("access_token") != "token-safe" {
			t.Fatalf("request=%s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"allow_select":false,"chat_type":"single","external_userid":["external-1"],"sender":"staff-1","text":{"content":"hello"}}`
		if string(body) != want {
			t.Fatalf("body=%s want=%s", body, want)
		}
		_, _ = writer.Write([]byte(`{"errcode":0,"msgid":"msg-1"}`))
	}))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), staticTokenProvider{token: AccessToken{value: "token-safe"}})

	result, err := client.CreatePrivateMessageTemplate(context.Background(), PrivateMessageTemplateRequest{Sender: "staff-1", ExternalUserID: "external-1", Text: "hello"})
	if err != nil || result.MessageID != "msg-1" {
		t.Fatalf("CreatePrivateMessageTemplate()=%+v err=%v", result, err)
	}
}

func TestCustomerAcquisitionClientPrivateMessageUnknownOutcomeIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = writer.Write([]byte(`{"errcode":0,"msgid":"msg-1"} {}`))
	}))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), staticTokenProvider{token: AccessToken{value: "token-safe"}})

	_, err := client.CreatePrivateMessageTemplate(context.Background(), PrivateMessageTemplateRequest{Sender: "staff-1", ExternalUserID: "external-1", Text: "hello"})
	if !errors.Is(err, ErrWriteOutcomeUnknown) || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestCustomerAcquisitionClientPrivateMessageRejectsInvalidInputBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := testCustomerAcquisitionClient(t, server.URL, server.Client(), staticTokenProvider{token: AccessToken{value: "token-safe"}})

	_, err := client.CreatePrivateMessageTemplate(context.Background(), PrivateMessageTemplateRequest{Sender: "staff-1", ExternalUserID: "external-1", Text: " "})
	if !errors.Is(err, ErrInvalidConfig) || calls.Load() != 0 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}
