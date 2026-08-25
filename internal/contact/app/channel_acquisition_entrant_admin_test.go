package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type entrantAdminStoreStub struct {
	command       ReconcileChannelAcquisitionEntrantReceiptCommand
	keyDigest     string
	payloadDigest string
	item          ChannelAcquisitionEntrantReceiptItem
}

func (*entrantAdminStoreStub) ListChannelAcquisitionEntrantReceipts(context.Context, int64, int64, int, int64) ([]ChannelAcquisitionEntrantReceiptItem, error) {
	return nil, errors.New("unexpected channel list")
}
func (*entrantAdminStoreStub) GetChannelAcquisitionEntrantReceipt(context.Context, int64, int64, int64) (ChannelAcquisitionEntrantReceiptItem, error) {
	return ChannelAcquisitionEntrantReceiptItem{}, errors.New("unexpected channel get")
}
func (*entrantAdminStoreStub) ListUnassignedChannelAcquisitionEntrantReceipts(context.Context, int64, int, int64) ([]ChannelAcquisitionEntrantReceiptItem, error) {
	return nil, nil
}
func (*entrantAdminStoreStub) GetUnassignedChannelAcquisitionEntrantReceipt(context.Context, int64, int64) (ChannelAcquisitionEntrantReceiptItem, error) {
	return ChannelAcquisitionEntrantReceiptItem{}, nil
}
func (store *entrantAdminStoreStub) ReconcileChannelAcquisitionEntrantReceipt(_ context.Context, command ReconcileChannelAcquisitionEntrantReceiptCommand, keyDigest, payloadDigest string) (ChannelAcquisitionEntrantReceiptItem, error) {
	store.command, store.keyDigest, store.payloadDigest = command, keyDigest, payloadDigest
	return store.item, nil
}

func TestUnassignedEntrantReconcileCannotAcceptClientChannelOrCorpScope(t *testing.T) {
	store := &entrantAdminStoreStub{item: ChannelAcquisitionEntrantReceiptItem{ReceiptID: 91, ChannelID: 41, EffectID: "eer_7"}}
	codec, err := NewChannelAcquisitionEntrantReceiptCursorCodec([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewChannelAcquisitionEntrantReceiptService(channelUOW{}, store, codec)
	if err != nil {
		t.Fatal(err)
	}
	command := ReconcileChannelAcquisitionEntrantReceiptCommand{ActorID: 11, ReceiptID: 91, EffectID: "eer_7", CustomerID: 22, Reason: "verified", IdempotencyKey: "reconcile-key-0001"}
	result, err := service.ReconcileUnassigned(context.Background(), command)
	if err != nil || result.ChannelID != 41 || !store.command.Unassigned || store.command.ChannelID != 0 || store.keyDigest == "" || store.payloadDigest == "" {
		t.Fatalf("result=%#v command=%#v key=%q payload=%q err=%v", result, store.command, store.keyDigest, store.payloadDigest, err)
	}
	command.ChannelID = 99
	if _, err = service.ReconcileUnassigned(context.Background(), command); !errors.Is(err, ErrInvalidChannelAcquisitionEntrantReceipt) {
		t.Fatalf("client channel error=%v", err)
	}
}

func TestUnassignedEntrantListUsesIndependentCursor(t *testing.T) {
	store := &entrantAdminStoreStub{}
	codec, err := NewChannelAcquisitionEntrantReceiptCursorCodec([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return time.Unix(1700000000, 0) }
	service, err := NewChannelAcquisitionEntrantReceiptService(channelUOW{}, store, codec)
	if err != nil {
		t.Fatal(err)
	}
	channelCursor, err := codec.Encode(11, 41, 91)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ListUnassigned(context.Background(), UnassignedChannelAcquisitionEntrantReceiptListInput{ActorID: 11, Cursor: channelCursor}); !errors.Is(err, ErrInvalidChannelAcquisitionEntrantReceipt) {
		t.Fatalf("channel cursor entered unassigned list: %v", err)
	}
}
