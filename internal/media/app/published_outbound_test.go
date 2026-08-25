package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type publishedUOWStub struct{ calls int }

func (u *publishedUOWStub) Within(ctx context.Context, fn func(context.Context) error) error {
	u.calls++
	return fn(ctx)
}

type publishedReaderStub struct{}

func (publishedReaderStub) ReadPublishedPackage(context.Context, int64) (PublishedPackage, error) {
	return PublishedPackage{ID: 1, Enabled: true, Snapshot: "snap", MediaRefs: json.RawMessage("[{\"kind\":\"attachment\",\"id\":7}]")}, nil
}

type publishedRuntimeStub struct{ calls int }

func (s *publishedRuntimeStub) Accept(_ context.Context, _ eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	s.calls++
	return eer.Projection{ID: "eer_7", Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMedia, State: eer.StateAccepted}, eer.OperationReceipt{}, nil
}

type effectBinderStub struct {
	acceptance PublishedOutboundAcceptance
	err        error
}

func (s *effectBinderStub) BindOutboundMediaAcceptance(_ context.Context, value PublishedOutboundAcceptance) (bool, error) {
	s.acceptance = value
	return false, s.err
}

func TestPublishedOutboundAcceptUsesOneUOWAndPersistsSnapshot(t *testing.T) {
	uow := &publishedUOWStub{}
	runtime := &publishedRuntimeStub{}
	binder := &effectBinderStub{}
	service := NewPublishedOutboundService(uow, publishedReaderStub{}, NewOutboundMediaService(runtime), binder)
	projection, replay, err := service.AcceptPublishedContentPackageForOutbound(context.Background(), 1, "target_7", "published-outbound-key-0001")
	if err != nil || replay || uow.calls != 1 || runtime.calls != 1 || projection.ID != "eer_7" {
		t.Fatalf("projection=%+v replay=%v uow=%d runtime=%d err=%v", projection, replay, uow.calls, runtime.calls, err)
	}
	if binder.acceptance.ContentPackageID != 1 || binder.acceptance.EffectID != 7 || binder.acceptance.SourceDigest == "" || binder.acceptance.PayloadDigest == "" || binder.acceptance.TargetDigest == "" || string(binder.acceptance.MediaRefs) != "[{\"kind\":\"attachment\",\"id\":7}]" {
		t.Fatalf("acceptance=%+v", binder.acceptance)
	}
}

func TestPublishedOutboundBindingFailureLeavesOuterUOWFailed(t *testing.T) {
	uow := &publishedUOWStub{}
	binder := &effectBinderStub{err: errors.New("binding conflict")}
	service := NewPublishedOutboundService(uow, publishedReaderStub{}, NewOutboundMediaService(&publishedRuntimeStub{}), binder)
	if _, _, err := service.AcceptPublishedContentPackageForOutbound(context.Background(), 1, "target_7", "published-outbound-key-0002"); err == nil || uow.calls != 1 {
		t.Fatalf("uow=%d err=%v", uow.calls, err)
	}
}

func TestDecodeEERIDStrict(t *testing.T) {
	if _, e := decodeEERID("eer_1"); e != nil {
		t.Fatal(e)
	}
	if _, e := decodeEERID("1"); e == nil {
		t.Fatal("accepted non opaque")
	}
}
