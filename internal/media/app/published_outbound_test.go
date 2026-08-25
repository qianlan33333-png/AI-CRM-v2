package app

import (
	"context"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	"testing"
	"time"
)

type publishedReaderStub struct{}

func (publishedReaderStub) ReadPublishedPackage(context.Context, int64) (PublishedPackage, error) {
	return PublishedPackage{ID: 1, Enabled: true, Snapshot: "snap"}, nil
}

type effectBinderStub struct{}

func (effectBinderStub) BindOutboundMediaEffect(context.Context, int64, string, string, int64, time.Time) (bool, error) {
	return false, nil
}
func TestDecodeEERIDStrict(t *testing.T) {
	if _, e := decodeEERID("eer_1"); e != nil {
		t.Fatal(e)
	}
	if _, e := decodeEERID("1"); e == nil {
		t.Fatal("accepted non opaque")
	}
}

var _ = eer.StateAccepted
