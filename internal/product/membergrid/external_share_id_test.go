package membergrid

import (
	"bytes"
	"context"
	"testing"
)

func TestRandomExternalShareIDFactoryProducesClosedOpaqueID(t *testing.T) {
	factory := &RandomExternalShareIDFactory{reader: bytes.NewReader(bytes.Repeat([]byte{0x5a}, 24))}
	shareID, err := factory.NewExternalShareID(context.Background())
	if err != nil || !validExternalShareID(shareID) || len(shareID) != 32 {
		t.Fatalf("share_id=%q err=%v", shareID, err)
	}
}
