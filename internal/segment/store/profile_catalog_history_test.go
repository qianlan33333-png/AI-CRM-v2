package store

import (
	"context"
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

var profileCatalogHistoryPostgresDSN = flag.String("profile-catalog-history-store-postgres-dsn", "", "isolated PostgreSQL DSN for schema 120 rollback verification")

func TestProfileCatalogHistoryStorePreservesSignedSourceValues(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	template, err := profileCatalogTemplate(segmentdb.SegmentV1ProfileTemplate{ID: 7, SourceID: -1, SourceKeyDigest: bytes32(1), SourcePayloadDigest: bytes32(2), TemplateCode: "code", TemplateName: "name", QuestionnaireSourceID: pgtype.Int8{Int64: 0, Valid: true}, ProgramSourceID: pgtype.Int8{Int64: -4, Valid: true}, Description: "desc", OriginalEnabled: false, Version: 0, CreatedByDigest: bytes32(3), UpdatedByDigest: bytes32(4), CreatedAt: profileCatalogTime(stamp), UpdatedAt: profileCatalogTime(stamp)})
	if err != nil || template.SourceID != -1 || template.QuestionnaireSourceID == nil || *template.QuestionnaireSourceID != 0 || template.ProgramSourceID == nil || *template.ProgramSourceID != -4 || template.OriginalEnabled || template.Version != 0 {
		t.Fatal("template signed source values were changed")
	}
	category, err := profileCatalogCategory(segmentdb.SegmentV1ProfileCategory{ID: 8, SourceID: 0, SourceKeyDigest: bytes32(1), SourcePayloadDigest: bytes32(2), TemplateSourceID: -1, TemplateHistoryID: 7, CategoryKey: "key", CategoryName: "name", Description: "desc", SortOrder: -2, OriginalEnabled: false, CreatedAt: profileCatalogTime(stamp), UpdatedAt: profileCatalogTime(stamp)})
	if err != nil || category.SourceID != 0 || category.TemplateSourceID != -1 || category.SortOrder != -2 {
		t.Fatal("category signed source values were changed")
	}
	mapping, err := profileCatalogMapping(segmentdb.SegmentV1ProfileOptionMapping{ID: 9, SourceID: -2, SourceKeyDigest: bytes32(1), SourcePayloadDigest: bytes32(2), TemplateSourceID: -1, CategorySourceID: 0, TemplateHistoryID: 7, CategoryHistoryID: 8, QuestionSourceID: -3, OptionSourceID: 0, CreatedAt: profileCatalogTime(stamp)})
	if err != nil || mapping.SourceID != -2 || mapping.QuestionSourceID != -3 || mapping.OptionSourceID != 0 {
		t.Fatal("mapping signed source values were changed")
	}
}

func TestProfileCatalogHistoryStoreFailsClosed(t *testing.T) {
	store := &ProfileCatalogHistoryStore{tx: func(context.Context) (pgx.Tx, error) {
		t.Fatal("invalid input reached caller transaction")
		return nil, nil
	}}
	if _, err := store.CreateHistoricalProfileTemplate(context.Background(), segmentport.HistoricalProfileTemplate{}); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
		t.Fatal("invalid template accepted")
	}
	if _, _, err := (&ProfileCatalogHistoryReader{}).ListHistoricalProfileOptionMappings(context.Background(), segmentport.ProfileCatalogHistoryQuery{Limit: 20}); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
		t.Fatal("missing mapping parents accepted")
	}
	if _, _, err := (&ProfileCatalogHistoryReader{}).ListHistoricalProfileCategories(context.Background(), segmentport.ProfileCatalogHistoryQuery{TemplateHistoryID: profileCatalogTestInt(1), CategoryHistoryID: profileCatalogTestInt(2), Limit: 20}); !errors.Is(err, segmentport.ErrProfileCatalogHistoryInvalid) {
		t.Fatal("category list ignored extra parent filter")
	}
}

func TestProfileCatalogHistoryPostgresRoundTripRollback(t *testing.T) {
	if *profileCatalogHistoryPostgresDSN == "" {
		t.Skip("set -profile-catalog-history-store-postgres-dsn for schema 120 rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *profileCatalogHistoryPostgresDSN)
	if err != nil {
		t.Fatal("connect isolated profile-catalog database")
	}
	defer pool.Close()
	reader := NewProfileCatalogHistoryReader(pool)
	_, before, err := reader.ListHistoricalProfileTemplates(ctx, segmentport.ProfileCatalogHistoryQuery{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatal("read initial SQLc count")
	}
	rollback := errors.New("force profile-catalog rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		created, err := NewProfileCatalogHistoryStore().CreateHistoricalProfileTemplate(txCtx, profileCatalogStoreFixture())
		if err != nil || created.ID < 1 {
			return errors.New("template create failed")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("caller transaction did not roll back")
	}
	_, after, err := reader.ListHistoricalProfileTemplates(ctx, segmentport.ProfileCatalogHistoryQuery{Limit: 100, Offset: 0})
	if err != nil || after != before {
		t.Fatal("rollback changed SQLc count")
	}
}

func profileCatalogStoreFixture() segmentport.HistoricalProfileTemplate {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.UTC)
	return segmentport.HistoricalProfileTemplate{SourceID: -12001, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}, TemplateCode: "rollback", TemplateName: "rollback", Description: "rollback", OriginalEnabled: false, Version: 0, CreatedByDigest: [32]byte{3}, UpdatedByDigest: [32]byte{4}, CreatedAt: stamp, UpdatedAt: stamp}
}
func bytes32(value byte) []byte                { return append(make([]byte, 31), value) }
func profileCatalogTestInt(value int64) *int64 { return &value }
