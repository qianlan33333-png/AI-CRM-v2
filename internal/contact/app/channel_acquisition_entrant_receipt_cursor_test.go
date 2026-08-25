package app

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestChannelAcquisitionEntrantReceiptCursorBindsActorAndChannel(t *testing.T) {
	codec, err := NewChannelAcquisitionEntrantReceiptCursorCodec([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return time.Unix(1700000000, 0) }
	token, err := codec.Encode(11, 41, 91)
	if err != nil {
		t.Fatal(err)
	}
	if got, decodeErr := codec.Decode(token, 11, 41); decodeErr != nil || got != 91 {
		t.Fatalf("got=%d err=%v", got, decodeErr)
	}
	if _, decodeErr := codec.Decode(token, 12, 41); !errors.Is(decodeErr, ErrInvalidChannelAcquisitionEntrantReceipt) {
		t.Fatalf("cross actor err=%v", decodeErr)
	}
	if _, decodeErr := codec.Decode(token, 11, 42); !errors.Is(decodeErr, ErrInvalidChannelAcquisitionEntrantReceipt) {
		t.Fatalf("cross channel err=%v", decodeErr)
	}
	codec.now = func() time.Time { return time.Unix(1700001000, 0) }
	if _, decodeErr := codec.Decode(token, 11, 41); !errors.Is(decodeErr, ErrInvalidChannelAcquisitionEntrantReceipt) {
		t.Fatalf("expired err=%v", decodeErr)
	}
}

func TestChannelAcquisitionEntrantReceiptUnassignedCursorHasIndependentScope(t *testing.T) {
	codec, err := NewChannelAcquisitionEntrantReceiptCursorCodec([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return time.Unix(1700000000, 0) }
	token, err := codec.EncodeUnassigned(11, 91)
	if err != nil {
		t.Fatal(err)
	}
	if got, decodeErr := codec.DecodeUnassigned(token, 11); decodeErr != nil || got != 91 {
		t.Fatalf("got=%d err=%v", got, decodeErr)
	}
	if _, decodeErr := codec.Decode(token, 11, 41); !errors.Is(decodeErr, ErrInvalidChannelAcquisitionEntrantReceipt) {
		t.Fatalf("unassigned cursor entered channel scope: %v", decodeErr)
	}
	channelToken, err := codec.Encode(11, 41, 91)
	if err != nil {
		t.Fatal(err)
	}
	if _, decodeErr := codec.DecodeUnassigned(channelToken, 11); !errors.Is(decodeErr, ErrInvalidChannelAcquisitionEntrantReceipt) {
		t.Fatalf("channel cursor entered unassigned scope: %v", decodeErr)
	}
}
