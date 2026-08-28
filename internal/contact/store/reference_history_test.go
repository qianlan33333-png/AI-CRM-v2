package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var referenceHistoryPostgresDSN = flag.String("contact-reference-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 135 rollback verification")

func TestReferenceHistoryStrictMappingAndReaderBoundary(t *testing.T) {
	ctx, at := context.Background(), time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	if _, err := NewReferenceHistoryStore().GetHistoricalExternalContactBinding(ctx, 1); !errors.Is(err, contact.ErrReferenceHistoryUnavailable) {
		t.Fatal(err)
	}
	if _, err := NewReferenceHistoryStore().CreateHistoricalWeComDirectoryMember(ctx, referenceStoreDirectory(at)); !errors.Is(err, contact.ErrReferenceHistoryUnavailable) {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*ReferenceHistoryReader{nil, NewReferenceHistoryReader(nil), NewReferenceHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalExternalContactBinding(ctx, contact.ReferenceHistoryQuery{Limit: 1}); !errors.Is(err, contact.ErrReferenceHistoryUnavailable) {
			t.Fatal(err)
		}
	}
	for _, page := range []contact.ReferenceHistoryQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewReferenceHistoryReader(nil).ListHistoricalWeComDirectoryMember(ctx, page); !errors.Is(err, contact.ErrReferenceHistoryInvalid) {
			t.Fatal(err)
		}
	}

	personID, identityID := int64(2), int64(3)
	binding, err := externalContactBindingValue(contactdb.ContactV1ExternalBindingHistory{
		ID: 1, SourceKeyDigest: referenceBytes(1), SourcePayloadDigest: referenceBytes(2), SourceFieldDigest: referenceBytes(3), ExternalUserIDDigest: referenceBytes(4), SourcePersonID: -8,
		PersonHistoryID: pgtype.Int8{Int64: personID, Valid: true}, IdentityID: pgtype.Int8{Int64: identityID, Valid: true}, IdentityAssurance: "declared", FirstBoundByUserIDDigest: referenceBytes(5), FirstOwnerUserIDDigest: referenceBytes(6), LastOwnerUserIDDigest: referenceBytes(7), CreatedAt: referenceStoredTime(at), UpdatedAt: referenceStoredTime(at.Add(-time.Second)),
	})
	if err != nil || binding.SourcePersonID != -8 || binding.PersonHistoryID == nil || *binding.PersonHistoryID != personID || binding.IdentityID == nil || *binding.IdentityID != identityID {
		t.Fatalf("binding=%+v err=%v", binding, err)
	}
	status := int32(-7)
	directory, err := wecomDirectoryMemberValue(contactdb.ContactV1DirectoryMemberHistory{
		ID: 2, SourceKeyDigest: referenceBytes(10), SourcePayloadDigest: referenceBytes(11), SourceFieldDigest: referenceBytes(12), SourceID: -9, WecomCorpIDDigest: referenceBytes(13), CorpIDDigest: referenceBytes(14), WecomUserIDDigest: referenceBytes(15), CorpAttribution: "unattributable", DisplayName: "", DepartmentIdsDigest: referenceBytes(16), DepartmentName: "", Position: "", WecomStatus: pgtype.Int4{Int32: status, Valid: true}, IsActive: false, SyncedAt: referenceStoredTime(at), RawPayloadDigest: referenceBytes(17), MobileDigest: referenceBytes(18), AvatarUrlDigest: referenceBytes(19), UpdatedByDigest: referenceBytes(20), FirstSeenAt: referenceStoredTime(at), LastSyncedAt: referenceStoredTime(at.Add(-time.Second)), CreatedAt: referenceStoredTime(at), UpdatedAt: referenceStoredTime(at.Add(-2 * time.Second)),
	})
	if err != nil || directory.SourceID != -9 || directory.MatchedStaffID != nil || directory.WeComStatus == nil || *directory.WeComStatus != status {
		t.Fatalf("directory=%+v err=%v", directory, err)
	}
}

func TestReferenceHistoryRejectsMalformedStoredPrivateEvidence(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	if _, err := externalContactBindingValue(contactdb.ContactV1ExternalBindingHistory{ID: 1, SourceKeyDigest: referenceBytes(1), SourcePayloadDigest: referenceBytes(2), SourceFieldDigest: referenceBytes(3), ExternalUserIDDigest: referenceBytes(4), IdentityAssurance: "unresolved", FirstBoundByUserIDDigest: referenceBytes(5), FirstOwnerUserIDDigest: referenceBytes(6), LastOwnerUserIDDigest: []byte{7}, CreatedAt: referenceStoredTime(at), UpdatedAt: referenceStoredTime(at)}); !errors.Is(err, contact.ErrReferenceHistoryUnavailable) {
		t.Fatal(err)
	}
	if _, err := wecomDirectoryMemberValue(contactdb.ContactV1DirectoryMemberHistory{ID: 1}); !errors.Is(err, contact.ErrReferenceHistoryUnavailable) {
		t.Fatal(err)
	}
}

