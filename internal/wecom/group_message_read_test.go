package wecom

import (
	"context"
	"errors"
	"testing"

	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

func TestGroupMessageReconciliationVerifierRequiresExactDeliveredRecord(t *testing.T) {
	receipt := groupopsport.GroupMessageReceipt{
		ExecutionID: 11, ExternalEffectID: "eer_41", MessageID: "msg-1", SenderUserID: "staff-1", UserID: "staff-1", ChatID: "chat-1",
		TaskEvidenceDigest: groupMessageReadDigest("task", "msg-1", "staff-1", "chat-1"),
	}
	request := groupopsport.ReconciliationEvidence{ExecutionID: 11, ExternalEffectID: "eer_41", EvidenceDigest: receipt.TaskEvidenceDigest}

	for _, test := range []struct {
		name    string
		result  GroupMessageSendResult
		deliver bool
	}{
		{name: "exact sent", result: GroupMessageSendResult{ChatID: "chat-1", UserID: "staff-1", Status: 1}, deliver: true},
		{name: "other group", result: GroupMessageSendResult{ChatID: "chat-2", UserID: "staff-1", Status: 1}},
		{name: "not sent", result: GroupMessageSendResult{ChatID: "chat-1", UserID: "staff-1", Status: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			evidence := &groupMessageReceiptFixture{receipt: receipt}
			verifier, err := NewGroupMessageReconciliationVerifier(&groupMessageReadFixture{result: test.result}, evidence)
			if err != nil {
				t.Fatal(err)
			}
			result, err := verifier.VerifyReconciliationEvidence(context.Background(), request)
			if err != nil || result.DeliveryProven != test.deliver || (test.deliver && evidence.delivery == "") || (!test.deliver && evidence.delivery != "") {
				t.Fatalf("result=%+v delivery=%q err=%v", result, evidence.delivery, err)
			}
		})
	}

	verifier, err := NewGroupMessageReconciliationVerifier(&groupMessageReadFixture{result: GroupMessageSendResult{ChatID: "chat-1", UserID: "staff-1", Status: 1}}, &groupMessageReceiptFixture{receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	request.EvidenceDigest = groupMessageReadDigest("caller-claimed")
	result, err := verifier.VerifyReconciliationEvidence(context.Background(), request)
	if err != nil || result.DeliveryProven {
		t.Fatalf("caller result=%+v err=%v", result, err)
	}
}

type groupMessageReadFixture struct{ result GroupMessageSendResult }

func (*groupMessageReadFixture) GetGroupMessageTask(_ context.Context, messageID, cursor string) (GroupMessageTaskPage, error) {
	if messageID != "msg-1" || cursor != "" {
		return GroupMessageTaskPage{}, errors.New("unexpected task request")
	}
	return GroupMessageTaskPage{Items: []GroupMessageTask{{UserID: "staff-1", Status: 2}}}, nil
}

func (fixture *groupMessageReadFixture) GetGroupMessageSendResult(_ context.Context, messageID, userID, cursor string) (GroupMessageSendResultPage, error) {
	if messageID != "msg-1" || userID != "staff-1" || cursor != "" {
		return GroupMessageSendResultPage{}, errors.New("unexpected send result request")
	}
	return GroupMessageSendResultPage{Items: []GroupMessageSendResult{fixture.result}}, nil
}

type groupMessageReceiptFixture struct {
	receipt  groupopsport.GroupMessageReceipt
	delivery string
}

func (fixture *groupMessageReceiptFixture) FindGroupMessageReceipt(_ context.Context, _ groupopsport.ReconciliationEvidence) (groupopsport.GroupMessageReceipt, bool, error) {
	return fixture.receipt, true, nil
}

func (fixture *groupMessageReceiptFixture) RecordGroupMessageDelivery(_ context.Context, _ groupopsport.GroupMessageReceipt, digest string) error {
	fixture.delivery = digest
	return nil
}
