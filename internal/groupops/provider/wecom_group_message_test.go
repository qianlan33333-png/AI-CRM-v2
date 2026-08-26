package groupopsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

func TestWeComGroupMessageClientUsesGroupProtocolAndRefreshesToken(t *testing.T) {
	var grants, createCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			grants++
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-` + string(rune('0'+grants)) + `","expires_in":7200}`))
		case "/cgi-bin/externalcontact/add_msg_template":
			createCalls++
			if request.Method != http.MethodPost || request.URL.Query().Get("access_token") == "" {
				t.Fatalf("create request=%s", request.URL.String())
			}
			var body map[string]any
			if json.NewDecoder(request.Body).Decode(&body) != nil || body["chat_type"] != "group" || body["sender"] != "staff-1" || body["allow_select"] != false {
				t.Fatalf("create body=%+v", body)
			}
			chatIDs, ok := body["chat_id_list"].([]any)
			if !ok || len(chatIDs) != 1 || chatIDs[0] != "chat-1" || body["external_userid"] != nil {
				t.Fatalf("unexpected group target=%+v", body)
			}
			if createCalls == 1 {
				_, _ = writer.Write([]byte(`{"errcode":42001,"errmsg":"expired"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"errmsg":"ok","msgid":"msg-1","fail_list":[]}`))
		case "/cgi-bin/externalcontact/get_groupmsg_task":
			assertGroupMessageQuery(t, request, "msg-1", "", "")
			_, _ = writer.Write([]byte(`{"errcode":0,"task_list":[{"userid":"staff-1","status":2}]}`))
		case "/cgi-bin/externalcontact/get_groupmsg_send_result":
			assertGroupMessageQuery(t, request, "msg-1", "staff-1", "")
			_, _ = writer.Write([]byte(`{"errcode":0,"send_list":[{"chat_id":"chat-1","userid":"staff-1","status":1}]}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	credentials, err := wecomclient.NewCredentials("corp-1", "secret-1")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{BaseURL: server.URL, Credentials: credentials, HTTPClient: server.Client(), Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewWeComGroupMessageClient(WeComGroupMessageClientConfig{BaseURL: server.URL, HTTPClient: server.Client(), Token: tokens})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateGroupMessageTask(context.Background(), GroupMessageCreateRequest{Sender: "staff-1", ChatIDs: []string{"chat-1"}, Text: "hello group"})
	if err != nil || created.MessageID != "msg-1" || created.Partial || grants != 2 || createCalls != 2 {
		t.Fatalf("created=%+v err=%v grants=%d calls=%d", created, err, grants, createCalls)
	}
	tasks, err := client.GetGroupMessageTask(context.Background(), "msg-1", "")
	if err != nil || len(tasks.Items) != 1 || tasks.Items[0].UserID != "staff-1" || tasks.Items[0].Status != 2 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	results, err := client.GetGroupMessageSendResult(context.Background(), "msg-1", "staff-1", "")
	if err != nil || len(results.Items) != 1 || results.Items[0].ChatID != "chat-1" || results.Items[0].Status != 1 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
}

func assertGroupMessageQuery(t *testing.T, request *http.Request, messageID, userID, cursor string) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.Query().Get("access_token") == "" {
		t.Fatalf("query request=%s", request.URL.String())
	}
	var body map[string]string
	if json.NewDecoder(request.Body).Decode(&body) != nil || body["msgid"] != messageID || body["userid"] != userID || body["cursor"] != cursor {
		t.Fatalf("query body=%+v", body)
	}
}

func TestWeComGroupMessageProviderClassifiesCreateTaskWithoutDeliveryClaim(t *testing.T) {
	request := groupMessageDispatchRequest()
	for _, test := range []struct {
		name    string
		created GroupMessageCreateResult
		err     error
		want    groupopsport.DispatchOutcome
		call    bool
		real    bool
	}{
		{name: "accepted task", created: GroupMessageCreateResult{MessageID: "msg-1"}, want: groupopsport.DispatchProviderAccepted, call: true, real: true},
		{name: "partial", created: GroupMessageCreateResult{MessageID: "msg-1", Partial: true}, want: groupopsport.DispatchProviderRejected, call: true, real: true},
		{name: "explicit upstream rejected", err: errWeComGroupUpstream, want: groupopsport.DispatchProviderRejected, call: true, real: true},
		{name: "transport unknown", err: errWeComGroupOutcomeUnknown, want: groupopsport.DispatchOutcomeUnknown, call: true, real: true},
		{name: "token unavailable", err: errWeComGroupNotDispatched, want: groupopsport.DispatchPreDispatchFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &groupMessageCreatorStub{result: test.created, err: test.err}
			provider, err := NewWeComGroupMessageProvider(client, "staff-1", groupMessageTargetResolver("chat-1"), &groupMessageReceiptWriterStub{})
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Dispatch(context.Background(), request)
			if err != nil || result.Outcome != test.want || result.BusinessCallDispatched != test.call || result.RealExternalCallExecuted != test.real || result.ReceiptDigest == "" || client.calls != 1 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, client.calls)
			}
		})
	}
	provider, err := NewWeComGroupMessageProvider(&groupMessageCreatorStub{}, "staff-1", groupMessageTargetResolver("chat-1"))
	if err != nil {
		t.Fatal(err)
	}
	bad := request
	bad.MaterialSnapshot = []byte(`{"schema_version":1,"node_kind":"message","reference":"unmapped-material"}`)
	result, err := provider.Dispatch(context.Background(), bad)
	if err != nil || result.Outcome != groupopsport.DispatchPreDispatchFailure || result.BusinessCallDispatched || result.RealExternalCallExecuted {
		t.Fatalf("bad material result=%+v err=%v", result, err)
	}
}

func TestWeComGroupMessageReconciliationVerifierFailsClosed(t *testing.T) {
	evidence := GroupMessageEvidence{ExecutionID: 11, ExternalEffectID: "eer_41", MessageID: "msg-1", SenderUserID: "staff-1", UserID: "staff-1", ChatID: "chat-1", TaskEvidenceDigest: groupMessageReceiptDigest("task", "msg-1", "staff-1", "chat-1")}
	request := groupopsport.ReconciliationEvidence{ExecutionID: 11, ExternalEffectID: "eer_41", EvidenceDigest: evidence.TaskEvidenceDigest}
	verifier, err := NewWeComGroupMessageReconciliationVerifier(&groupMessageQueryStub{results: []GroupMessageSendResultPage{{Items: []GroupMessageSendResult{{ChatID: "chat-1", UserID: "staff-1", Status: 1}}}}}, &groupMessageEvidenceSource{evidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	result, err := verifier.VerifyReconciliationEvidence(context.Background(), request)
	if err != nil || !result.DeliveryProven || result.EvidenceDigest == "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	verifier, err = NewWeComGroupMessageReconciliationVerifier(&groupMessageQueryStub{results: []GroupMessageSendResultPage{{Items: []GroupMessageSendResult{{ChatID: "chat-1", UserID: "staff-1", Status: 2}}}}}, &groupMessageEvidenceSource{evidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	result, err = verifier.VerifyReconciliationEvidence(context.Background(), request)
	if err != nil || result.DeliveryProven {
		t.Fatalf("non-delivery result=%+v err=%v", result, err)
	}
	request.EvidenceDigest = string(digest("caller-claimed"))
	result, err = verifier.VerifyReconciliationEvidence(context.Background(), request)
	if err != nil || result.DeliveryProven {
		t.Fatalf("caller evidence result=%+v err=%v", result, err)
	}
}

type groupMessageCreatorStub struct {
	result GroupMessageCreateResult
	err    error
	calls  int
}

func (stub *groupMessageCreatorStub) CreateGroupMessageTask(_ context.Context, request GroupMessageCreateRequest) (GroupMessageCreateResult, error) {
	stub.calls++
	if request.Sender != "staff-1" || len(request.ChatIDs) != 1 || request.ChatIDs[0] != "chat-1" || request.Text != "hello group" {
		return GroupMessageCreateResult{}, errors.New("unexpected group message request")
	}
	return stub.result, stub.err
}

type groupMessageTargetResolver string

func (resolver groupMessageTargetResolver) ResolveGroupMessageTarget(_ context.Context, target string) (string, bool, error) {
	return string(resolver), target == "asset-1", nil
}

type groupMessageQueryStub struct {
	results []GroupMessageSendResultPage
	calls   int
}

func (*groupMessageQueryStub) GetGroupMessageTask(_ context.Context, messageID, cursor string) (GroupMessageTaskPage, error) {
	if messageID != "msg-1" || cursor != "" {
		return GroupMessageTaskPage{}, errors.New("unexpected task query")
	}
	return GroupMessageTaskPage{Items: []GroupMessageTask{{UserID: "staff-1", Status: 2}}}, nil
}

func (stub *groupMessageQueryStub) GetGroupMessageSendResult(_ context.Context, messageID, userID, cursor string) (GroupMessageSendResultPage, error) {
	if messageID != "msg-1" || userID != "staff-1" || cursor != "" || stub.calls >= len(stub.results) {
		return GroupMessageSendResultPage{}, errors.New("unexpected send result query")
	}
	result := stub.results[stub.calls]
	stub.calls++
	return result, nil
}

type groupMessageEvidenceSource struct{ evidence GroupMessageEvidence; delivery string }

func (source *groupMessageEvidenceSource) FindGroupMessageReceipt(_ context.Context, _ groupopsport.ReconciliationEvidence) (GroupMessageEvidence, bool, error) {
	return source.evidence, true, nil
}

func (source *groupMessageEvidenceSource) RecordGroupMessageDelivery(_ context.Context, _ GroupMessageEvidence, digest string) error { source.delivery = digest; return nil }

type groupMessageReceiptWriterStub struct{ calls int }
func (stub *groupMessageReceiptWriterStub) RecordGroupMessageTask(_ context.Context, receipt groupopsport.GroupMessageReceipt) error {
	stub.calls++
	if receipt.ExecutionID != 11 || receipt.ExternalEffectID != "eer_41" || receipt.MessageID != "msg-1" || receipt.SenderUserID != "staff-1" || receipt.UserID != "staff-1" || receipt.ChatID != "chat-1" { return errors.New("unexpected receipt") }
	return nil
}

func groupMessageDispatchRequest() groupopsport.DispatchRequest {
	return groupopsport.DispatchRequest{
		ExecutionID: 11, ExternalEffectID: "eer_41", TargetReference: "asset-1",
		ContentSnapshot:  []byte(`{"schema_version":1,"node_kind":"message","message_text":"hello group"}`),
		MaterialSnapshot: []byte(`{"schema_version":1,"node_kind":"message","reference":""}`),
		ContentDigest:    string(digest("group-content")),
		MaterialDigest:   string(digest("group-material")),
	}
}
