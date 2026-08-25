package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/compiler"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

var ErrSegmentDefinitionConflict = errors.New("segment definition changed after audience configuration snapshot")

type AudienceDefinitionEngine struct{ store RefreshStore }

var _ segmentport.AudienceDefinitionEngine = (*AudienceDefinitionEngine)(nil)

func NewAudienceDefinitionEngine(store RefreshStore) *AudienceDefinitionEngine {
	return &AudienceDefinitionEngine{store: store}
}

func (engine *AudienceDefinitionEngine) Preview(
	ctx context.Context,
	definition segmentport.Definition,
	reference time.Time,
) (segmentport.DefinitionEvaluation, error) {
	if engine == nil || engine.store == nil || ctx == nil || reference.IsZero() || reference.Location() != time.UTC {
		return segmentport.DefinitionEvaluation{}, ErrInvalidSegmentRefresh
	}
	canonical, err := canonicalDefinition(definition)
	if err != nil {
		return segmentport.DefinitionEvaluation{}, ErrInvalidSegmentRefresh
	}
	memberIDs, err := materializedIDs(ctx, engine.store, canonical, reference)
	if err != nil {
		return segmentport.DefinitionEvaluation{}, errors.Join(ErrSegmentRefreshFailed, err)
	}
	encoded, err := json.Marshal(memberIDs)
	if err != nil {
		return segmentport.DefinitionEvaluation{}, errors.Join(ErrSegmentRefreshFailed, err)
	}
	return segmentport.DefinitionEvaluation{
		MemberCount: int64(len(memberIDs)), MemberDigest: sha256.Sum256(encoded), EvaluatedAt: reference,
	}, nil
}

func (engine *AudienceDefinitionEngine) Materialize(
	ctx context.Context,
	segmentID segmentport.SegmentID,
	definition segmentport.Definition,
	reference time.Time,
) (segmentport.DefinitionEvaluation, error) {
	if segmentID <= 0 {
		return segmentport.DefinitionEvaluation{}, ErrInvalidSegmentRefresh
	}
	locked, err := engine.store.LockDefinition(ctx, segmentID)
	if err != nil {
		return segmentport.DefinitionEvaluation{}, err
	}
	wanted, err := canonicalDefinition(definition)
	if err != nil {
		return segmentport.DefinitionEvaluation{}, ErrInvalidSegmentRefresh
	}
	current, err := canonicalDefinition(locked)
	if err != nil {
		return segmentport.DefinitionEvaluation{}, ErrSegmentRefreshFailed
	}
	if !bytes.Equal(current, wanted) {
		return segmentport.DefinitionEvaluation{}, ErrSegmentDefinitionConflict
	}
	memberIDs, err := materializedIDs(ctx, engine.store, wanted, reference)
	if err != nil {
		return segmentport.DefinitionEvaluation{}, err
	}
	if err = engine.store.ReplaceMembers(ctx, segmentID, memberIDs, reference); err != nil {
		return segmentport.DefinitionEvaluation{}, err
	}
	encoded, err := json.Marshal(memberIDs)
	if err != nil {
		return segmentport.DefinitionEvaluation{}, errors.Join(ErrSegmentRefreshFailed, err)
	}
	return segmentport.DefinitionEvaluation{
		MemberCount: int64(len(memberIDs)), MemberDigest: sha256.Sum256(encoded), EvaluatedAt: reference,
	}, nil
}

func materializedIDs(ctx context.Context, store RefreshStore, definition segmentport.Definition, reference time.Time) ([]int64, error) {
	ast, err := dsl.Parse(definition)
	if err != nil {
		return nil, ErrInvalidSegmentRefresh
	}
	program, err := compiler.Compile(ast, reference)
	if err != nil {
		return nil, ErrInvalidSegmentRefresh
	}
	queries, err := store.QuerySet(ctx)
	if err != nil || queries == nil {
		return nil, errors.Join(ErrSegmentRefreshFailed, err)
	}
	ids, err := compiler.Execute(ctx, program, queries)
	if err != nil {
		return nil, errors.Join(ErrSegmentRefreshFailed, err)
	}
	return ids, nil
}
