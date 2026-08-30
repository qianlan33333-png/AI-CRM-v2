package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/compiler"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestAudienceDefinitionEnginePreviewsAndMaterializesExactSnapshot(t *testing.T) {
	reference := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	definition := segmentport.Definition(`{"field":"is_deleted","op":"eq","value":false}`)
	store := &audienceDefinitionStore{
		definition: definition,
		queries:    audienceDefinitionQueries{universe: []int64{3, 1, 2}},
	}
	engine := NewAudienceDefinitionEngine(store)

	preview, err := engine.Preview(context.Background(), definition, reference)
	if err != nil || preview.MemberCount != 3 || preview.MemberDigest == ([32]byte{}) || !preview.EvaluatedAt.Equal(reference) {
		t.Fatalf("Preview() result=%+v err=%v", preview, err)
	}
	materialized, err := engine.Materialize(context.Background(), 42, definition, reference)
	if err != nil || !reflect.DeepEqual(preview, materialized) || !reflect.DeepEqual(store.replaced, []int64{1, 2, 3}) {
		t.Fatalf("Materialize() result=%+v replaced=%v err=%v", materialized, store.replaced, err)
	}
}

func TestAudienceDefinitionEngineRejectsSnapshotDriftBeforeReplacingMembers(t *testing.T) {
	reference := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	store := &audienceDefinitionStore{
		definition: segmentport.Definition(`{"field":"stage_id","op":"eq","value":7}`),
		queries:    audienceDefinitionQueries{universe: []int64{1}},
	}
	engine := NewAudienceDefinitionEngine(store)
	_, err := engine.Materialize(context.Background(), 42, segmentport.Definition(`{"field":"stage_id","op":"eq","value":8}`), reference)
	if !errors.Is(err, ErrSegmentDefinitionConflict) || store.replaced != nil {
		t.Fatalf("Materialize() error=%v replaced=%v", err, store.replaced)
	}
}

type audienceDefinitionStore struct {
	definition segmentport.Definition
	queries    compiler.QuerySet
	replaced   []int64
}

func (store *audienceDefinitionStore) LockDefinition(context.Context, segmentport.SegmentID) ([]byte, error) {
	return append([]byte(nil), store.definition...), nil
}
func (store *audienceDefinitionStore) QuerySet(context.Context) (compiler.QuerySet, error) {
	return store.queries, nil
}
func (store *audienceDefinitionStore) ReplaceMembers(_ context.Context, _ segmentport.SegmentID, ids []int64, _ time.Time) error {
	store.replaced = append([]int64(nil), ids...)
	return nil
}

type audienceDefinitionQueries struct{ universe []int64 }

func (queries audienceDefinitionQueries) Universe(context.Context) ([]int64, error) {
	return append([]int64(nil), queries.universe...), nil
}
func (audienceDefinitionQueries) StageEqual(context.Context, int64) ([]int64, error) { return nil, nil }
func (audienceDefinitionQueries) StageAny(context.Context, []int64) ([]int64, error) { return nil, nil }
func (audienceDefinitionQueries) OwnerEqual(context.Context, int64) ([]int64, error) { return nil, nil }
func (audienceDefinitionQueries) OwnerAny(context.Context, []int64) ([]int64, error) { return nil, nil }
func (audienceDefinitionQueries) ChannelEqual(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (audienceDefinitionQueries) ChannelAny(context.Context, []int64) ([]int64, error) {
	return nil, nil
}
func (audienceDefinitionQueries) TagAny(context.Context, []int64) ([]int64, error) { return nil, nil }
func (audienceDefinitionQueries) AddedBefore(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (audienceDefinitionQueries) AddedAfter(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (audienceDefinitionQueries) LastInteractBefore(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (audienceDefinitionQueries) LastInteractAfter(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (queries audienceDefinitionQueries) DeletedEqual(context.Context, bool) ([]int64, error) {
	return append([]int64(nil), queries.universe...), nil
}
func (queries audienceDefinitionQueries) LegacyAudiencePackageSnapshot(context.Context, int64) ([]int64, error) {
	return append([]int64(nil), queries.universe...), nil
}
