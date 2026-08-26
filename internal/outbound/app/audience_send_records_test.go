package app

import (
	"context"
	"errors"
	"testing"
	"time"

	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type audienceSendRecordFixture struct {
	present bool
	items   []outboundport.AudienceSendRecord
	total   int64
	item    outboundport.AudienceSendRecord
	err     error
}

func (fixture *audienceSendRecordFixture) AudiencePackageExists(context.Context, int64) (bool, error) {
	return fixture.present, fixture.err
}
func (fixture *audienceSendRecordFixture) ListAudienceSendRecords(context.Context, int64, int32, int32) ([]outboundport.AudienceSendRecord, int64, error) {
	return fixture.items, fixture.total, fixture.err
}
func (fixture *audienceSendRecordFixture) GetAudienceSendRecord(context.Context, int64, int64) (outboundport.AudienceSendRecord, error) {
	return fixture.item, fixture.err
}

func TestAudienceSendRecordsExposeOnlyClosedReceiptFacts(t *testing.T) {
	now := time.Date(2026, 8, 26, 7, 0, 0, 0, time.UTC)
	record := outboundport.AudienceSendRecord{ID: 41, State: outbound.CampaignDispatchReconciled, TechnicalAttemptCount: 1, ProviderResultReceived: true, ReceiptPresent: true, DeliveryProven: true, BusinessCallDispatched: true, RealExternalCallExecuted: true, CreatedAt: now, UpdatedAt: now}
	fixture := &audienceSendRecordFixture{present: true, items: []outboundport.AudienceSendRecord{record}, total: 1, item: record}
	service, err := NewAudienceSendRecordService(fixture)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.List(context.Background(), 7, 20, 0)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0] != record {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	item, err := service.Get(context.Background(), 7, 41)
	if err != nil || item != record {
		t.Fatalf("item=%#v err=%v", item, err)
	}
}

func TestAudienceSendRecordsFailClosed(t *testing.T) {
	service, err := NewAudienceSendRecordService(&audienceSendRecordFixture{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.List(context.Background(), 7, 20, 0); !errors.Is(err, outbound.ErrCampaignHandoffNotFound) {
		t.Fatalf("missing package error=%v", err)
	}
	if _, err = service.List(context.Background(), 7, 101, 0); !errors.Is(err, outbound.ErrCampaignDispatchInvalid) {
		t.Fatalf("invalid limit error=%v", err)
	}
	invalid := outboundport.AudienceSendRecord{ID: 1, State: outbound.CampaignDispatchReconciled, DeliveryProven: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	service, _ = NewAudienceSendRecordService(&audienceSendRecordFixture{present: true, items: []outboundport.AudienceSendRecord{invalid}, total: 1})
	if _, err = service.List(context.Background(), 7, 20, 0); !errors.Is(err, outbound.ErrCampaignDispatchUnavailable) {
		t.Fatalf("invalid receipt truth error=%v", err)
	}
}
