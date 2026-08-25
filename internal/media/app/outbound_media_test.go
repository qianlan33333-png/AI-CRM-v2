package app

import (
	"context"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	"testing"
	"time"
)

type mediaRuntimeStub struct{ command eer.AcceptCommand }

func (s *mediaRuntimeStub) Accept(_ context.Context, c eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	s.command = c
	return eer.Projection{ID: "effect", Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMedia, State: eer.StateAccepted, Generation: 1, UpdatedAt: time.Now()}, eer.OperationReceipt{}, nil
}
func TestAcceptOutboundMediaUsesClosedAcceptedEnvelope(t *testing.T) {
	stub := &mediaRuntimeStub{}
	d := mediaEERDigest("x")
	p, e := NewOutboundMediaService(stub).AcceptOutboundMedia(context.Background(), OutboundMediaAcceptCommand{SourceDigest: d, TargetDigest: d, PayloadDigest: d, ReceiptKey: "key"})
	if e != nil || p.Kind != eer.KindOutboundMedia || stub.command.Envelope.Kind() != eer.KindOutboundMedia {
		t.Fatalf("%v %#v", e, p)
	}
}
