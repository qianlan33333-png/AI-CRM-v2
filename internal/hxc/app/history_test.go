package app

import (
	"testing"
	"time"

	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestHistoricalHXCAllowsSourceTimestampOrdering(t *testing.T) {
	later := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
	earlier := later.Add(-time.Second)
	if _, err := HistoricalHXCMetaDigest(hxc.HistoricalHXCMeta{HistoricalHXCIdentity: historyTestIdentity(1), StartedAt: later, FinishedAt: &earlier}); err != nil {
		t.Fatalf("meta source order rejected: %v", err)
	}
	if _, err := HistoricalHXCActivationDigest(hxc.HistoricalHXCActivation{HistoricalHXCIdentity: historyTestIdentity(2), SourceTable: "public/user_ops_activation_status_source", CreatedAt: later, UpdatedAt: earlier}); err != nil {
		t.Fatalf("activation source order rejected: %v", err)
	}
	if _, err := HistoricalHXCLeadDigest(hxc.HistoricalHXCLead{HistoricalHXCIdentity: historyTestIdentity(3), CreatedAt: later, UpdatedAt: earlier}); err != nil {
		t.Fatalf("lead source order rejected: %v", err)
	}
}

func TestHistoricalHXCSnapshotRequiresObservedProjection(t *testing.T) {
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
	value := hxc.HistoricalHXCSnapshot{HistoricalHXCIdentity: historyTestIdentity(4), Observation: "observed_snapshot", ObservedAt: at}
	if _, err := HistoricalHXCSnapshotDigest(value); err != nil {
		t.Fatalf("valid observed projection rejected: %v", err)
	}
	for _, observation := range []string{"", "other"} {
		value.Observation = observation
		if _, err := HistoricalHXCSnapshotDigest(value); err == nil {
			t.Fatalf("invalid observation accepted: %q", observation)
		}
	}
}

func historyTestIdentity(first byte) hxc.HistoricalHXCIdentity {
	var key, payload [32]byte
	key[0], payload[0] = first, first+20
	return hxc.HistoricalHXCIdentity{ID: 1, SourceKeyDigest: key, SourcePayloadDigest: payload}
}
