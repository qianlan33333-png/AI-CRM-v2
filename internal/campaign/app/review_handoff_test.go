package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

type reviewFixture struct {
	reviews                map[string]campaign.TouchPlanReview
	handoffs               map[string]campaign.TouchPlanHandoff
	receipts               map[string]campaign.TouchPlanReviewReceipt
	recipients             map[string][]campaign.TouchPlanRecipient
	events                 []campaign.TouchPlanReviewEvent
	nextReceipt, nextEvent int64
	corrupt                bool
}

func newReviewFixture(planID string) *reviewFixture {
	return &reviewFixture{reviews: map[string]campaign.TouchPlanReview{planID: {PlanID: planID, CampaignCode: "spring-campaign", Status: campaign.TouchPlanReviewDraft, Version: 1}}, handoffs: map[string]campaign.TouchPlanHandoff{}, receipts: map[string]campaign.TouchPlanReviewReceipt{}, recipients: map[string][]campaign.TouchPlanRecipient{planID: {{PlanID: planID, CustomerID: 3}, {PlanID: planID, CustomerID: 7}, {PlanID: planID, CustomerID: 9}}}, nextReceipt: 1, nextEvent: 1}
}
func (f *reviewFixture) Within(_ context.Context, fn func(context.Context) error) error {
	before := f.clone()
	if err := fn(context.Background()); err != nil {
		*f = *before
		return err
	}
	return nil
}
func (f *reviewFixture) ReserveReviewReceipt(_ context.Context, r campaignport.ReviewReceiptReservation) (campaign.TouchPlanReviewReceipt, bool, error) {
	key := reviewReceiptKey(r.ActorID, r.KeyDigest)
	if old, ok := f.receipts[key]; ok {
		if old.Operation != r.Operation || old.PayloadDigest != r.PayloadDigest || old.PlanID != r.PlanID {
			return campaign.TouchPlanReviewReceipt{}, false, campaign.ErrIdempotencyConflict
		}
		return cloneReceipt(old), false, nil
	}
	value := campaign.TouchPlanReviewReceipt{ID: f.nextReceipt, ActorID: r.ActorID, Operation: r.Operation, KeyDigest: r.KeyDigest, PayloadDigest: r.PayloadDigest, PlanID: r.PlanID, CampaignCode: r.CampaignCode, State: campaign.TouchPlanReviewReceiptReserved}
	f.nextReceipt++
	f.receipts[key] = value
	return value, true, nil
}
func (f *reviewFixture) LockTouchPlanReview(_ context.Context, campaignCode, planID string) (campaign.TouchPlanReview, error) {
	value, ok := f.reviews[planID]
	if !ok {
		return campaign.TouchPlanReview{}, campaign.ErrNotFound
	}
	if value.CampaignCode != campaignCode {
		return campaign.TouchPlanReview{}, campaign.ErrNotFound
	}
	return value, nil
}
func (f *reviewFixture) SaveTouchPlanReview(_ context.Context, value campaign.TouchPlanReview, expected int64) error {
	current, ok := f.reviews[value.PlanID]
	if !ok {
		return campaign.ErrNotFound
	}
	if current.Version != expected {
		return campaign.ErrConflict
	}
	f.reviews[value.PlanID] = value
	return nil
}
func (f *reviewFixture) ReadTouchPlanReview(_ context.Context, campaignCode, planID string) (campaign.TouchPlanReview, error) {
	value, ok := f.reviews[planID]
	if !ok {
		return campaign.TouchPlanReview{}, campaign.ErrNotFound
	}
	if value.CampaignCode != campaignCode {
		return campaign.TouchPlanReview{}, campaign.ErrNotFound
	}
	if f.corrupt {
		value.Version++
	}
	return value, nil
}
func (f *reviewFixture) CreateTouchPlanHandoff(_ context.Context, value campaign.TouchPlanHandoff) error {
	if _, ok := f.handoffs[value.PlanID]; ok {
		return campaign.ErrConflict
	}
	f.handoffs[value.PlanID] = value
	return nil
}
func (f *reviewFixture) ReadTouchPlanHandoff(_ context.Context, campaignCode, planID string) (campaign.TouchPlanHandoff, error) {
	value, ok := f.handoffs[planID]
	if !ok {
		return campaign.TouchPlanHandoff{}, campaign.ErrNotFound
	}
	if value.CampaignCode != campaignCode {
		return campaign.TouchPlanHandoff{}, campaign.ErrNotFound
	}
	return value, nil
}
func (f *reviewFixture) CompleteReviewReceipt(_ context.Context, id int64, result campaign.TouchPlanReviewResult, _ time.Time) error {
	for key, value := range f.receipts {
		if value.ID == id && value.State == campaign.TouchPlanReviewReceiptReserved {
			value.State = campaign.TouchPlanReviewReceiptCompleted
			value.Result = &result
			f.receipts[key] = value
			return nil
		}
	}
	return campaign.ErrUnavailable
}
func (f *reviewFixture) ListTouchPlanRecipients(_ context.Context, campaignCode, planID string, after int64, limit int32) ([]campaign.TouchPlanRecipient, error) {
	if f.reviews[planID].CampaignCode != campaignCode {
		return nil, campaign.ErrNotFound
	}
	values := []campaign.TouchPlanRecipient{}
	for _, value := range f.recipients[planID] {
		if value.CustomerID > after && len(values) < int(limit) {
			values = append(values, value)
		}
	}
	return values, nil
}
func (f *reviewFixture) GetTouchPlanRecipient(_ context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipient, error) {
	if f.reviews[planID].CampaignCode != campaignCode {
		return campaign.TouchPlanRecipient{}, campaign.ErrNotFound
	}
	for _, value := range f.recipients[planID] {
		if value.CustomerID == customerID {
			return value, nil
		}
	}
	return campaign.TouchPlanRecipient{}, campaign.ErrNotFound
}
func (f *reviewFixture) AppendTouchPlanReviewEvent(_ context.Context, value campaign.TouchPlanReviewEvent) (int64, error) {
	if !campaign.ValidReviewAuditType(value.AuditType) || value.PlanID == "" || value.ReviewVersion < 1 || value.ActorID < 1 || value.OccurredAt.IsZero() {
		return 0, campaign.ErrUnavailable
	}
	id := f.nextEvent
	f.nextEvent++
	f.events = append(f.events, value)
	return id, nil
}
func (f *reviewFixture) clone() *reviewFixture {
	result := newReviewFixture("")
	result.reviews = map[string]campaign.TouchPlanReview{}
	result.handoffs = map[string]campaign.TouchPlanHandoff{}
	result.receipts = map[string]campaign.TouchPlanReviewReceipt{}
	result.recipients = map[string][]campaign.TouchPlanRecipient{}
	for k, v := range f.reviews {
		result.reviews[k] = v
	}
	for k, v := range f.handoffs {
		result.handoffs[k] = v
	}
	for k, v := range f.receipts {
		result.receipts[k] = cloneReceipt(v)
	}
	for k, v := range f.recipients {
		result.recipients[k] = append([]campaign.TouchPlanRecipient(nil), v...)
	}
	result.events = append([]campaign.TouchPlanReviewEvent(nil), f.events...)
	result.nextReceipt = f.nextReceipt
	result.nextEvent = f.nextEvent
	result.corrupt = f.corrupt
	return result
}
func cloneReceipt(value campaign.TouchPlanReviewReceipt) campaign.TouchPlanReviewReceipt {
	if value.Result != nil {
		result := cloneReviewResult(*value.Result)
		value.Result = &result
	}
	return value
}
func reviewReceiptKey(actor int64, key [sha256.Size]byte) string {
	return fmt.Sprintf("%d/%x", actor, key)
}

