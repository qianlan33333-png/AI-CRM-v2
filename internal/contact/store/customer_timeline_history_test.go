package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

func TestCustomerTimelineHistoryReaderFailsClosed(t *testing.T) {
	ctx := context.Background()
	if _, err := NewCustomerTimelineHistoryStore().GetHistoricalCustomerTimelineEvent(ctx, 1); !errors.Is(err, contact.ErrCustomerTimelineHistoryUnavailable) {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*CustomerTimelineHistoryReader{nil, NewCustomerTimelineHistoryReader(nil), NewCustomerTimelineHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalCustomerTimelineEvents(ctx, contact.CustomerTimelineHistoryQuery{Limit: 1}); !errors.Is(err, contact.ErrCustomerTimelineHistoryUnavailable) {
			t.Fatal(err)
		}
	}
	for _, query := range []contact.CustomerTimelineHistoryQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewCustomerTimelineHistoryReader(nil).ListHistoricalCustomerTimelineEvents(ctx, query); !errors.Is(err, contact.ErrCustomerTimelineHistoryInvalid) {
			t.Fatal(err)
		}
	}
}

func TestCustomerTimelineHistoryKeepsPrivateEvidenceOutOfReadModel(t *testing.T) {
	at := time.Date(2026, 8, 29, 2, 3, 4, 123456000, time.UTC)
	row := contactdb.ContactV1CustomerTimelineHistory{
		ID: 1, SourceKeyDigest: timelineDigestBytes("key"), SourcePayloadDigest: timelineDigestBytes("payload"), SourceFieldDigest: timelineDigestBytes("field"),
		SourceID: 7, EventID: "evt", EventType: "opened", EventTime: timelineTimestamp(at), Title: "private-title", Summary: "private-summary",
		SourceTable: "orders", SourceValue: "42", MetadataJson: `{"private":"value"}`, CreatedAt: timelineTimestamp(at), Unionid: "private-union",
		CustomerID: pgtype.Int8{Int64: 9, Valid: true},
	}
	value, err := timelineHistoryValue(row)
	if err != nil || value.Title != "private-title" || value.UnionID != "private-union" || string(value.MetadataJSON) != row.MetadataJson {
		t.Fatalf("value=%+v err=%v", value, err)
	}
	encoded, err := json.Marshal(timelineSafeRead(value))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-title", "private-summary", "private-union", "private\":\"value", "source_key_digest"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("safe read leaked %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"event_id":"evt"`) || !strings.Contains(string(encoded), `"customer_id":9`) {
		t.Fatalf("safe read lost allowed fields: %s", encoded)
	}
}

func TestCustomerTimelineHistoryRejectsMalformedStoredEvidence(t *testing.T) {
	row := contactdb.ContactV1CustomerTimelineHistory{ID: 1, SourceKeyDigest: []byte{1}, SourcePayloadDigest: timelineDigestBytes("payload"), SourceFieldDigest: timelineDigestBytes("field"), MetadataJson: `{}`}
	if _, err := timelineHistoryValue(row); !errors.Is(err, contact.ErrCustomerTimelineHistoryUnavailable) {
		t.Fatal(err)
	}
}

func timelineDigestBytes(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}
