package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaigndb "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var campaignDefinitionHistoryPostgresDSN = flag.String("campaign-definition-history-store-postgres-dsn", "", "isolated PostgreSQL DSN for schema 131 rollback verification")

func TestCampaignDefinitionHistoryFailsClosedAndValidates(t *testing.T) {
	ctx := context.Background()
	store := NewCampaignDefinitionHistoryStore()
	definition, step := campaignDefinitionHistoryFixture(1)
	if _, err := store.CreateHistoricalCampaignDefinition(ctx, definition); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
		t.Fatalf("create without caller transaction = %v", err)
	}
	definition.ID = 1
	if _, err := store.CreateHistoricalCampaignDefinition(ctx, definition); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("generated definition ID accepted: %v", err)
	}
	step.ID = 1
	if _, err := store.CreateHistoricalCampaignDefinitionStep(ctx, step); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("generated step ID accepted: %v", err)
	}

	var pool *pgxpool.Pool
	for _, reader := range []*CampaignDefinitionHistoryReader{nil, NewCampaignDefinitionHistoryReader(nil), NewCampaignDefinitionHistoryReader(pool)} {
		if _, err := reader.GetHistoricalCampaignDefinition(ctx, 1); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
			t.Fatalf("nil reader escaped fail closed: %v", err)
		}
	}
	reader := NewCampaignDefinitionHistoryReader(nil)
	if _, _, err := reader.ListHistoricalCampaignDefinitions(ctx, 0, 0); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("definition limit accepted: %v", err)
	}
	if _, _, err := reader.ListHistoricalCampaignDefinitions(ctx, 101, 0); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("definition oversized limit accepted: %v", err)
	}
	if _, _, err := reader.ListHistoricalCampaignDefinitionSteps(ctx, nil, 20, -1); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("step negative offset accepted: %v", err)
	}
	signed := int64(-7)
	if _, _, err := reader.ListHistoricalCampaignDefinitionSteps(ctx, &signed, 20, 0); !errors.Is(err, campaignport.ErrCampaignHistoryUnavailable) {
		t.Fatalf("signed source ID was rejected before database access: %v", err)
	}
}

