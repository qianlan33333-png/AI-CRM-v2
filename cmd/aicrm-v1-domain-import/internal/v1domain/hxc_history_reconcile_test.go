package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxchistory"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestVerifyHXCHistoryRowChecksEveryImmutableTarget(t *testing.T) {
	reader := newHXCHistoryReconcileReader()
	tests := []struct {
		table, target string
		value         any
	}{
		{v1hxchistory.DashboardMetaTableID, hxcHistoryMetaTarget, reader.meta},
		{v1hxchistory.DashboardSnapshotTableID, hxcHistorySnapshotTarget, reader.snapshot},
		{v1hxchistory.ActivationStatusTableID, hxcHistoryActivationTarget, reader.activations[3]},
		{v1hxchistory.HuangxiaocanActivationID, hxcHistoryActivationTarget, reader.activations[4]},
		{v1hxchistory.ExperienceLeadsTableID, hxcHistoryLeadTarget, reader.lead},
		{v1hxchistory.ImportBatchesTableID, hxcHistoryBatchTarget, reader.batch},
	}
	for _, test := range tests {
		t.Run(test.table, func(t *testing.T) {
			row := hxcHistoryReconcileRow(t, test.table, test.target, test.value)
			proof, err := verifyHXCHistoryRow(context.Background(), reader, row)
			if err != nil || proof == "" {
				t.Fatalf("proof=%q err=%v", proof, err)
			}
		})
	}
}

func TestVerifyHXCHistoryRowRejectsTargetOrSourceDrift(t *testing.T) {
	reader := newHXCHistoryReconcileReader()
	row := hxcHistoryReconcileRow(t, v1hxchistory.ActivationStatusTableID, hxcHistoryActivationTarget, reader.activations[3])
	row.TargetDigest = append([]byte(nil), row.TargetDigest...)
	row.TargetDigest[0]++
	if _, err := verifyHXCHistoryRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("digest drift err=%v", err)
	}
	row = hxcHistoryReconcileRow(t, v1hxchistory.ActivationStatusTableID, hxcHistoryActivationTarget, reader.activations[3])
	row.TableID = v1hxchistory.HuangxiaocanActivationID
	if _, err := verifyHXCHistoryRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("activation source drift err=%v", err)
	}
	row = hxcHistoryReconcileRow(t, v1hxchistory.DashboardMetaTableID, hxcHistoryMetaTarget, reader.meta)
	badKey := digestByte(99)
	row.SourceKeyDigest = badKey[:]
	if _, err := verifyHXCHistoryRow(context.Background(), reader, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("source-key drift err=%v", err)
	}
}

func TestReconcileHXCHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	if _, err := ReconcileHXCHistory(context.Background(), nil, "v1-hxc-history-a2", "archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("err=%v", err)
	}
}

type hxcHistoryReconcileReader struct {
	meta        hxcport.HistoricalHXCMeta
	snapshot    hxcport.HistoricalHXCSnapshot
	activations map[int64]hxcport.HistoricalHXCActivation
	lead        hxcport.HistoricalHXCLead
	batch       hxcport.HistoricalHXCBatch
}

var _ hxcport.HXCHistoryReader = hxcHistoryReconcileReader{}

func (reader hxcHistoryReconcileReader) GetHistoricalHXCMeta(_ context.Context, id int64) (hxcport.HistoricalHXCMeta, error) {
	if id != reader.meta.ID {
		return hxcport.HistoricalHXCMeta{}, hxcport.ErrHXCHistoryUnavailable
	}
	return reader.meta, nil
}

func (reader hxcHistoryReconcileReader) ListHistoricalHXCMeta(context.Context, hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCMeta, int64, error) {
	return []hxcport.HistoricalHXCMeta{reader.meta}, 1, nil
}

func (reader hxcHistoryReconcileReader) GetHistoricalHXCSnapshot(_ context.Context, id int64) (hxcport.HistoricalHXCSnapshot, error) {
	if id != reader.snapshot.ID {
		return hxcport.HistoricalHXCSnapshot{}, hxcport.ErrHXCHistoryUnavailable
	}
	return reader.snapshot, nil
}

func (reader hxcHistoryReconcileReader) ListHistoricalHXCSnapshot(context.Context, hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCSnapshot, int64, error) {
	return []hxcport.HistoricalHXCSnapshot{reader.snapshot}, 1, nil
}

func (reader hxcHistoryReconcileReader) GetHistoricalHXCActivation(_ context.Context, id int64) (hxcport.HistoricalHXCActivation, error) {
	value, ok := reader.activations[id]
	if !ok {
		return hxcport.HistoricalHXCActivation{}, hxcport.ErrHXCHistoryUnavailable
	}
	return value, nil
}

