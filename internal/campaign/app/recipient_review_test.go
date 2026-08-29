package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

type recipientReviewFixture struct {
	reviews    map[string]campaign.TouchPlanReview
	recipients map[string][]campaign.TouchPlanRecipient
	values     map[string]campaign.TouchPlanRecipientReview
	receipts   map[string]campaign.TouchPlanRecipientReviewReceipt
	events     []campaign.TouchPlanRecipientReviewEvent
	nextID     int64
	eventErr   error
	memberRead struct {
		status campaign.TouchPlanRecipientReviewStatus
		limit  int32
		offset int32
	}
}

func newRecipientReviewFixture(planID string) *recipientReviewFixture {
	return &recipientReviewFixture{
		reviews: map[string]campaign.TouchPlanReview{planID: {PlanID: planID, CampaignCode: "spring-campaign", Status: campaign.TouchPlanReviewDraft, Version: 1}},
		recipients: map[string][]campaign.TouchPlanRecipient{planID: {
			{PlanID: planID, CustomerID: 3},
			{PlanID: planID, CustomerID: 7},
		}},
		values:   map[string]campaign.TouchPlanRecipientReview{},
		receipts: map[string]campaign.TouchPlanRecipientReviewReceipt{},
		nextID:   1,
	}
}

func (f *recipientReviewFixture) Within(_ context.Context, fn func(context.Context) error) error {
	before := f.clone()
	if err := fn(context.Background()); err != nil {
		*f = *before
		return err
	}
	return nil
}

func (f *recipientReviewFixture) ListLatestCampaignMemberStatuses(_ context.Context, campaignCode string, status campaign.TouchPlanRecipientReviewStatus, limit, offset int32) (campaign.CampaignMemberStatusSnapshot, error) {
	f.memberRead.status, f.memberRead.limit, f.memberRead.offset = status, limit, offset
	planID := ""
	for candidate, review := range f.reviews {
		if review.CampaignCode == campaignCode {
			planID = candidate
			break
		}
	}
	if planID == "" {
		return campaign.CampaignMemberStatusSnapshot{}, campaign.ErrNotFound
	}
	items := make([]campaign.CampaignMemberStatus, 0, len(f.recipients[planID]))
	for _, recipient := range f.recipients[planID] {
		projected := campaign.TouchPlanRecipientReviewPending
		if review, ok := f.values[recipientReviewValueKey(planID, recipient.CustomerID)]; ok {
			projected = review.Status
		}
		if status == "" || projected == status {
			items = append(items, campaign.CampaignMemberStatus{PlanID: planID, CustomerID: recipient.CustomerID, Status: projected})
		}
	}
	total := int64(len(items))
	start := int(offset)
	if start > len(items) {
		start = len(items)
	}
	end := start + int(limit)
	if end > len(items) {
		end = len(items)
	}
	return campaign.CampaignMemberStatusSnapshot{PlanID: planID, Items: items[start:end], Total: total}, nil
}

func (f *recipientReviewFixture) ReserveTouchPlanRecipientReviewReceipt(_ context.Context, reservation campaignport.RecipientReviewReceiptReservation) (campaign.TouchPlanRecipientReviewReceipt, bool, error) {
	key := recipientReviewReceiptKey(reservation.ActorID, reservation.KeyDigest)
	if existing, ok := f.receipts[key]; ok {
		if existing.Operation != reservation.Operation || existing.PayloadDigest != reservation.PayloadDigest || existing.PlanID != reservation.PlanID || existing.CampaignCode != reservation.CampaignCode || existing.CustomerID != reservation.CustomerID {
			return campaign.TouchPlanRecipientReviewReceipt{}, false, campaign.ErrIdempotencyConflict
		}
		return cloneRecipientReviewReceipt(existing), false, nil
	}
	value := campaign.TouchPlanRecipientReviewReceipt{ID: f.nextID, ActorID: reservation.ActorID, Operation: reservation.Operation, KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, PlanID: reservation.PlanID, CampaignCode: reservation.CampaignCode, CustomerID: reservation.CustomerID, State: campaign.TouchPlanRecipientReviewReceiptReserved}
	f.nextID++
	f.receipts[key] = value
	return value, true, nil
}

