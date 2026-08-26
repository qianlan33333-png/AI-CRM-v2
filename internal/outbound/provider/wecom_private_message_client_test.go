package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWeComPrivateMessageClientCreatesExactTemplate(t *testing.T) {
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
	client := testWeComPrivateMessageClient(t, server, func(context.Context) (string, error) { return "token-safe", nil })

	result, err := client.CreatePrivateMessageTemplate(context.Background(), privateMessageTemplateRequest{Sender: "staff-1", ExternalUserID: "external-1", Text: "hello"})
	if err != nil || result.MessageID != "msg-1" {
		t.Fatalf("CreatePrivateMessageTemplate()=%+v err=%v", result, err)
	}
}

func TestWeComPrivateMessageClientUnknownOutcomeIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = writer.Write([]byte(`{"errcode":0,"msgid":"msg-1"} {}`))
	}))
	defer server.Close()
	client := testWeComPrivateMessageClient(t, server, func(context.Context) (string, error) { return "token-safe", nil })

	_, err := client.CreatePrivateMessageTemplate(context.Background(), privateMessageTemplateRequest{Sender: "staff-1", ExternalUserID: "external-1", Text: "hello"})
	if !errors.Is(err, errWeComPrivateMessageOutcomeUnknown) || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestWeComPrivateMessageClientFailsBeforeDispatchWithoutToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := testWeComPrivateMessageClient(t, server, func(context.Context) (string, error) { return "", errors.New("token unavailable") })

	_, err := client.CreatePrivateMessageTemplate(context.Background(), privateMessageTemplateRequest{Sender: "staff-1", ExternalUserID: "external-1", Text: "hello"})
	if !errors.Is(err, errWeComPrivateMessageNotDispatched) || calls.Load() != 0 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func testWeComPrivateMessageClient(t *testing.T, server *httptest.Server, token func(context.Context) (string, error)) *weComPrivateMessageClient {
	t.Helper()
	client, err := NewWeComPrivateMessageClient(WeComPrivateMessageClientConfig{BaseURL: server.URL, HTTPClient: server.Client(), Token: token})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
