package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	memberusage "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxcmemberusagehistory"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var hxcMemberUsageTestKey = []byte("01234567890123456789012345678901")

func TestHXCMemberUsageHistoryFactPreservesPrivateAndNullableFields(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789123, time.FixedZone("source", 8*3600))
	source := hxcMemberUsageSourceFixture(t, 7, 1, at)
	fact, err := hxcMemberUsageHistoryFact(source)
	if err != nil {
		t.Fatal(err)
	}
	fact.ID = 1
	if _, err = hxcapp.HistoricalHXCMemberUsageDigest(fact); err != nil || fact.Generation != -7 || fact.RegisteredAt == nil || fact.FirstUsedAt != nil || fact.ProjectedAt.Location() != time.UTC || fact.ProjectedAt.Nanosecond() != 456789000 || !json.Valid(fact.PayloadJSON) {
		t.Fatalf("fact=%+v err=%v", fact, err)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || string(encoded) == "" || hxcMemberUsageContainsAny(string(encoded), source.ResolverUnionID(), source.LegacyOwnerUserID(), source.MobileHash(), string(source.PayloadJSON)) {
		t.Fatalf("private source leaked: %s err=%v", encoded, err)
	}
	*source.RegisteredAt = at.Add(99 * time.Hour)
	source.PayloadJSON[0] = '['
	if fact.RegisteredAt.Equal(*source.RegisteredAt) || string(fact.PayloadJSON) != `{"private":"payload-private-1"}` {
		t.Fatal("fact retained source aliases")
	}
}

func TestHXCMemberUsageHistoryEntriesPreserveOrdinalAndVerifyTarget(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789000, time.UTC)
	first := hxcMemberUsageSourceFixture(t, 10, 1, at)
	second := hxcMemberUsageSourceFixture(t, 11, 2, at)
	store := &hxcMemberUsageTargetFake{values: map[int64]hxcport.HistoricalHXCMemberUsage{}}
	entries, err := hxcMemberUsageEntries(context.Background(), []memberusage.MemberUsageObservationFact{first, second}, "v1-full-archive-20260827", hxcMemberUsageTxStub{}, store, store)
	if err != nil || len(entries) != 2 || entries[0].scope.ImportVersion != HXCMemberUsageHistoryVersion || entries[0].scope.TableID != memberusage.MemberUsageProjectionTableID || entries[0].scope.TargetDomain != "hxc" || entries[0].scope.TargetTable != hxcMemberUsageHistoryTarget || entries[0].kind != hxcport.HXCHistoryMemberUsage || entries[0].ordinal != 10 || entries[1].ordinal != 11 || entries[0].source != SourceIdentifier(entries[0].key) {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	stored, err := hxcMemberUsageHistoryFact(second)
	if err != nil {
		t.Fatal(err)
	}
	stored.ID = 7
	store.values[7] = stored
	digest, err := entries[1].verify(context.Background(), 7)
	if err != nil || digest == ([sha256.Size]byte{}) {
		t.Fatalf("verify=%x err=%v", digest, err)
	}
	stored.MobileHash += "-drift"
	store.values[7] = stored
	if _, err = entries[1].verify(context.Background(), 7); !errors.Is(err, ErrConflict) {
		t.Fatalf("private target drift=%v", err)
	}
	if _, err = hxcMemberUsageEntries(context.Background(), []memberusage.MemberUsageObservationFact{second, first}, "run", hxcMemberUsageTxStub{}, store, store); !errors.Is(err, ErrConflict) {
		t.Fatalf("out-of-order ordinal=%v", err)
	}
	if _, err = hxcMemberUsageEntries(context.Background(), []memberusage.MemberUsageObservationFact{first, first}, "run", hxcMemberUsageTxStub{}, store, store); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate source=%v", err)
	}
	tooLarge := make([]memberusage.MemberUsageObservationFact, memberusage.StreamBatchSize+1)
	if _, err = hxcMemberUsageEntries(context.Background(), tooLarge, "run", hxcMemberUsageTxStub{}, store, store); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("oversize batch=%v", err)
	}
}

func TestHXCMemberUsageEntriesRejectInvalidBatchContext(t *testing.T) {
	source := hxcMemberUsageSourceFixture(t, 1, 1, time.Now())
	store := &hxcMemberUsageTargetFake{values: map[int64]hxcport.HistoricalHXCMemberUsage{}}
	for name, input := range map[string]struct {
		ctx    context.Context
		run    string
		tx     pgx.Tx
		store  hxcport.HXCMemberUsageHistoryStore
		reader hxcport.HXCMemberUsageHistoryReader
	}{
		"nil context": {ctx: nil, run: "run", tx: hxcMemberUsageTxStub{}, store: store, reader: store},
		"blank run":   {ctx: context.Background(), run: "", tx: hxcMemberUsageTxStub{}, store: store, reader: store},
		"nil tx":      {ctx: context.Background(), run: "run", store: store, reader: store},
		"nil store":   {ctx: context.Background(), run: "run", tx: hxcMemberUsageTxStub{}, reader: store},
		"nil reader":  {ctx: context.Background(), run: "run", tx: hxcMemberUsageTxStub{}, store: store},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := hxcMemberUsageEntries(input.ctx, []memberusage.MemberUsageObservationFact{source}, input.run, input.tx, input.store, input.reader); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestHXCMemberUsageHistoryJournalRejectsMalformedReceipt(t *testing.T) {
	bridge := hxcMemberUsageHistoryJournal{journal: &Journal{scope: Scope{ImportVersion: HXCMemberUsageHistoryVersion, ArchiveRunID: "run", AdapterID: v1archive.DefaultAdapterID, TableID: memberusage.MemberUsageProjectionTableID, TargetDomain: "hxc", TargetTable: hxcMemberUsageHistoryTarget}}}
	source := SourceIdentifier(hxcMemberUsageDigest("source"))
	valid := hxcport.HXCHistoryReceipt{Kind: hxcport.HXCHistoryMemberUsage, SourceIdentifier: source, PayloadDigest: hxcMemberUsageDigest("payload"), TargetDigest: hxcMemberUsageDigest("target"), TargetID: 1}
	for name, mutate := range map[string]func(*hxcport.HXCHistoryReceipt){
		"replayed": func(value *hxcport.HXCHistoryReceipt) { value.Replayed = true },
		"kind":     func(value *hxcport.HXCHistoryReceipt) { value.Kind = "other" },
		"source":   func(value *hxcport.HXCHistoryReceipt) { value.SourceIdentifier = "not-hex" },
		"target":   func(value *hxcport.HXCHistoryReceipt) { value.TargetID = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			mutate(&value)
			if err := bridge.RecordHXCHistory(context.Background(), value); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("malformed receipt=%v", err)
			}
		})
	}
}