func TestCampaignDefinitionHistoryStepMappingCurrentCampaignCode(t *testing.T) {
	value, err := campaignDefinitionHistoryStepValue(campaigndb.CampaignV1DefinitionStepHistory{
		ID: 1, SourceID: -2, CampaignSourceID: -3, SegmentSourceID: -4,
		CurrentCampaignCode: pgtype.Text{String: "existing-campaign", Valid: true}, SourceParentState: "current_definition",
		StepIndex: -1, DayOffset: -2, SendTime: "", Timezone: "", ContentMasked: "", SkipRecentDays: -3,
		CreatedAt:           campaignDefinitionHistoryTimestamp(time.Date(2025, 2, 3, 4, 5, 6, 123456000, time.FixedZone("source", 8*60*60))),
		UpdatedAt:           campaignDefinitionHistoryTimestamp(time.Date(2025, 2, 3, 4, 5, 7, 123456000, time.UTC)),
		OriginalDisposition: "archive", OriginalReason: "missing parent",
		ContentDigest: campaignDefinitionHistoryDigest(1), PrivateDigest: campaignDefinitionHistoryDigest(2), SourceKeyDigest: campaignDefinitionHistoryDigest(3),
		SourcePayloadDigest: campaignDefinitionHistoryDigest(4), SourceFieldDigest: campaignDefinitionHistoryDigest(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.CurrentCampaignCode == nil || *value.CurrentCampaignCode != "existing-campaign" || value.HistoryDefinitionID != nil {
		t.Fatalf("current campaign code mapping = %+v", value)
	}
	if !value.CreatedAt.Equal(time.Date(2025, 2, 2, 20, 5, 6, 123456000, time.UTC)) {
		t.Fatalf("created timestamp was not UTC microsecond normalized: %s", value.CreatedAt)
	}
}

func TestCampaignDefinitionHistoryPostgresRoundTripRollback(t *testing.T) {
	if *campaignDefinitionHistoryPostgresDSN == "" {
		t.Skip("set -campaign-definition-history-store-postgres-dsn for isolated schema 131 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *campaignDefinitionHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := campaigndb.New(pool)
	beforeDefinitions, err := queries.CountHistoricalCampaignDefinitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeSteps, err := queries.CountHistoricalCampaignDefinitionSteps(ctx, pgtype.Int8{})
	if err != nil {
		t.Fatal(err)
	}

	rollback := errors.New("campaign definition history forced rollback")
	var definitionID, stepID int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewCampaignDefinitionHistoryStore()
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		reader := NewCampaignDefinitionHistoryReader(tx)
		definition, step := campaignDefinitionHistoryFixture(time.Now().UTC().UnixNano())
		createdDefinition, createErr := store.CreateHistoricalCampaignDefinition(txCtx, definition)
		if createErr != nil {
			return fmt.Errorf("definition create: %w", createErr)
		}
		definition.ID = createdDefinition.ID
		if !reflect.DeepEqual(createdDefinition, definition) {
			return fmt.Errorf("definition create mapping changed fields")
		}
		if loaded, getErr := store.GetHistoricalCampaignDefinition(txCtx, definition.ID); getErr != nil || !reflect.DeepEqual(loaded, definition) {
			return fmt.Errorf("definition store get: equal=%t err=%v", reflect.DeepEqual(loaded, definition), getErr)
		}
		if loaded, getErr := reader.GetHistoricalCampaignDefinition(txCtx, definition.ID); getErr != nil || !reflect.DeepEqual(loaded, definition) {
			return fmt.Errorf("definition reader get: equal=%t err=%v", reflect.DeepEqual(loaded, definition), getErr)
		}
		if beforeDefinitions > math.MaxInt32 {
			return fmt.Errorf("definition count exceeds page offset range: %d", beforeDefinitions)
		}
		definitions, total, listErr := reader.ListHistoricalCampaignDefinitions(txCtx, 20, int32(beforeDefinitions))
		if listErr != nil || total != beforeDefinitions+1 || len(definitions) != 1 || definitions[0].ID != definition.ID {
			return fmt.Errorf("definition list: total=%d items=%d err=%v", total, len(definitions), listErr)
		}

		step.HistoryDefinitionID = &definition.ID
		createdStep, createErr := store.CreateHistoricalCampaignDefinitionStep(txCtx, step)
		if createErr != nil {
			return fmt.Errorf("step create: %w", createErr)
		}
		step.ID = createdStep.ID
		if !reflect.DeepEqual(createdStep, step) {
			return fmt.Errorf("step create mapping changed fields")
		}
		if loaded, getErr := store.GetHistoricalCampaignDefinitionStep(txCtx, step.ID); getErr != nil || !reflect.DeepEqual(loaded, step) {
			return fmt.Errorf("step store get: equal=%t err=%v", reflect.DeepEqual(loaded, step), getErr)
		}
		steps, total, listErr := reader.ListHistoricalCampaignDefinitionSteps(txCtx, &step.CampaignSourceID, 20, 0)
		if listErr != nil || total != 1 || len(steps) != 1 || !reflect.DeepEqual(steps[0], step) {
			return fmt.Errorf("step list: total=%d items=%d err=%v", total, len(steps), listErr)
		}
		if _, createErr = store.CreateHistoricalCampaignDefinitionStep(txCtx, step); !errors.Is(createErr, campaignport.ErrCampaignHistoryInvalid) {
			return fmt.Errorf("created ID replay input = %v", createErr)
		}
		definitionID, stepID = definition.ID, step.ID
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	afterDefinitions, err := queries.CountHistoricalCampaignDefinitions(ctx)
	if err != nil || afterDefinitions != beforeDefinitions {
		t.Fatalf("definition rollback count before=%d after=%d err=%v", beforeDefinitions, afterDefinitions, err)
	}
	afterSteps, err := queries.CountHistoricalCampaignDefinitionSteps(ctx, pgtype.Int8{})
	if err != nil || afterSteps != beforeSteps {
		t.Fatalf("step rollback count before=%d after=%d err=%v", beforeSteps, afterSteps, err)
	}
	for _, id := range []int64{definitionID, stepID} {
		if id < 1 {
			t.Fatal("rollback did not create both records")
		}
	}
	if _, err := queries.GetHistoricalCampaignDefinition(ctx, definitionID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("rolled back definition remained: %v", err)
	}
	if _, err := queries.GetHistoricalCampaignDefinitionStep(ctx, stepID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("rolled back step remained: %v", err)
	}
}

func campaignDefinitionHistoryFixture(sourceID int64) (campaignport.HistoricalCampaignDefinition, campaignport.HistoricalCampaignDefinitionStep) {
	created := time.Date(2025, 2, 2, 20, 5, 6, 123456000, time.UTC)
	approved := created.Add(time.Hour)
	digest := func(label string) [32]byte {
		return sha256.Sum256([]byte(fmt.Sprintf("campaign-definition-history/%d/%s", sourceID, label)))
	}
	definition := campaignport.HistoricalCampaignDefinition{
		SourceID: sourceID, Code: "", DisplayName: "", Intent: "", AnchorMode: "", AnchorDate: "", ReviewStatus: "", RunStatus: "",
		ApprovedAt: &approved, PausedReason: "", CreatedAt: created, UpdatedAt: created.Add(-time.Second), OriginalDisposition: "archive", OriginalReason: "legacy definition",
		PrivateDigest: digest("definition-private"), SourceKeyDigest: digest("definition-key"), SourcePayloadDigest: digest("definition-payload"), SourceFieldDigest: digest("definition-field"),
		RedactedRoots: []string{"payload"},
	}
	step := campaignport.HistoricalCampaignDefinitionStep{
		SourceID: sourceID - 1, CampaignSourceID: -sourceID, SegmentSourceID: 1 - sourceID, SourceParentState: "history_definition",
		StepIndex: -1, DayOffset: -2, SendTime: "", Timezone: "", ContentMasked: "", StopOnReply: false, SkipRecentDays: -3,
		CreatedAt: created, UpdatedAt: created.Add(-time.Second), OriginalDisposition: "quarantine", OriginalReason: "missing source parent",
		ContentDigest: digest("step-content"), PrivateDigest: digest("step-private"), SourceKeyDigest: digest("step-key"), SourcePayloadDigest: digest("step-payload"), SourceFieldDigest: digest("step-field"),
		RedactedRoots: []string{"content"},
	}
	return definition, step
}

func campaignDefinitionHistoryDigest(seed byte) []byte {
	return append([]byte{seed}, make([]byte, 31)...)
}
