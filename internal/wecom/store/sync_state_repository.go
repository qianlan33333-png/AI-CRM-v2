// Package store persists WeCom-owned cursor state through the generated query
// family. It never writes Contact or Identity tables.
package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

type SyncStateRepository struct{ db wecomdb.DBTX }

var _ wecomapp.SyncStateStore = (*SyncStateRepository)(nil)

func NewSyncStateRepository(db wecomdb.DBTX) *SyncStateRepository {
	return &SyncStateRepository{db: db}
}

func (repository *SyncStateRepository) LoadCursor(ctx context.Context, key string) (wecomapp.CursorState, error) {
	if repository == nil || repository.db == nil || !validSyncKey(key) {
		return wecomapp.CursorState{}, wecomapp.ErrInvalidCursorSync
	}
	row, err := wecomdb.New(repository.db).LoadWeComSyncState(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return wecomapp.CursorState{}, nil
	}
	if err != nil {
		return wecomapp.CursorState{}, err
	}
	if !validCursor(row.Cursor) || (row.Completed && row.Cursor != "") {
		return wecomapp.CursorState{}, wecomapp.ErrCursorSyncFailed
	}
	return wecomapp.CursorState{Cursor: row.Cursor, Completed: row.Completed}, nil
}

func (repository *SyncStateRepository) AdvanceCursor(
	ctx context.Context,
	key, expectedCursor, nextCursor string,
	completed bool,
) error {
	if repository == nil || !validSyncKey(key) || !validCursor(expectedCursor) || !validCursor(nextCursor) || completed != (nextCursor == "") {
		return wecomapp.ErrInvalidCursorSync
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	row, err := wecomdb.New(tx).AdvanceWeComSyncState(ctx, wecomdb.AdvanceWeComSyncStateParams{
		SyncKey: key, Cursor: nextCursor, Completed: completed, ExpectedCursor: expectedCursor,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return wecomapp.ErrCursorAdvanced
	}
	if err != nil {
		return err
	}
	if row.Cursor != nextCursor || row.Completed != completed {
		return wecomapp.ErrCursorSyncFailed
	}
	return nil
}

func validSyncKey(value string) bool {
	return value != "" && len(value) <= 200 && strings.TrimSpace(value) == value
}

func validCursor(value string) bool {
	return len(value) <= 512 && strings.TrimSpace(value) == value
}
