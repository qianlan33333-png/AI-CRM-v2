package store

import (
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

func TestCH02CorrelationResolutionIsExactAndFailClosedOnMultiplicity(t *testing.T) {
	if got := channelAcquisitionCorrelationResolution(nil); got.Cardinality != contactport.AcquisitionAssetCorrelationZero {
		t.Fatalf("zero=%+v", got)
	}
	row := contactdb.ResolveChannelAcquisitionAssetCorrelationRow{EffectID: 41, ChannelID: 7, AssetKind: string(contactport.AcquisitionAssetQRCode), AssetVersion: 3}
	if got := channelAcquisitionCorrelationResolution([]contactdb.ResolveChannelAcquisitionAssetCorrelationRow{row}); got.Cardinality != contactport.AcquisitionAssetCorrelationOne || got.Match.EffectID != "eer_41" || got.Match.ChannelID != 7 || got.Match.AssetVersion != 3 {
		t.Fatalf("one=%+v", got)
	}
	if got := channelAcquisitionCorrelationResolution([]contactdb.ResolveChannelAcquisitionAssetCorrelationRow{row, row}); got.Cardinality != contactport.AcquisitionAssetCorrelationMultiple || got.Match != (contactport.AcquisitionAssetCorrelationMatch{}) {
		t.Fatalf("multiple=%+v", got)
	}
}
