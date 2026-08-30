package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	wecomarchive "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/archive"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

var ErrMessageArchiveSyncStore = errors.New("WeCom message archive sync store unavailable")

type MessageArchiveSyncRepository struct{}

func NewMessageArchiveSyncRepository() *MessageArchiveSyncRepository {
	return &MessageArchiveSyncRepository{}
}

func (*MessageArchiveSyncRepository) State(ctx context.Context) (wecomarchive.State, error) {
	queries, err := messageArchiveQueries(ctx)
	if err != nil {
		return wecomarchive.State{}, errors.Join(ErrMessageArchiveSyncStore, err)
	}
	row, err := queries.GetMessageArchiveSyncState(ctx)
	if err != nil || row.LastSeq < 0 {
		return wecomarchive.State{}, errors.Join(ErrMessageArchiveSyncStore, err)
	}
	return wecomarchive.State{LastSeq: row.LastSeq}, nil
}

func (*MessageArchiveSyncRepository) StartRun(ctx context.Context, cursor int64) (int64, error) {
	queries, err := messageArchiveQueries(ctx)
	if err != nil || cursor < 0 {
		return 0, errors.Join(ErrMessageArchiveSyncStore, err)
	}
	runID, err := queries.StartMessageArchiveSyncRun(ctx, cursor)
	if err != nil || runID < 1 {
		return 0, errors.Join(ErrMessageArchiveSyncStore, err)
	}
	return runID, nil
}

func (*MessageArchiveSyncRepository) SaveBatch(ctx context.Context, records []wecomarchive.Record, cursor int64, completedAt time.Time) (inserted, unresolved int64, err error) {
	queries, err := messageArchiveQueries(ctx)
	if err != nil || cursor < 0 || completedAt.IsZero() {
		return 0, 0, errors.Join(ErrMessageArchiveSyncStore, err)
	}
	for _, record := range records {
		customerID := pgtype.Int8{}
		identityState := "unattributed"
		if record.CustomerID != nil && *record.CustomerID > 0 {
			customerID = pgtype.Int8{Int64: int64(*record.CustomerID), Valid: true}
			identityState = "resolved"
		} else {
			unresolved++
			if record.ExternalUserID != "" {
				identityState = "unresolved"
			}
		}
		row, insertErr := queries.UpsertMessageArchiveRecord(ctx, wecomdb.UpsertMessageArchiveRecordParams{
			SourceMessageID: record.SourceMessageID, CustomerID: customerID, ExternalUserid: record.ExternalUserID,
			ChatType: record.ChatType, OwnerUserid: record.OwnerUserID, Sender: record.Sender, Receiver: record.Receiver,
			ChatID: record.ChatID, Roomid: record.RoomID, GroupName: record.GroupName, MessageType: record.MessageType,
			ContentMasked: record.Content, SentAt: pgtype.Timestamptz{Time: record.SentAt.UTC(), Valid: true},
			ProviderSeq: record.ProviderSeq, IdentityState: identityState, SourcePayloadDigest: record.SourcePayloadDigest[:],
		})
		if insertErr != nil || row.ID < 1 {
			return 0, 0, errors.Join(ErrMessageArchiveSyncStore, insertErr)
		}
		if row.Inserted {
			inserted++
		}
	}
	rows, err := queries.AdvanceMessageArchiveSyncState(ctx, wecomdb.AdvanceMessageArchiveSyncStateParams{
		LastSeq: cursor, CompletedAt: pgtype.Timestamptz{Time: completedAt.UTC(), Valid: true},
	})
	if err != nil || rows != 1 {
		return 0, 0, errors.Join(ErrMessageArchiveSyncStore, err)
	}
	return inserted, unresolved, nil
}

func (*MessageArchiveSyncRepository) FinishRun(ctx context.Context, result wecomarchive.RunResult, failureCode string, finishedAt time.Time) error {
	queries, err := messageArchiveQueries(ctx)
	if err != nil || result.RunID < 1 || finishedAt.IsZero() {
		return errors.Join(ErrMessageArchiveSyncStore, err)
	}
	state := "succeeded"
	if failureCode != "" {
		state = "failed"
	}
	rows, err := queries.FinishMessageArchiveSyncRun(ctx, wecomdb.FinishMessageArchiveSyncRunParams{
		ID: result.RunID, State: state, CursorTo: result.CursorTo, FetchedCount: result.Fetched,
		AcceptedCount: result.Accepted, InsertedCount: result.Inserted, UnresolvedCount: result.Unresolved,
		FailureCode: failureCode, FinishedAt: pgtype.Timestamptz{Time: finishedAt.UTC(), Valid: true},
	})
	if err != nil || rows != 1 {
		return errors.Join(ErrMessageArchiveSyncStore, err)
	}
	return nil
}

func (*MessageArchiveSyncRepository) ResolvePending(ctx context.Context, scope string) (int64, error) {
	queries, err := messageArchiveQueries(ctx)
	if err != nil || scope == "" {
		return 0, errors.Join(ErrMessageArchiveSyncStore, err)
	}
	resolved, err := queries.ResolveMessageArchiveRecords(ctx, scope)
	if err != nil || resolved < 0 {
		return 0, errors.Join(ErrMessageArchiveSyncStore, err)
	}
	return resolved, nil
}

var _ wecomarchive.Store = (*MessageArchiveSyncRepository)(nil)