func (f *recipientReviewFixture) LockTouchPlanReview(_ context.Context, campaignCode, planID string) (campaign.TouchPlanReview, error) {
	value, ok := f.reviews[planID]
	if !ok || value.CampaignCode != campaignCode {
		return campaign.TouchPlanReview{}, campaign.ErrNotFound
	}
	return value, nil
}

func (f *recipientReviewFixture) GetTouchPlanRecipient(_ context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipient, error) {
	if review, ok := f.reviews[planID]; !ok || review.CampaignCode != campaignCode {
		return campaign.TouchPlanRecipient{}, campaign.ErrNotFound
	}
	for _, value := range f.recipients[planID] {
		if value.CustomerID == customerID {
			return value, nil
		}
	}
	return campaign.TouchPlanRecipient{}, campaign.ErrNotFound
}

func (f *recipientReviewFixture) LockTouchPlanRecipientReview(_ context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipientReview, bool, error) {
	value, ok := f.values[recipientReviewValueKey(planID, customerID)]
	if !ok {
		return campaign.TouchPlanRecipientReview{}, false, nil
	}
	if value.CampaignCode != campaignCode || value.PlanID != planID || value.CustomerID != customerID {
		return campaign.TouchPlanRecipientReview{}, false, campaign.ErrNotFound
	}
	return value, true, nil
}

func (f *recipientReviewFixture) SaveTouchPlanRecipientReview(_ context.Context, value campaign.TouchPlanRecipientReview, expectedVersion int64) error {
	key := recipientReviewValueKey(value.PlanID, value.CustomerID)
	current, exists := f.values[key]
	if exists && current.Version != expectedVersion || !exists && expectedVersion != 0 {
		return campaign.ErrConflict
	}
	f.values[key] = value
	return nil
}

func (f *recipientReviewFixture) ReadTouchPlanRecipientReview(_ context.Context, campaignCode, planID string, customerID int64) (campaign.TouchPlanRecipientReview, error) {
	value, ok := f.values[recipientReviewValueKey(planID, customerID)]
	if !ok || value.CampaignCode != campaignCode {
		return campaign.TouchPlanRecipientReview{}, campaign.ErrNotFound
	}
	return value, nil
}

func (f *recipientReviewFixture) CompleteTouchPlanRecipientReviewReceipt(_ context.Context, id int64, result campaign.TouchPlanRecipientReviewResult, _ time.Time) error {
	for key, receipt := range f.receipts {
		if receipt.ID == id && receipt.State == campaign.TouchPlanRecipientReviewReceiptReserved {
			receipt.State = campaign.TouchPlanRecipientReviewReceiptCompleted
			receipt.Result = &result
			f.receipts[key] = receipt
			return nil
		}
	}
	return campaign.ErrUnavailable
}

func (f *recipientReviewFixture) AppendTouchPlanRecipientReviewEvent(_ context.Context, value campaign.TouchPlanRecipientReviewEvent) (int64, error) {
	if f.eventErr != nil {
		return 0, f.eventErr
	}
	if !campaign.ValidTouchPlanRecipientReviewAuditType(value.AuditType) || value.PlanID == "" || value.CampaignCode == "" || value.CustomerID < 1 || value.RecipientVersion < 1 || value.ActorID < 1 || value.OccurredAt.IsZero() {
		return 0, campaign.ErrUnavailable
	}
	id := f.nextID
	f.nextID++
	f.events = append(f.events, value)
	return id, nil
}

func (f *recipientReviewFixture) clone() *recipientReviewFixture {
	result := &recipientReviewFixture{
		reviews:    map[string]campaign.TouchPlanReview{},
		recipients: map[string][]campaign.TouchPlanRecipient{},
		values:     map[string]campaign.TouchPlanRecipientReview{},
		receipts:   map[string]campaign.TouchPlanRecipientReviewReceipt{},
		events:     append([]campaign.TouchPlanRecipientReviewEvent(nil), f.events...),
		nextID:     f.nextID,
		eventErr:   f.eventErr,
	}
	for key, value := range f.reviews {
		result.reviews[key] = value
	}
	for key, values := range f.recipients {
		result.recipients[key] = append([]campaign.TouchPlanRecipient(nil), values...)
	}
	for key, value := range f.values {
		result.values[key] = value
	}
	for key, value := range f.receipts {
		result.receipts[key] = cloneRecipientReviewReceipt(value)
	}
	return result
}