func TestReviewHandoffTransitionsReplayAndLocalFacts(t *testing.T) {
	planID := testReviewPlanID('a')
	fixture := newReviewFixture(planID)
	service := reviewService(t, fixture)
	now := time.Date(2026, 8, 23, 12, 0, 0, 123456789, time.UTC)
	service.now = func() time.Time { return now }
	submitted, err := service.Submit(context.Background(), campaign.SubmitTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 7}, IdempotencyKey: "submit-review-key1"})
	if err != nil || submitted.Status != campaign.TouchPlanReviewPending || submitted.Version != 2 || submitted.SubmittedAt.Nanosecond() != 123456000 || len(fixture.events) != 1 || fixture.events[0].AuditType != campaign.ReviewAuditSubmitted {
		t.Fatalf("submitted=%+v events=%+v err=%v", submitted, fixture.events, err)
	}
	replay, err := service.Submit(context.Background(), campaign.SubmitTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 7}, IdempotencyKey: "submit-review-key1"})
	if err != nil || replay != submitted || len(fixture.events) != 1 {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	decision := campaign.DecideTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 2, Actor: campaign.Actor{ID: 8}, IdempotencyKey: "approve-review-key", Confirmation: campaign.ReviewConfirmation("approve", planID)}
	approved, err := service.Approve(context.Background(), decision)
	if err != nil || approved.Review.Status != campaign.TouchPlanReviewApproved || approved.Handoff == nil || !campaign.ValidTouchPlanHandoff(*approved.Handoff) || !reflect.DeepEqual(approved.EventIDs, []int64{2, 3}) || len(fixture.handoffs) != 1 {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}
	replayed, err := service.Approve(context.Background(), decision)
	if err != nil || !reflect.DeepEqual(replayed, approved) || len(fixture.events) != 3 {
		t.Fatalf("replayed=%+v err=%v", replayed, err)
	}
	lateSubmitReplay, err := service.Submit(context.Background(), campaign.SubmitTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 7}, IdempotencyKey: "submit-review-key1"})
	if err != nil || lateSubmitReplay != submitted || len(fixture.events) != 3 {
		t.Fatalf("durable submit replay=%+v err=%v", lateSubmitReplay, err)
	}
}
func TestReviewCampaignCodeScopeFailsBeforeWrite(t *testing.T) {
	planID := testReviewPlanID('e')
	fixture := newReviewFixture(planID)
	service := reviewService(t, fixture)
	_, err := service.Submit(context.Background(), campaign.SubmitTouchPlanReviewCommand{CampaignCode: "other-campaign", PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 7}, IdempotencyKey: "cross-campaign-key"})
	if !errors.Is(err, campaign.ErrNotFound) || len(fixture.receipts) != 0 || len(fixture.events) != 0 || fixture.reviews[planID].Status != campaign.TouchPlanReviewDraft {
		t.Fatalf("err=%v receipts=%d events=%d review=%+v", err, len(fixture.receipts), len(fixture.events), fixture.reviews[planID])
	}
}
func TestReviewCASConfirmationTerminalAndReadback(t *testing.T) {
	planID := testReviewPlanID('b')
	fixture := newReviewFixture(planID)
	service := reviewService(t, fixture)
	if _, err := service.Submit(context.Background(), campaign.SubmitTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 2, Actor: campaign.Actor{ID: 7}, IdempotencyKey: "submit-version-key"}); !errors.Is(err, campaign.ErrConflict) {
		t.Fatalf("cas=%v", err)
	}
	if _, err := service.Submit(context.Background(), campaign.SubmitTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 7}, IdempotencyKey: "submit-success-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reject(context.Background(), campaign.DecideTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 2, Actor: campaign.Actor{ID: 8}, IdempotencyKey: strings.Join([]string{"reject", "wrong", "key", "1"}, "-"), Confirmation: "REJECT"}); !errors.Is(err, campaign.ErrInvalidArgument) {
		t.Fatalf("confirmation=%v", err)
	}
	rejected, err := service.Reject(context.Background(), campaign.DecideTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 2, Actor: campaign.Actor{ID: 8}, IdempotencyKey: "reject-success-key", Confirmation: campaign.ReviewConfirmation("reject", planID)})
	if err != nil || rejected.Review.Status != campaign.TouchPlanReviewRejected || rejected.Handoff != nil {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}
	if _, err = service.Approve(context.Background(), campaign.DecideTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 3, Actor: campaign.Actor{ID: 8}, IdempotencyKey: "approve-terminal-key", Confirmation: campaign.ReviewConfirmation("approve", planID)}); !errors.Is(err, campaign.ErrStateConflict) {
		t.Fatalf("terminal=%v", err)
	}
	fixture = newReviewFixture(planID)
	fixture.corrupt = true
	service = reviewService(t, fixture)
	if _, err = service.Submit(context.Background(), campaign.SubmitTouchPlanReviewCommand{CampaignCode: "spring-campaign", PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 7}, IdempotencyKey: "readback-failure1"}); !errors.Is(err, campaign.ErrUnavailable) || len(fixture.receipts) != 0 {
		t.Fatalf("readback=%v receipts=%d", err, len(fixture.receipts))
	}
}
func TestReviewRecipientsStayPlanBound(t *testing.T) {
	planID := testReviewPlanID('c')
	service := reviewService(t, newReviewFixture(planID))
	page, err := service.ListRecipients(context.Background(), "spring-campaign", planID, nil, 2)
	if err != nil || len(page.Items) != 2 || page.Next == nil || page.Next.PlanID != planID || page.Next.CustomerID != 7 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	next, err := service.ListRecipients(context.Background(), "spring-campaign", planID, page.Next, 2)
	if err != nil || len(next.Items) != 1 || next.Items[0].CustomerID != 9 {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	if _, err = service.ListRecipients(context.Background(), "spring-campaign", planID, &campaign.TouchPlanRecipientKeyset{PlanID: testReviewPlanID('d'), CustomerID: 7}, 2); !errors.Is(err, campaign.ErrInvalidArgument) {
		t.Fatalf("cursor=%v", err)
	}
	one, err := service.GetRecipient(context.Background(), "spring-campaign", planID, 7)
	if err != nil || one != (campaign.TouchPlanRecipient{PlanID: planID, CustomerID: 7}) {
		t.Fatalf("recipient=%+v err=%v", one, err)
	}
	empty, err := service.ListRecipients(context.Background(), "spring-campaign", planID, &campaign.TouchPlanRecipientKeyset{PlanID: planID, CustomerID: 9}, 2)
	if err != nil || len(empty.Items) != 0 || empty.Next != nil {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	if _, err = service.ListRecipients(context.Background(), "other-campaign", planID, nil, 2); !errors.Is(err, campaign.ErrNotFound) {
		t.Fatalf("cross campaign list=%v", err)
	}
}
func reviewService(t *testing.T, f *reviewFixture) *ReviewHandoffService {
	t.Helper()
	service, err := NewReviewHandoffService(f, f, f)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
func testReviewPlanID(value rune) string {
	return "ctp_" + strings.Repeat(string(value), sha256.Size*2)
}
