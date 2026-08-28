package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var deferredIdentityHistoryPostgresDSN = flag.String("deferred-identity-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 132 rollback verification")

func TestDeferredIdentityHistoryStrictMappingAndReaderBoundary(t *testing.T) {
	ctx, at := context.Background(), time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	if _, err := NewDeferredIdentityHistoryStore().GetHistoricalDeferredPerson(ctx, 1); !errors.Is(err, contact.ErrDeferredIdentityHistoryUnavailable) {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*DeferredIdentityHistoryReader{nil, NewDeferredIdentityHistoryReader(nil), NewDeferredIdentityHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalDeferredPerson(ctx, contact.DeferredIdentityHistoryQuery{Limit: 1}); !errors.Is(err, contact.ErrDeferredIdentityHistoryUnavailable) {
			t.Fatal(err)
		}
	}
	for _, page := range []contact.DeferredIdentityHistoryQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewDeferredIdentityHistoryReader(nil).ListHistoricalDeferredIdentityConflict(ctx, page); !errors.Is(err, contact.ErrDeferredIdentityHistoryInvalid) {
			t.Fatal(err)
		}
	}

	person, err := deferredPersonValue(contactdb.ContactV1DeferredPersonHistory{ID: 1, SourceKeyDigest: deferredBytes(1), SourcePayloadDigest: deferredBytes(2), SourceFieldDigest: deferredBytes(3), SourceID: -8, MobileDigest: deferredBytes(4), ThirdPartyUserIDDigest: deferredBytes(5), PrivateDigest: deferredBytes(6), RedactedRoots: []string{}, CreatedAt: deferredStoredTime(at), UpdatedAt: deferredStoredTime(at.Add(-time.Second))})
	if err != nil || person.SourceID != -8 || person.RedactedRoots == nil || len(person.RedactedRoots) != 0 {
		t.Fatalf("person=%+v err=%v", person, err)
	}
	conflict, err := deferredConflictValue(contactdb.ContactV1DeferredIdentityConflictHistory{ID: 2, SourceKeyDigest: deferredBytes(10), SourcePayloadDigest: deferredBytes(11), SourceFieldDigest: deferredBytes(12), SourceID: 0, UnionIDDigest: deferredBytes(13), CandidateUnionIDDigest: deferredBytes(14), ExternalUserIDDigest: deferredBytes(15), OpenIDDigest: deferredBytes(16), MobileDigest: deferredBytes(17), LegacySourceKeyDigest: deferredBytes(18), PayloadJsonDigest: deferredBytes(19), SourcePayloadJsonDigest: deferredBytes(20), ResolutionNoteDigest: deferredBytes(21), PrivateDigest: deferredBytes(22), RedactedRoots: []string{"payload"}, CreatedAt: deferredStoredTime(at), UpdatedAt: deferredStoredTime(at.Add(-time.Second)), ResolvedAt: pgtype.Timestamptz{}})
	if err != nil || conflict.SourceID != 0 || conflict.ResolvedAt != nil || conflict.RedactedRoots[0] != "payload" {
		t.Fatalf("conflict=%+v err=%v", conflict, err)
	}
	typeValue := int32(-7)
	gender := deferredBytes(42)
	missing, err := missingRootIdentityValue(contactdb.ContactV1MissingRootIdentityHistory{ID: 3, SourceKeyDigest: deferredBytes(30), SourcePayloadDigest: deferredBytes(31), SourceFieldDigest: deferredBytes(32), SourceID: -9, Dm01RunID: 2, Dm01SourceKeyDigest: deferredBytes(33), Dm01SourceHmacKeyVersion: "v1-domain-a1", QuarantineReason: "missing_customer_root", Type: pgtype.Int4{Int32: typeValue, Valid: true}, CorpIDDigest: deferredBytes(34), ExternalUserIDDigest: deferredBytes(35), UnionIDDigest: deferredBytes(36), OpenIDDigest: deferredBytes(37), FollowUserIDDigest: deferredBytes(38), NameDigest: deferredBytes(39), AvatarDigest: deferredBytes(40), GenderDigest: gender, RawProfileDigest: deferredBytes(41), PrivateDigest: deferredBytes(43), RedactedRoots: []string{"raw_profile"}, FirstSeenAt: deferredStoredTime(at), LastSeenAt: deferredStoredTime(at.Add(-time.Second)), CreatedAt: deferredStoredTime(at), UpdatedAt: deferredStoredTime(at.Add(-2 * time.Second))})
	if err != nil || missing.SourceID != -9 || missing.Type == nil || *missing.Type != typeValue || missing.GenderDigest == nil || *missing.GenderDigest != deferredArray(42) {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
}

func TestDeferredIdentityHistoryRejectsMalformedStoredPrivateEvidence(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	if _, err := deferredPersonValue(contactdb.ContactV1DeferredPersonHistory{ID: 1, SourceKeyDigest: deferredBytes(1), SourcePayloadDigest: deferredBytes(2), SourceFieldDigest: deferredBytes(3), MobileDigest: deferredBytes(4), ThirdPartyUserIDDigest: deferredBytes(5), PrivateDigest: []byte{6}, RedactedRoots: []string{}, CreatedAt: deferredStoredTime(at), UpdatedAt: deferredStoredTime(at)}); !errors.Is(err, contact.ErrDeferredIdentityHistoryUnavailable) {
		t.Fatal(err)
	}
	if _, err := deferredConflictValue(contactdb.ContactV1DeferredIdentityConflictHistory{ID: 1}); !errors.Is(err, contact.ErrDeferredIdentityHistoryUnavailable) {
		t.Fatal(err)
	}
}

func TestDeferredIdentityHistoryPostgresRoundTripRollback(t *testing.T) {
	if *deferredIdentityHistoryPostgresDSN == "" {
		t.Skip("set -deferred-identity-history-postgres-dsn for isolated schema 132 rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *deferredIdentityHistoryPostgresDSN)
	if err != nil {
		t.Fatal("open_isolated_postgres")
	}
	defer pool.Close()
	q := contactdb.New(pool)
	beforePerson, err := q.CountHistoricalDeferredPerson(ctx)
	if err != nil {
		t.Fatal("count_person_before")
	}
	beforeConflict, err := q.CountHistoricalDeferredIdentityConflict(ctx)
	if err != nil {
		t.Fatal("count_conflict_before")
	}
	beforeMissing, err := q.CountHistoricalMissingRootIdentity(ctx)
	if err != nil {
		t.Fatal("count_missing_before")
	}
	rollback := errors.New("deferred identity history rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		fixture, fixtureErr := contactdb.New(tx).CreateArchiveIdentityRootFixture(txCtx, contactdb.CreateArchiveIdentityRootFixtureParams{Manifest: deferredBytes(90), RepositorySha: strings.Repeat("a", 40), Watermark: deferredStoredTime(time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)), Deleted: false, SourceKey: deferredBytes(91), Payload: deferredBytes(92), ReceiptPayload: deferredBytes(92), FieldDigest: deferredBytes(93)})
		if fixtureErr != nil {
			return fixtureErr
		}
		store, reader := NewDeferredIdentityHistoryStore(), NewDeferredIdentityHistoryReader(pool)
		at := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
		person, createErr := store.CreateHistoricalDeferredPerson(txCtx, deferredStorePerson(at))
		if createErr != nil {
			return createErr
		}
		conflict, createErr := store.CreateHistoricalDeferredIdentityConflict(txCtx, deferredStoreConflict(at))
		if createErr != nil {
			return createErr
		}
		missing := deferredStoreMissing(at, fixture.RunID)
		missingValue, createErr := store.CreateHistoricalMissingRootIdentity(txCtx, missing)
		if createErr != nil {
			return createErr
		}
		if got, getErr := reader.GetHistoricalDeferredPerson(txCtx, person.ID); getErr != nil || !reflect.DeepEqual(got, person) {
			return errors.New("person_caller_tx_not_visible")
		}
		if got, getErr := reader.GetHistoricalDeferredIdentityConflict(txCtx, conflict.ID); getErr != nil || !reflect.DeepEqual(got, conflict) {
			return errors.New("conflict_caller_tx_not_visible")
		}
		if got, getErr := reader.GetHistoricalMissingRootIdentity(txCtx, missingValue.ID); getErr != nil || !reflect.DeepEqual(got, missingValue) {
			return errors.New("missing_caller_tx_not_visible")
		}
		if rows, total, listErr := reader.ListHistoricalDeferredPerson(txCtx, contact.DeferredIdentityHistoryQuery{Limit: 1, Offset: int32(beforePerson + 1)}); listErr != nil || total != beforePerson+1 || len(rows) != 0 {
			return errors.New("page_not_transaction_bound")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("rollback_verification_failed")
	}
	afterPerson, personErr := q.CountHistoricalDeferredPerson(ctx)
	afterConflict, conflictErr := q.CountHistoricalDeferredIdentityConflict(ctx)
	afterMissing, missingErr := q.CountHistoricalMissingRootIdentity(ctx)
	if personErr != nil || conflictErr != nil || missingErr != nil || afterPerson != beforePerson || afterConflict != beforeConflict || afterMissing != beforeMissing {
		t.Fatal("rollback_retained_history")
	}
}

func deferredBytes(value byte) []byte   { result := deferredArray(value); return result[:] }
func deferredArray(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
func deferredStoredTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func deferredStorePerson(at time.Time) contact.HistoricalDeferredPerson {
	return contact.HistoricalDeferredPerson{SourceID: -1, SourceKeyDigest: deferredArray(1), SourcePayloadDigest: deferredArray(2), SourceFieldDigest: deferredArray(3), MobileDigest: deferredArray(4), ThirdPartyUserIDDigest: deferredArray(5), PrivateDigest: deferredArray(6), RedactedRoots: []string{}, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func deferredStoreConflict(at time.Time) contact.HistoricalDeferredIdentityConflict {
	return contact.HistoricalDeferredIdentityConflict{SourceID: 0, SourceKeyDigest: deferredArray(10), SourcePayloadDigest: deferredArray(11), SourceFieldDigest: deferredArray(12), UnionIDDigest: deferredArray(13), CandidateUnionIDDigest: deferredArray(14), ExternalUserIDDigest: deferredArray(15), OpenIDDigest: deferredArray(16), MobileDigest: deferredArray(17), LegacySourceKeyDigest: deferredArray(18), PayloadJSONDigest: deferredArray(19), SourcePayloadJSONDigest: deferredArray(20), ResolutionNoteDigest: deferredArray(21), PrivateDigest: deferredArray(22), RedactedRoots: []string{"payload"}, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func deferredStoreMissing(at time.Time, runID int64) contact.HistoricalMissingRootIdentity {
	return contact.HistoricalMissingRootIdentity{SourceID: -2, SourceKeyDigest: deferredArray(30), SourcePayloadDigest: deferredArray(31), SourceFieldDigest: deferredArray(32), DM01RunID: runID, DM01SourceKeyDigest: deferredArray(33), DM01SourceHMACKeyVersion: "v1-domain-a1", QuarantineReason: "missing_customer_root", CorpIDDigest: deferredArray(34), ExternalUserIDDigest: deferredArray(35), UnionIDDigest: deferredArray(36), OpenIDDigest: deferredArray(37), FollowUserIDDigest: deferredArray(38), NameDigest: deferredArray(39), AvatarDigest: deferredArray(40), RawProfileDigest: deferredArray(41), PrivateDigest: deferredArray(42), RedactedRoots: []string{}, FirstSeenAt: at, LastSeenAt: at.Add(-time.Second), CreatedAt: at, UpdatedAt: at.Add(-2 * time.Second)}
}