func (reader hxcHistoryReconcileReader) ListHistoricalHXCActivation(context.Context, hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCActivation, int64, error) {
	return []hxcport.HistoricalHXCActivation{}, 0, nil
}

func (reader hxcHistoryReconcileReader) GetHistoricalHXCLead(_ context.Context, id int64) (hxcport.HistoricalHXCLead, error) {
	if id != reader.lead.ID {
		return hxcport.HistoricalHXCLead{}, hxcport.ErrHXCHistoryUnavailable
	}
	return reader.lead, nil
}

func (reader hxcHistoryReconcileReader) ListHistoricalHXCLead(context.Context, hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCLead, int64, error) {
	return []hxcport.HistoricalHXCLead{reader.lead}, 1, nil
}

func (reader hxcHistoryReconcileReader) GetHistoricalHXCBatch(_ context.Context, id int64) (hxcport.HistoricalHXCBatch, error) {
	if id != reader.batch.ID {
		return hxcport.HistoricalHXCBatch{}, hxcport.ErrHXCHistoryUnavailable
	}
	return reader.batch, nil
}

func (reader hxcHistoryReconcileReader) ListHistoricalHXCBatch(context.Context, hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCBatch, int64, error) {
	return []hxcport.HistoricalHXCBatch{reader.batch}, 1, nil
}

func newHXCHistoryReconcileReader() hxcHistoryReconcileReader {
	at := time.Date(2026, 8, 28, 15, 0, 0, 123456000, time.UTC)
	identity := func(id, sourceID int64, key, payload byte) hxcport.HistoricalHXCIdentity {
		return hxcport.HistoricalHXCIdentity{ID: id, SourceID: sourceID, SourceKeyDigest: digestByte(key), SourcePayloadDigest: digestByte(payload)}
	}
	return hxcHistoryReconcileReader{
		meta:     hxcport.HistoricalHXCMeta{HistoricalHXCIdentity: identity(1, -1, 11, 21), StartedAt: at, Status: "finished", TriggerSource: "snapshot"},
		snapshot: hxcport.HistoricalHXCSnapshot{HistoricalHXCIdentity: identity(2, 0, 12, 22), Observation: v1hxchistory.ObservedSnapshot, ObservedAt: at},
		activations: map[int64]hxcport.HistoricalHXCActivation{
			3: {HistoricalHXCIdentity: identity(3, 1, 13, 23), SourceTable: v1hxchistory.ActivationStatusTableID, OriginalState: "old", CreatedAt: at, UpdatedAt: at},
			4: {HistoricalHXCIdentity: identity(4, 2, 14, 24), SourceTable: v1hxchistory.HuangxiaocanActivationID, OriginalState: "old", CreatedAt: at, UpdatedAt: at},
		},
		lead:  hxcport.HistoricalHXCLead{HistoricalHXCIdentity: identity(5, -2, 15, 25), OriginalType: "legacy", CreatedAt: at, UpdatedAt: at},
		batch: hxcport.HistoricalHXCBatch{HistoricalHXCIdentity: identity(6, 3, 16, 26), ImportType: "legacy", CreatedAt: at},
	}
}

func hxcHistoryReconcileRow(t *testing.T, table, target string, value any) reconciliationRow {
	t.Helper()
	var identity hxcport.HistoricalHXCIdentity
	var digest [sha256.Size]byte
	var err error
	switch typed := value.(type) {
	case hxcport.HistoricalHXCMeta:
		identity = typed.HistoricalHXCIdentity
		digest, err = hxcapp.HistoricalHXCMetaDigest(typed)
	case hxcport.HistoricalHXCSnapshot:
		identity = typed.HistoricalHXCIdentity
		digest, err = hxcapp.HistoricalHXCSnapshotDigest(typed)
	case hxcport.HistoricalHXCActivation:
		identity = typed.HistoricalHXCIdentity
		digest, err = hxcapp.HistoricalHXCActivationDigest(typed)
	case hxcport.HistoricalHXCLead:
		identity = typed.HistoricalHXCIdentity
		digest, err = hxcapp.HistoricalHXCLeadDigest(typed)
	case hxcport.HistoricalHXCBatch:
		identity = typed.HistoricalHXCIdentity
		digest, err = hxcapp.HistoricalHXCBatchDigest(typed)
	default:
		t.Fatalf("unexpected value %T", value)
	}
	if err != nil {
		t.Fatal(err)
	}
	domain, targetTable, id := hxcHistoryDomain, target, strconv.FormatInt(identity.ID, 10)
	return reconciliationRow{TableID: table, SourceKeyDigest: identity.SourceKeyDigest[:], PayloadDigest: identity.SourcePayloadDigest[:], TargetDomain: &domain, TargetTable: &targetTable, TargetID: &id, TargetDigest: digest[:]}
}