func cloneRecipientReviewReceipt(value campaign.TouchPlanRecipientReviewReceipt) campaign.TouchPlanRecipientReviewReceipt {
	if value.Result != nil {
		result := *value.Result
		value.Result = &result
	}
	return value
}

func recipientReviewReceiptKey(actorID int64, key [sha256.Size]byte) string {
	return fmt.Sprintf("%d/%x", actorID, key)
}

func recipientReviewValueKey(planID string, customerID int64) string {
	return fmt.Sprintf("%s/%d", planID, customerID)
}

func TestRecipientReviewOverrideDecisionReplayAndNoHandoff(t *testing.T) {
	planID := testReviewPlanID('a')
	fixture := newRecipientReviewFixture(planID)
	service := recipientReviewService(t, fixture)
	service.now = func() time.Time { return time.Date(2026, 8, 27, 10, 0, 0, 123456789, time.UTC) }

	override := campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 1, ExpectedRecipientVersion: 0, MessageOverride: "您好，欢迎查看本次方案。", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-override-key"}
	saved, err := service.SaveMessageOverride(context.Background(), override)
	if err != nil || saved.Review.Status != campaign.TouchPlanRecipientReviewPending || saved.Review.Version != 1 || saved.Review.MessageOverride != override.MessageOverride || saved.Review.Safety != campaign.LocalInitiationSafety() || saved.EventID < 1 || len(fixture.events) != 1 {
		t.Fatalf("saved=%+v events=%+v err=%v", saved, fixture.events, err)
	}
	if review := fixture.reviews[planID]; review.Status != campaign.TouchPlanReviewDraft || review.Version != 1 {
		t.Fatalf("plan review changed=%+v", review)
	}
	replayed, err := service.SaveMessageOverride(context.Background(), override)
	if err != nil || !reflect.DeepEqual(replayed, saved) || len(fixture.events) != 1 {
		t.Fatalf("replayed=%+v events=%+v err=%v", replayed, fixture.events, err)
	}

	fixture.reviews[planID] = pendingRecipientReview(planID, 2)
	updated, err := service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 2, ExpectedRecipientVersion: 1, MessageOverride: "您好，方案已更新。", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-pending-override"})
	if err != nil || updated.Review.Status != campaign.TouchPlanRecipientReviewPending || updated.Review.Version != 2 || updated.Review.MessageOverride != "您好，方案已更新。" || len(fixture.events) != 2 {
		t.Fatalf("updated=%+v events=%+v err=%v", updated, fixture.events, err)
	}
	approved, err := service.Approve(context.Background(), campaign.DecideTouchPlanRecipientCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 2, ExpectedRecipientVersion: 2, Actor: campaign.Actor{ID: 42}, IdempotencyKey: "test-test-test-test"})
	if err != nil || approved.Review.Status != campaign.TouchPlanRecipientReviewApproved || approved.Review.Version != 3 || approved.Review.MessageOverride != updated.Review.MessageOverride || len(fixture.events) != 3 {
		t.Fatalf("approved=%+v events=%+v err=%v", approved, fixture.events, err)
	}
	if review := fixture.reviews[planID]; review.Status != campaign.TouchPlanReviewPending || review.Version != 2 {
		t.Fatalf("recipient decision changed plan=%+v", review)
	}
	if fixture.events[2].AuditType != campaign.RecipientReviewAuditApproved || fixture.events[2].CustomerID != 7 {
		t.Fatalf("approval event=%+v", fixture.events[2])
	}
	if _, err = service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 2, ExpectedRecipientVersion: 3, MessageOverride: "cannot replace an approved decision", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "terminal-override-key"}); !errors.Is(err, campaign.ErrStateConflict) {
		t.Fatalf("terminal override=%v", err)
	}

	rejected, err := service.Reject(context.Background(), campaign.DecideTouchPlanRecipientCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 3, ExpectedPlanVersion: 2, ExpectedRecipientVersion: 0, Actor: campaign.Actor{ID: 42}, IdempotencyKey: "recipient-reject-key-1"})
	if err != nil || rejected.Review.Status != campaign.TouchPlanRecipientReviewRejected || rejected.Review.Version != 1 || rejected.Review.MessageOverride != "" || len(fixture.events) != 4 {
		t.Fatalf("rejected=%+v events=%+v err=%v", rejected, fixture.events, err)
	}
	if fixture.events[3].AuditType != campaign.RecipientReviewAuditRejected || fixture.events[3].CustomerID != 3 || fixture.reviews[planID].Version != 2 {
		t.Fatalf("reject event=%+v plan=%+v", fixture.events[3], fixture.reviews[planID])
	}
}

