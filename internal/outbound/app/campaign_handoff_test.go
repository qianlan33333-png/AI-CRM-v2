package app

import (
	"context"
	"errors"
	"testing"
	"time"

	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type campaignHandoffTxKey struct{}
type campaignHandoffFixture struct {
	receipt                                                       outboundport.CampaignHandoffReceipt
	stored                                                        outbound.AcceptedCampaignHandoff
	snapshot                                                      outboundport.ApprovedCampaignHandoffSnapshot
	sourceCalls, eventCalls, createCalls, completeCalls, uowCalls int
	eventID                                                       int64
	readbackTampered                                              bool
	readbackLinkTampered                                          bool
}

func (fixture *campaignHandoffFixture) Within(ctx context.Context, callback func(context.Context) error) error {
	fixture.uowCalls++
	return callback(context.WithValue(ctx, campaignHandoffTxKey{}, fixture.uowCalls))
}
func (fixture *campaignHandoffFixture) LockApprovedCampaignHandoff(ctx context.Context, _, _ string) (outboundport.ApprovedCampaignHandoffSnapshot, error) {
	fixture.sourceCalls++
	if ctx.Value(campaignHandoffTxKey{}) == nil {
		return outboundport.ApprovedCampaignHandoffSnapshot{}, errors.New("missing outer tx")
	}
	return cloneApprovedCampaignHandoffSnapshot(fixture.snapshot), nil
}
func (fixture *campaignHandoffFixture) ReserveCampaignHandoff(_ context.Context, value outboundport.CampaignHandoffReservation) (outboundport.CampaignHandoffReceipt, bool, error) {
	if fixture.receipt.ID > 0 {
		return fixture.receipt, false, nil
	}
	fixture.receipt = outboundport.CampaignHandoffReceipt{ID: 1, ActorID: value.ActorID, KeyDigest: value.KeyDigest, PayloadDigest: value.PayloadDigest, CampaignCode: value.CampaignCode, PlanID: value.PlanID, State: campaignHandoffReceiptReserved}
	return fixture.receipt, true, nil
}
func (fixture *campaignHandoffFixture) CreateAcceptedCampaignHandoff(_ context.Context, snapshot outboundport.ApprovedCampaignHandoffSnapshot, actorID int64, now time.Time) (int64, error) {
	fixture.createCalls++
	links, _ := outbound.CanonicalCampaignHandoffLinks(snapshot.CustomerIDs)
	fixture.stored = outbound.AcceptedCampaignHandoff{ID: 9, CampaignCode: snapshot.CampaignCode, PlanID: snapshot.PlanID, ReviewVersion: snapshot.ReviewVersion, SourceDigest: snapshot.SourceDigest, TargetDigest: snapshot.TargetDigest, ContentDigest: snapshot.ContentDigest, TargetCount: int32(len(links)), StepCount: int32(len(snapshot.Steps)), Status: outbound.CampaignHandoffHeld, AcceptedBy: actorID, AcceptedAt: now, Safety: outbound.LocalCampaignHandoffSafety(), Steps: append([]outbound.CampaignHandoffStep(nil), snapshot.Steps...), Links: links}
	return 9, nil
}
func (fixture *campaignHandoffFixture) ReadAcceptedCampaignHandoff(context.Context, string, string) (outbound.AcceptedCampaignHandoff, error) {
	value := fixture.stored
	if fixture.readbackTampered {
		value.TargetCount++
	}
	if fixture.readbackLinkTampered {
		value.Links[0].CustomerID++
	}
	return value, nil
}
func (fixture *campaignHandoffFixture) ReadCampaignHandoffSummary(context.Context, string, string) (outbound.CampaignHandoffSummary, error) {
	return outbound.SummaryOf(fixture.stored), nil
}
func (fixture *campaignHandoffFixture) CompleteCampaignHandoffReceipt(_ context.Context, id, eventID int64, result outbound.CampaignHandoffSummary, _ time.Time) error {
	fixture.completeCalls++
	if id != fixture.receipt.ID || eventID != fixture.eventID {
		return errors.New("wrong receipt completion")
	}
	fixture.receipt.State, fixture.receipt.Result = campaignHandoffReceiptCompleted, &result
	return nil
}
func (fixture *campaignHandoffFixture) AppendCampaignHandoffFact(ctx context.Context, value outboundport.CampaignHandoffEvent) (int64, error) {
	fixture.eventCalls++
	if ctx.Value(campaignHandoffTxKey{}) == nil || value.HandoffID != 9 {
		return 0, errors.New("event outside outer tx")
	}
	return fixture.eventID, nil
}

func TestCampaignHandoffAcceptUsesOuterUoWAndReplaysBeforeCampaignOrEvent(t *testing.T) {
	fixture := newCampaignHandoffFixture()
	service, err := NewCampaignHandoffService(fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 23, 3, 4, 5, 123456000, time.UTC) }
	command := campaignHandoffCommand()
	first, err := service.Accept(context.Background(), command)
	if err != nil || !outbound.ValidCampaignHandoffSummary(first) || fixture.sourceCalls != 1 || fixture.createCalls != 1 || fixture.eventCalls != 1 || fixture.completeCalls != 1 {
		t.Fatalf("first=%+v err=%v calls=%d/%d/%d/%d", first, err, fixture.sourceCalls, fixture.createCalls, fixture.eventCalls, fixture.completeCalls)
	}
	replay, err := service.Accept(context.Background(), command)
	if err != nil || replay != first || fixture.sourceCalls != 1 || fixture.createCalls != 1 || fixture.eventCalls != 1 || fixture.completeCalls != 1 || fixture.uowCalls != 2 {
		t.Fatalf("replay=%+v err=%v calls=%d/%d/%d/%d uow=%d", replay, err, fixture.sourceCalls, fixture.createCalls, fixture.eventCalls, fixture.completeCalls, fixture.uowCalls)
	}
}

