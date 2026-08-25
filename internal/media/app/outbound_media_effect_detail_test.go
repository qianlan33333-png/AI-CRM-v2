package app

import (
	"context"
	"testing"
)

type outboundMediaEffectDetailReaderStub struct {
	contentPackageID int64
	targetDigest     string
}

type outboundMediaEffectDetailUOW struct{ calls int }

func (u *outboundMediaEffectDetailUOW) Within(ctx context.Context, operation func(context.Context) error) error {
	u.calls++
	return operation(ctx)
}

func (s *outboundMediaEffectDetailReaderStub) ReadOutboundMediaEffectDetail(_ context.Context, contentPackageID int64, targetDigest string) (OutboundMediaEffectDetail, error) {
	s.contentPackageID, s.targetDigest = contentPackageID, targetDigest
	return OutboundMediaEffectDetail{ContentPackageID: contentPackageID, EffectID: "eer_7", State: "accepted"}, nil
}

func TestOutboundMediaEffectDetailServiceHashesOpaqueTarget(t *testing.T) {
	reader := &outboundMediaEffectDetailReaderStub{}
	uow := &outboundMediaEffectDetailUOW{}
	detail, err := NewOutboundMediaEffectDetailService(uow, reader).ReadOutboundMediaEffectDetail(context.Background(), 42, "external_contact_7")
	if err != nil || detail.EffectID != "eer_7" || uow.calls != 1 || reader.contentPackageID != 42 || reader.targetDigest != mediaEERDigest("outbound-media-target", "external_contact_7") {
		t.Fatalf("detail=%#v uow=%#v reader=%#v err=%v", detail, uow, reader, err)
	}
}