func TestCampaignMemberStatusProjectionAndFilter(t *testing.T) {
	planID := testReviewPlanID('9')
	fixture := newRecipientReviewFixture(planID)
	fixture.values[recipientReviewValueKey(planID, 7)] = campaign.TouchPlanRecipientReview{Status: campaign.TouchPlanRecipientReviewApproved}
	service := recipientReviewService(t, fixture)

	page, err := service.ListCampaignMembers(context.Background(), "spring-campaign", "", 1, 0)
	if err != nil || page.PlanID != planID || page.Total != 2 || page.Limit != 1 || page.Offset != 0 || len(page.Items) != 1 || page.Items[0].CustomerID != 3 || page.Items[0].Status != campaign.TouchPlanRecipientReviewPending || page.Safety != campaign.LocalInitiationSafety() {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	approved, err := service.ListCampaignMembers(context.Background(), "spring-campaign", campaign.TouchPlanRecipientReviewApproved, 100, 0)
	if err != nil || approved.Total != 1 || len(approved.Items) != 1 || approved.Items[0].CustomerID != 7 || fixture.memberRead.status != campaign.TouchPlanRecipientReviewApproved {
		t.Fatalf("approved=%+v read=%+v err=%v", approved, fixture.memberRead, err)
	}
	if _, err = service.ListCampaignMembers(context.Background(), "spring-campaign", "draft", 100, 0); !errors.Is(err, campaign.ErrInvalidArgument) {
		t.Fatalf("invalid status err=%v", err)
	}
	fixture.recipients[planID] = []campaign.TouchPlanRecipient{{PlanID: planID, CustomerID: 7}, {PlanID: planID, CustomerID: 3}}
	if _, err = service.ListCampaignMembers(context.Background(), "spring-campaign", "", 100, 0); !errors.Is(err, campaign.ErrUnavailable) {
		t.Fatalf("unordered projection err=%v", err)
	}
}

func TestRecipientReviewScopeCASAndIdempotency(t *testing.T) {
	planID := testReviewPlanID('b')
	fixture := newRecipientReviewFixture(planID)
	service := recipientReviewService(t, fixture)
	command := campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 1, ExpectedRecipientVersion: 0, MessageOverride: "first", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-scope-key-1"}
	if _, err := service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "other-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 1, ExpectedRecipientVersion: 0, MessageOverride: "scope", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-other-campaign"}); !errors.Is(err, campaign.ErrNotFound) || len(fixture.receipts) != 0 || len(fixture.values) != 0 {
		t.Fatalf("campaign scope err=%v receipts=%d values=%d", err, len(fixture.receipts), len(fixture.values))
	}
	if _, err := service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 99, ExpectedPlanVersion: 1, ExpectedRecipientVersion: 0, MessageOverride: "scope", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-other-target-1"}); !errors.Is(err, campaign.ErrNotFound) || len(fixture.receipts) != 0 || len(fixture.values) != 0 {
		t.Fatalf("recipient scope err=%v receipts=%d values=%d", err, len(fixture.receipts), len(fixture.values))
	}
	if _, err := service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 2, ExpectedRecipientVersion: 0, MessageOverride: "cas", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-plan-cas-key"}); !errors.Is(err, campaign.ErrConflict) || len(fixture.receipts) != 0 {
		t.Fatalf("plan CAS err=%v receipts=%d", err, len(fixture.receipts))
	}
	if _, err := service.SaveMessageOverride(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	otherPlanID := testReviewPlanID('d')
	fixture.reviews[otherPlanID] = campaign.TouchPlanReview{PlanID: otherPlanID, CampaignCode: "spring-campaign", Status: campaign.TouchPlanReviewDraft, Version: 1}
	fixture.recipients[otherPlanID] = []campaign.TouchPlanRecipient{{PlanID: otherPlanID, CustomerID: 7}}
	if _, err := service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: otherPlanID, CustomerID: 7, ExpectedPlanVersion: 1, ExpectedRecipientVersion: 0, MessageOverride: "other plan", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-other-plan-key"}); err != nil || len(fixture.values) != 2 || fixture.values[recipientReviewValueKey(planID, 7)].MessageOverride != "first" || fixture.values[recipientReviewValueKey(otherPlanID, 7)].MessageOverride != "other plan" {
		t.Fatalf("plan+recipient scope err=%v values=%+v", err, fixture.values)
	}
	if _, err := service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 1, ExpectedRecipientVersion: 0, MessageOverride: "stale", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-version-cas"}); !errors.Is(err, campaign.ErrConflict) || fixture.values[recipientReviewValueKey(planID, 7)].MessageOverride != "first" {
		t.Fatalf("recipient CAS err=%v value=%+v", err, fixture.values[recipientReviewValueKey(planID, 7)])
	}
	if _, err := service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 1, ExpectedRecipientVersion: 1, MessageOverride: "different payload", Actor: campaign.Actor{ID: 41}, IdempotencyKey: command.IdempotencyKey}); !errors.Is(err, campaign.ErrIdempotencyConflict) {
		t.Fatalf("idempotency payload err=%v", err)
	}
}

