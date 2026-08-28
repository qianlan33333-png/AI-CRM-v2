package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

type radarClickHistoryReconcileReader struct {
	value radarport.HistoricalRadarClick
	err   error
}

func (reader *radarClickHistoryReconcileReader) GetHistoricalRadarClick(_ context.Context, id int64) (radarport.HistoricalRadarClick, error) {
	if reader == nil || reader.err != nil || reader.value.ID != id {
		return radarport.HistoricalRadarClick{}, errors.New("historical click unavailable")
	}
	return reader.value, nil
}

func (reader *radarClickHistoryReconcileReader) ListHistoricalRadarClick(context.Context, radarport.RadarClickHistoryQuery) ([]radarport.HistoricalRadarClick, int64, error) {
	if reader == nil || reader.err != nil {
		return nil, 0, errors.New("historical click unavailable")
	}
	return []radarport.HistoricalRadarClick{reader.value}, 1, nil
}

func TestVerifyRadarClickHistoryRowBindsCompleteTarget(t *testing.T) {
	reader, row := radarClickHistoryReconcileFixture(t)
	proof, err := verifyRadarClickHistoryRow(context.Background(), reader, row)
	if err != nil || proof != "history_only:"+hex.EncodeToString(row.TargetDigest) {
		t.Fatalf("proof=%q err=%v", proof, err)
	}

	for name, mutate := range map[string]func(*radarClickHistoryReconcileReader, *reconciliationRow){
		"source key hmac": func(_ *radarClickHistoryReconcileReader, row *reconciliationRow) { row.SourceKeyDigest[0]++ },
		"payload hmac":    func(_ *radarClickHistoryReconcileReader, row *reconciliationRow) { row.PayloadDigest[0]++ },
		"field hmac":      func(_ *radarClickHistoryReconcileReader, row *reconciliationRow) { row.FieldDigest[0]++ },
		"target digest":   func(_ *radarClickHistoryReconcileReader, row *reconciliationRow) { row.TargetDigest[0]++ },
		"source table":    func(_ *radarClickHistoryReconcileReader, row *reconciliationRow) { row.TableID = "public/radar_links" },
		"target domain": func(_ *radarClickHistoryReconcileReader, row *reconciliationRow) {
			value := "campaign"
			row.TargetDomain = &value
		},
		"target table": func(_ *radarClickHistoryReconcileReader, row *reconciliationRow) {
			value := "radar_links"
			row.TargetTable = &value
		},
		"non canonical id": func(_ *radarClickHistoryReconcileReader, row *reconciliationRow) {
			value := "071"
			row.TargetID = &value
		},
		"actual target id":     func(reader *radarClickHistoryReconcileReader, _ *reconciliationRow) { reader.value.ID++ },
		"private field digest": func(reader *radarClickHistoryReconcileReader, _ *reconciliationRow) { reader.value.OpenIDDigest[0]++ },
		"reader error": func(reader *radarClickHistoryReconcileReader, _ *reconciliationRow) {
			reader.err = errors.New("unavailable")
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate, target := radarClickHistoryReconcileFixture(t)
			mutate(candidate, &target)
			if _, err := verifyRadarClickHistoryRow(context.Background(), candidate, target); !errors.Is(err, ErrConflict) {
				t.Fatalf("err=%v", err)
			}
		})
	}

	_, row = radarClickHistoryReconcileFixture(t)
	var typedNil *radarClickHistoryReconcileReader
	if _, err := verifyRadarClickHistoryRow(context.Background(), typedNil, row); !errors.Is(err, ErrConflict) {
		t.Fatalf("typed nil err=%v", err)
	}
}

func TestReconcileRadarClickHistoryRejectsWrongVersionBeforeDatabase(t *testing.T) {
	var pool *pgxpool.Pool
	if _, err := ReconcileRadarClickHistory(context.Background(), pool, "v1-radar-click-history-a2", "archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("err=%v", err)
	}
}

func radarClickHistoryReconcileFixture(t *testing.T) (*radarClickHistoryReconcileReader, reconciliationRow) {
	t.Helper()
	value := radarport.HistoricalRadarClick{
		ID: 71, SourceID: 17, LinkSourceID: 18, Code: "legacy-code", RawStage: "observed", SourceChannel: "", TargetTypeSnapshot: "article", SourceChannelSnapshot: "manual", ErrorCode: "",
		CreatedAt:       time.Date(2026, 8, 28, 8, 9, 10, 123456000, time.UTC),
		SourceKeyDigest: radarClickHistoryDigest(1), SourcePayloadDigest: radarClickHistoryDigest(2), SourceFieldDigest: radarClickHistoryDigest(3),
		OpenIDDigest: radarClickHistoryDigest(4), UnionIDDigest: radarClickHistoryDigest(5), ExternalUserIDDigest: radarClickHistoryDigest(6), CampaignIDDigest: radarClickHistoryDigest(7),
		StaffIDDigest: radarClickHistoryDigest(8), UserAgentDigest: radarClickHistoryDigest(9), IPDigest: radarClickHistoryDigest(10), PersonIDDigest: radarClickHistoryDigest(11),
		IPHashDigest: radarClickHistoryDigest(12), CampaignSnapshotDigest: radarClickHistoryDigest(13), StaffSnapshotDigest: radarClickHistoryDigest(14), RefererDigest: radarClickHistoryDigest(15), QueryParamsDigest: radarClickHistoryDigest(16),
	}
	digest, err := radarapp.HistoricalRadarClickDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	domain, targetTable, targetID := "radar", "radar_v1_click_history", strconv.FormatInt(value.ID, 10)
	return &radarClickHistoryReconcileReader{value: value}, reconciliationRow{
		TableID: "public/radar_click_events", SourceKeyDigest: value.SourceKeyDigest[:], PayloadDigest: value.SourcePayloadDigest[:], FieldDigest: value.SourceFieldDigest[:],
		TargetDomain: &domain, TargetTable: &targetTable, TargetID: &targetID, TargetDigest: digest[:],
	}
}

func radarClickHistoryDigest(first byte) [sha256.Size]byte {
	var value [sha256.Size]byte
	value[0] = first
	return value
}
