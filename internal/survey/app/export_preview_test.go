package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestExportPreviewDefaultsMaskTopLevelIdentityAndCreatesBlockedPlan(t *testing.T) {
	store := &exportPreviewStoreStub{snapshot: previewSnapshot()}
	service := NewExportPreviewService(testUOW{}, store, &exportPreviewAuthorizerStub{})
	service.now = func() time.Time { return time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC) }

	result, err := service.Preview(context.Background(), ExportPreviewCommand{QuestionnaireID: 41, ActorID: 7, IdempotencyKey: "preview-default-key-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if result.WriteModelStatus != ExportPreviewStatus || result.ExportPreview.EstimatedCount != 9 || len(result.ExportPreview.MaskedSample) != 3 || result.ExportPreview.FileCreated || result.SideEffectPlan.AdapterMode != "real_blocked" || !result.SideEffectPlan.RequiresApproval || result.SideEffectPlan.RealExternalCallExecuted || result.SideEffectPlan.Status != "planned" || result.SideEffectPlan.EffectType != ExportPreviewEffectType {
		t.Fatalf("preview result = %#v", result)
	}
	if got := result.ExportPreview.Fields; len(got) != 4 || got[0] != "submission_id" || got[1] != "external_userid" || got[2] != "answers" || got[3] != "created_at" {
		t.Fatalf("fields = %v", got)
	}
	first := result.ExportPreview.MaskedSample[0]
	if first["submission_id"] != "44" || first["external_userid"] != "masked" || string(first["answers"].(json.RawMessage)) != `{"free_text":"still sensitive"}` || first["created_at"].(time.Time) != previewTime(12, 0) {
		t.Fatalf("first masked sample = %#v", first)
	}
	if store.readLimit != ExportPreviewLimit || store.completeCalls != 1 || store.fileWrites != 0 || store.providerCalls != 0 || store.receiptContains("still sensitive") || store.receiptContains("external-44") {
		t.Fatalf("store calls/read receipt = %#v", store)
	}
}

func TestExportPreviewMasksEveryTopLevelIdentityFieldButPreservesSensitiveAnswerForManager(t *testing.T) {
	store := &exportPreviewStoreStub{snapshot: previewSnapshot()}
	service := NewExportPreviewService(testUOW{}, store, &exportPreviewAuthorizerStub{})
	fields := []string{"mobile", "openid", "unionid", "respondent_key", "customer_name", "follow_user_userid", "answers", "unknown"}
	result, err := service.Preview(context.Background(), ExportPreviewCommand{QuestionnaireID: 41, ActorID: 7, IdempotencyKey: "preview-custom-key-00001", Fields: fields})
	if err != nil {
		t.Fatal(err)
	}
	row := result.ExportPreview.MaskedSample[0]
	for _, field := range fields[:6] {
		if row[field] != "masked" {
			t.Fatalf("%s = %#v", field, row[field])
		}
	}
	if string(row["answers"].(json.RawMessage)) != `{"free_text":"still sensitive"}` || row["unknown"] != nil {
		t.Fatalf("answers/unknown = %#v", row)
	}
}