func TestRecipientReviewStateAndEventRollback(t *testing.T) {
	planID := testReviewPlanID('c')
	fixture := newRecipientReviewFixture(planID)
	service := recipientReviewService(t, fixture)
	decision := campaign.DecideTouchPlanRecipientCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 1, ExpectedRecipientVersion: 0, Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-draft-decision"}
	if _, err := service.Approve(context.Background(), decision); !errors.Is(err, campaign.ErrStateConflict) {
		t.Fatalf("draft decision=%v", err)
	}
	if len(fixture.values) != 0 || len(fixture.receipts) != 0 || len(fixture.events) != 0 {
		t.Fatalf("draft decision wrote values=%d receipts=%d events=%d", len(fixture.values), len(fixture.receipts), len(fixture.events))
	}
	fixture.reviews[planID] = terminalRecipientReview(planID)
	if _, err := service.SaveMessageOverride(context.Background(), campaign.SaveTouchPlanRecipientMessageOverrideCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 3, ExpectedRecipientVersion: 0, MessageOverride: "terminal plan", Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-terminal-plan"}); !errors.Is(err, campaign.ErrStateConflict) {
		t.Fatalf("terminal plan=%v", err)
	}
	fixture.reviews[planID] = pendingRecipientReview(planID, 2)
	fixture.eventErr = errors.New("event unavailable")
	if _, err := service.Reject(context.Background(), campaign.DecideTouchPlanRecipientCommand{CampaignCode: "spring-campaign", PlanID: planID, CustomerID: 7, ExpectedPlanVersion: 2, ExpectedRecipientVersion: 0, Actor: campaign.Actor{ID: 41}, IdempotencyKey: "recipient-event-rollback"}); !errors.Is(err, campaign.ErrUnavailable) || len(fixture.values) != 0 || len(fixture.receipts) != 0 || len(fixture.events) != 0 {
		t.Fatalf("event rollback err=%v values=%d receipts=%d events=%d", err, len(fixture.values), len(fixture.receipts), len(fixture.events))
	}
}

func pendingRecipientReview(planID string, version int64) campaign.TouchPlanReview {
	return campaign.TouchPlanReview{PlanID: planID, CampaignCode: "spring-campaign", Status: campaign.TouchPlanReviewPending, Version: version, SubmittedByActorID: 11, SubmittedAt: time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)}
}

func terminalRecipientReview(planID string) campaign.TouchPlanReview {
	return campaign.TouchPlanReview{PlanID: planID, CampaignCode: "spring-campaign", Status: campaign.TouchPlanReviewApproved, Version: 3, SubmittedByActorID: 11, SubmittedAt: time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC), ReviewedByActorID: 12, ReviewedAt: time.Date(2026, 8, 27, 9, 1, 0, 0, time.UTC), ConfirmationDigest: campaign.ReviewConfirmationDigest(campaign.ReviewConfirmation("approve", planID))}
}

func recipientReviewService(t *testing.T, fixture *recipientReviewFixture) *RecipientReviewService {
	t.Helper()
	service, err := NewRecipientReviewService(fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