func TestCampaignHandoffAcceptRejectsTamperedStrictReadback(t *testing.T) {
	fixture := newCampaignHandoffFixture()
	fixture.readbackTampered = true
	service, _ := NewCampaignHandoffService(fixture, fixture, fixture, fixture)
	service.now = func() time.Time { return time.Date(2026, 8, 23, 3, 4, 5, 0, time.UTC) }
	if _, err := service.Accept(context.Background(), campaignHandoffCommand()); !errors.Is(err, outbound.ErrCampaignHandoffUnavailable) || fixture.completeCalls != 0 {
		t.Fatalf("err=%v complete=%d", err, fixture.completeCalls)
	}
}

func TestCampaignHandoffAcceptRejectsTamperedLinkReadback(t *testing.T) {
	fixture := newCampaignHandoffFixture()
	fixture.readbackLinkTampered = true
	service, _ := NewCampaignHandoffService(fixture, fixture, fixture, fixture)
	service.now = func() time.Time { return time.Date(2026, 8, 23, 3, 4, 5, 0, time.UTC) }
	if _, err := service.Accept(context.Background(), campaignHandoffCommand()); !errors.Is(err, outbound.ErrCampaignHandoffUnavailable) || fixture.completeCalls != 0 {
		t.Fatalf("err=%v complete=%d", err, fixture.completeCalls)
	}
}

func newCampaignHandoffFixture() *campaignHandoffFixture {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return &campaignHandoffFixture{eventID: 11, snapshot: outboundport.ApprovedCampaignHandoffSnapshot{
		CampaignCode: "spring-campaign", PlanID: "ctp_" + digest, ReviewVersion: 3,
		SourceDigest: digest, TargetDigest: digest, ContentDigest: digest,
		CustomerIDs: []int64{7, 3}, Steps: []outbound.CampaignHandoffStep{{Index: 1, Content: "hello"}},
		ApprovedAt: time.Date(2026, 8, 23, 2, 0, 0, 0, time.UTC),
	}}
}
func campaignHandoffCommand() AcceptCampaignHandoffCommand {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return AcceptCampaignHandoffCommand{CampaignCode: "spring-campaign", PlanID: "ctp_" + digest, ExpectedReviewVersion: 3, ActorID: 7, IdempotencyKey: "aaaaaaaaaaaaaaaa"}
}