func TestReferenceHistoryPostgresRoundTripRollback(t *testing.T) {
	if *referenceHistoryPostgresDSN == "" {
		t.Skip("set -contact-reference-history-postgres-dsn for isolated schema 135 rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *referenceHistoryPostgresDSN)
	if err != nil {
		t.Fatal("open_isolated_postgres")
	}
	defer pool.Close()
	q := contactdb.New(pool)
	beforeBindings, err := q.CountHistoricalExternalContactBinding(ctx)
	if err != nil {
		t.Fatal("count_bindings_before")
	}
	beforeDirectory, err := q.CountHistoricalWeComDirectoryMember(ctx)
	if err != nil {
		t.Fatal("count_directory_before")
	}
	rollback := errors.New("reference history rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store, reader := NewReferenceHistoryStore(), NewReferenceHistoryReader(pool)
		at := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
		binding, createErr := store.CreateHistoricalExternalContactBinding(txCtx, referenceStoreBinding(at))
		if createErr != nil {
			return createErr
		}
		directory, createErr := store.CreateHistoricalWeComDirectoryMember(txCtx, referenceStoreDirectory(at))
		if createErr != nil {
			return createErr
		}
		if got, getErr := reader.GetHistoricalExternalContactBinding(txCtx, binding.ID); getErr != nil || !reflect.DeepEqual(got, binding) {
			return errors.New("binding_caller_tx_not_visible")
		}
		if got, getErr := reader.GetHistoricalWeComDirectoryMember(txCtx, directory.ID); getErr != nil || !reflect.DeepEqual(got, directory) {
			return errors.New("directory_caller_tx_not_visible")
		}
		bindings, total, listErr := reader.ListHistoricalExternalContactBinding(txCtx, contact.ReferenceHistoryQuery{Limit: 1, Offset: int32(beforeBindings)})
		if listErr != nil || total != beforeBindings+1 || len(bindings) != 1 || bindings[0].ID != binding.ID {
			return errors.New("binding_page_not_transaction_bound")
		}
		directoryRows, total, listErr := reader.ListHistoricalWeComDirectoryMember(txCtx, contact.ReferenceHistoryQuery{Limit: 1, Offset: int32(beforeDirectory)})
		if listErr != nil || total != beforeDirectory+1 || len(directoryRows) != 1 || directoryRows[0].ID != directory.ID {
			return errors.New("directory_page_not_transaction_bound")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("rollback_verification_failed")
	}
	afterBindings, bindingErr := q.CountHistoricalExternalContactBinding(ctx)
	afterDirectory, directoryErr := q.CountHistoricalWeComDirectoryMember(ctx)
	if bindingErr != nil || directoryErr != nil || afterBindings != beforeBindings || afterDirectory != beforeDirectory {
		t.Fatal("rollback_retained_history")
	}
}

func referenceBytes(value byte) []byte   { result := referenceArray(value); return result[:] }
func referenceArray(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
func referenceStoredTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func referenceStoreBinding(at time.Time) contact.HistoricalExternalContactBinding {
	return contact.HistoricalExternalContactBinding{SourceKeyDigest: referenceArray(1), SourcePayloadDigest: referenceArray(2), SourceFieldDigest: referenceArray(3), ExternalUserIDDigest: referenceArray(4), SourcePersonID: -8, IdentityAssurance: "unresolved", FirstBoundByUserIDDigest: referenceArray(5), FirstOwnerUserIDDigest: referenceArray(6), LastOwnerUserIDDigest: referenceArray(7), CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func referenceStoreDirectory(at time.Time) contact.HistoricalWeComDirectoryMember {
	return contact.HistoricalWeComDirectoryMember{SourceKeyDigest: referenceArray(10), SourcePayloadDigest: referenceArray(11), SourceFieldDigest: referenceArray(12), SourceID: -9, WeComCorpIDDigest: referenceArray(13), CorpIDDigest: referenceArray(14), WeComUserIDDigest: referenceArray(15), CorpAttribution: "unattributable", DisplayName: "", DepartmentIDsDigest: referenceArray(16), DepartmentName: "", Position: "", IsActive: false, SyncedAt: at, RawPayloadDigest: referenceArray(17), MobileDigest: referenceArray(18), AvatarURLDigest: referenceArray(19), UpdatedByDigest: referenceArray(20), FirstSeenAt: at, LastSyncedAt: at.Add(-time.Second), CreatedAt: at, UpdatedAt: at.Add(-2 * time.Second)}
}