func TestExportPreviewIdempotencyDoesNotPersistSensitiveSamples(t *testing.T) {
	store := &exportPreviewStoreStub{snapshot: previewSnapshot()}
	service := NewExportPreviewService(testUOW{}, store, &exportPreviewAuthorizerStub{})
	command := ExportPreviewCommand{QuestionnaireID: 41, ActorID: 7, IdempotencyKey: "preview-replay-key-00001"}
	if _, err := service.Preview(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Preview(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if store.reserveCalls != 2 || store.completeCalls != 1 || store.readCalls != 2 || store.receiptContains("still sensitive") || store.receiptContains("external-44") {
		t.Fatalf("idempotency state = %#v", store)
	}
	if _, err := service.Preview(context.Background(), ExportPreviewCommand{QuestionnaireID: 41, ActorID: 7, IdempotencyKey: command.IdempotencyKey, Fields: []string{"answers"}}); !errors.Is(err, ErrConflict) || store.readCalls != 2 {
		t.Fatalf("payload conflict error=%v reads=%d", err, store.readCalls)
	}
}

func TestExportPreviewRejectsNonObjectInputAndInvalidFields(t *testing.T) {
	for _, input := range []any{nil, []any{}, "not an object", map[string]any{"fields": "not an array"}, map[string]any{"fields": []any{""}}} {
		if _, err := DecodeExportPreviewFields(input); !errors.Is(err, ErrInvalidExportPreview) {
			t.Fatalf("DecodeExportPreviewFields(%#v) = %v", input, err)
		}
	}
	fields, err := DecodeExportPreviewFields(map[string]any{})
	if err != nil || len(fields) != 4 || fields[0] != "submission_id" {
		t.Fatalf("default fields = %v, %v", fields, err)
	}
	fields, err = DecodeExportPreviewFields(map[string]any{"fields": []any{"answers", "mobile"}})
	if err != nil || len(fields) != 2 || fields[1] != "mobile" {
		t.Fatalf("custom fields = %v, %v", fields, err)
	}
}

func TestExportPreviewFailsClosedForAuthorizationOwnerAndStoreErrors(t *testing.T) {
	denied := NewExportPreviewService(testUOW{}, &exportPreviewStoreStub{snapshot: previewSnapshot()}, &exportPreviewAuthorizerStub{denied: true})
	if _, err := denied.Preview(context.Background(), validPreviewCommand()); !errors.Is(err, ErrExportPreviewDenied) {
		t.Fatalf("denied preview error = %v", err)
	}
	for name, storeErr := range map[string]error{"missing questionnaire": ErrNotFound, "database unavailable": errors.New("database unavailable")} {
		t.Run(name, func(t *testing.T) {
			service := NewExportPreviewService(testUOW{}, &exportPreviewStoreStub{readErr: storeErr}, &exportPreviewAuthorizerStub{})
			want := storeErr
			if name == "database unavailable" {
				want = ErrUnavailable
			}
			if _, err := service.Preview(context.Background(), validPreviewCommand()); !errors.Is(err, want) {
				t.Fatalf("preview error = %v, want %v", err, want)
			}
		})
	}
}

func TestExportPreviewRejectsUnorderedOrOversizedSnapshot(t *testing.T) {
	bad := previewSnapshot()
	bad.Submissions[0], bad.Submissions[1] = bad.Submissions[1], bad.Submissions[0]
	service := NewExportPreviewService(testUOW{}, &exportPreviewStoreStub{snapshot: bad}, &exportPreviewAuthorizerStub{})
	if _, err := service.Preview(context.Background(), validPreviewCommand()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unordered preview error = %v", err)
	}
}

type exportPreviewStoreStub struct {
	snapshot      ExportPreviewSnapshot
	readErr       error
	receipts      map[string]ExportPreviewReceipt
	reserveCalls  int
	readCalls     int
	completeCalls int
	readLimit     int32
	fileWrites    int
	providerCalls int
}

func (store *exportPreviewStoreStub) ReserveExportPreview(_ context.Context, reservation ExportPreviewReservation) (ExportPreviewReceipt, bool, error) {
	store.reserveCalls++
	if store.receipts == nil {
		store.receipts = map[string]ExportPreviewReceipt{}
	}
	key := reservation.ActorScope + ":" + string(reservation.KeyDigest[:])
	if receipt, exists := store.receipts[key]; exists {
		return receipt, false, nil
	}
	receipt := ExportPreviewReceipt{ID: int64(len(store.receipts) + 1), ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, State: "in_progress"}
	store.receipts[key] = receipt
	return receipt, true, nil
}

func (store *exportPreviewStoreStub) CompleteExportPreview(_ context.Context, id int64, _ time.Time) (ExportPreviewReceipt, error) {
	store.completeCalls++
	for key, receipt := range store.receipts {
		if receipt.ID == id {
			receipt.State = "completed"
			store.receipts[key] = receipt
			return receipt, nil
		}
	}
	return ExportPreviewReceipt{}, ErrUnavailable
}

func (store *exportPreviewStoreStub) ReadExportPreview(_ context.Context, _ surveyport.ID, limit int32) (ExportPreviewSnapshot, error) {
	store.readCalls++
	store.readLimit = limit
	return store.snapshot, store.readErr
}

func (store *exportPreviewStoreStub) receiptContains(value string) bool {
	for _, receipt := range store.receipts {
		encoded, _ := json.Marshal(receipt)
		if strings.Contains(string(encoded), value) {
			return true
		}
	}
	return false
}

type exportPreviewAuthorizerStub struct{ denied bool }

func (stub *exportPreviewAuthorizerStub) AuthorizeExportPreview(_ context.Context, permission string) error {
	if permission != ExportPreviewPermission {
		return ErrUnavailable
	}
	if stub.denied {
		return ErrExportPreviewDenied
	}
	return nil
}

func validPreviewCommand() ExportPreviewCommand {
	return ExportPreviewCommand{QuestionnaireID: 41, ActorID: 7, IdempotencyKey: "preview-valid-key-000001"}
}

func previewSnapshot() ExportPreviewSnapshot {
	return ExportPreviewSnapshot{
		QuestionnaireID: 41, EstimatedCount: 9,
		Submissions: []ExportPreviewSubmission{
			{ID: 44, QuestionnaireID: 41, ExternalUserID: "external-44", OpenID: "openid-44", UnionID: "union-44", Mobile: "13800000000", RespondentKey: "respondent-44", CustomerName: "姓名", FollowUserUserID: "staff-44", Answers: json.RawMessage(`{"free_text":"still sensitive"}`), CreatedAt: previewTime(12, 0), SubmittedAt: previewTime(12, 0)},
			{ID: 43, QuestionnaireID: 41, ExternalUserID: "external-43", Answers: json.RawMessage(`{"free_text":"second"}`), CreatedAt: previewTime(11, 0), SubmittedAt: previewTime(11, 0)},
			{ID: 42, QuestionnaireID: 41, ExternalUserID: "external-42", Answers: json.RawMessage(`{"free_text":"third"}`), CreatedAt: previewTime(10, 0), SubmittedAt: previewTime(10, 0)},
		},
	}
}

func previewTime(hour, minute int) time.Time {
	return time.Date(2026, 8, 16, hour, minute, 0, 0, time.UTC)
}
