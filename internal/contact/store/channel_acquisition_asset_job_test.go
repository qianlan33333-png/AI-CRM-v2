package store

import (
	"context"
	"errors"
	"testing"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

func TestCH02RiverJobInserterUsesEffectIDOnlyDigestAndRequiresCallerUOW(t *testing.T) {
	first := channelAcquisitionAssetJobDigest(contactapp.ChannelAcquisitionAssetJobArgs{EffectID: "eer_41"})
	second := channelAcquisitionAssetJobDigest(contactapp.ChannelAcquisitionAssetJobArgs{EffectID: "eer_42"})
	if first == "" || second == "" || first == second || channelAcquisitionAssetJobDigest(contactapp.ChannelAcquisitionAssetJobArgs{}) != "" {
		t.Fatalf("job digests=%q/%q", first, second)
	}
	inserter := &ChannelAcquisitionAssetRiverJobInserter{}
	if _, err := inserter.Insert(context.Background(), contactapp.ChannelAcquisitionAssetJobArgs{EffectID: "eer_41"}, 2, time.Now()); !errors.Is(err, contactapp.ErrChannelAcquisitionAssetUnavailable) {
		t.Fatalf("unconfigured inserter err=%v", err)
	}
	if inserter, err := NewChannelAcquisitionAssetRiverJobInserter(nil); inserter != nil || !errors.Is(err, contactapp.ErrChannelAcquisitionAssetUnavailable) {
		t.Fatalf("nil-pool inserter=%v err=%v", inserter, err)
	}
}
