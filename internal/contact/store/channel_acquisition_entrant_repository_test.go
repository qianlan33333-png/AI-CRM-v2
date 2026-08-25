package store

import (
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestSameEntrantReplayKeepsAttributedTerminalOnTransientIdentityMiss(t *testing.T) {
	match := contactport.AcquisitionAssetCorrelationMatch{EffectID: "eer_41", ChannelID: 7, Kind: contactport.AcquisitionAssetQRCode, AssetVersion: 3}
	receipt := contactport.ChannelAcquisitionEntrantReceipt{Status: contactport.ChannelAcquisitionEntrantAttributed, EffectID: match.EffectID, ChannelID: match.ChannelID, Kind: match.Kind, AssetVersion: match.AssetVersion, CustomerID: 22, CustomerEventID: 16}
	if !sameEntrantReplay(receipt, contactport.ChannelAcquisitionEntrantCommand{Match: match, CustomerID: 0}) {
		t.Fatal("transient identity miss must replay terminal receipt")
	}
	if sameEntrantReplay(receipt, contactport.ChannelAcquisitionEntrantCommand{Match: match, CustomerID: 23}) {
		t.Fatal("different customer must conflict")
	}
	changed := match
	changed.AssetVersion++
	if sameEntrantReplay(receipt, contactport.ChannelAcquisitionEntrantCommand{Match: changed, CustomerID: 0}) {
		t.Fatal("different asset must conflict")
	}
}