type hxcMemberUsageTargetFake struct {
	values map[int64]hxcport.HistoricalHXCMemberUsage
}

func (fake *hxcMemberUsageTargetFake) CreateHistoricalHXCMemberUsage(context.Context, hxcport.HistoricalHXCMemberUsage) (hxcport.HistoricalHXCMemberUsage, error) {
	return hxcport.HistoricalHXCMemberUsage{}, errors.New("unused")
}
func (fake *hxcMemberUsageTargetFake) GetHistoricalHXCMemberUsage(_ context.Context, id int64) (hxcport.HistoricalHXCMemberUsage, error) {
	value, found := fake.values[id]
	if !found {
		return hxcport.HistoricalHXCMemberUsage{}, errors.New("not found")
	}
	return value, nil
}
func (fake *hxcMemberUsageTargetFake) ListHistoricalHXCMemberUsage(context.Context, hxcport.HXCMemberUsageHistoryQuery) ([]hxcport.HistoricalHXCMemberUsage, int64, error) {
	return nil, 0, errors.New("unused")
}

type hxcMemberUsageTxStub struct{}

func (hxcMemberUsageTxStub) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("unused") }
func (hxcMemberUsageTxStub) Commit(context.Context) error          { return errors.New("unused") }
func (hxcMemberUsageTxStub) Rollback(context.Context) error        { return errors.New("unused") }
func (hxcMemberUsageTxStub) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unused")
}
func (hxcMemberUsageTxStub) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (hxcMemberUsageTxStub) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (hxcMemberUsageTxStub) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("unused")
}
func (hxcMemberUsageTxStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unused")
}
func (hxcMemberUsageTxStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unused")
}
func (hxcMemberUsageTxStub) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (hxcMemberUsageTxStub) Conn() *pgx.Conn                                  { return nil }

func hxcMemberUsageSourceFixture(t *testing.T, ordinal int64, seed byte, at time.Time) memberusage.MemberUsageObservationFact {
	t.Helper()
	unionID := fmt.Sprintf("union-private-%d", seed)
	ownerUserID := fmt.Sprintf("owner-private-%d", seed)
	mobileHash := fmt.Sprintf("mobile-private-%d", seed)
	payloadJSON := json.RawMessage(fmt.Sprintf(`{"private":"payload-private-%d"}`, seed))
	payload, err := json.Marshal(map[string]any{
		"generation": int64(-7), "unionid": unionID, "owner_userid": ownerUserID, "mobile_hash": mobileHash,
		"is_member": false, "is_registered": true, "registered_at": at.Format(time.RFC3339Nano), "has_real_usage": true,
		"first_used_at": nil, "last_used_at": at.Add(-time.Hour).Format(time.RFC3339Nano), "member_since": nil, "membership_expires_at": at.Add(time.Hour).Format(time.RFC3339Nano),
		"membership_tier": "tier", "membership_status": "status", "membership_source": "membership", "registration_source": "registration", "usage_source": "usage",
		"updated_at": nil, "payload_json": payloadJSON, "projected_at": at.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	keyJSON := []byte(fmt.Sprintf(`[-7, "%s", "%s"]`, unionID, ownerUserID))
	key, err := v1archive.SourceKeyHMAC(hxcMemberUsageTestKey, "ai_audience_hxc_member_usage_projection", keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest, err := v1archive.PayloadHMAC(hxcMemberUsageTestKey, "ai_audience_hxc_member_usage_projection", payload)
	if err != nil {
		t.Fatal(err)
	}
	field, err := v1archive.FieldHMAC(hxcMemberUsageTestKey, "ai_audience_hxc_member_usage_projection", nil)
	if err != nil {
		t.Fatal(err)
	}
	result := memberusage.AdaptMemberUsageObservation(v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: memberusage.MemberUsageProjectionTableID, SourceOrdinal: ordinal, SourceKeyHMAC: key, PayloadHMAC: payloadDigest, FieldHMAC: field, Payload: payload}, hxcMemberUsageTestKey, ordinal)
	if result.Disposition != memberusage.DispositionCandidate || result.Fact == nil {
		t.Fatalf("adapter result=%#v", result)
	}
	return *result.Fact
}

func hxcMemberUsageDigest(value string) [sha256.Size]byte { return sha256.Sum256([]byte(value)) }

func hxcMemberUsageContainsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
