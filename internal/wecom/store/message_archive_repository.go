package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

type MessageArchiveRepository struct{}

var _ wecomapp.MessageArchiveStore = (*MessageArchiveRepository)(nil)

func NewMessageArchiveRepository() *MessageArchiveRepository { return &MessageArchiveRepository{} }

func (*MessageArchiveRepository) ReserveMessageArchiveSync(ctx context.Context, command wecomapp.ArchiveSyncCommand, digest []byte) (wecomapp.ArchiveSyncReceipt, []byte, error) {
	queries, err := messageArchiveQueries(ctx)
	if err != nil {
		return wecomapp.ArchiveSyncReceipt{}, nil, err
	}
	row, err := queries.ReserveMessageArchiveSyncReceipt(ctx, wecomdb.ReserveMessageArchiveSyncReceiptParams{
		IdempotencyScope: command.Actor, IdempotencyKey: command.IdempotencyKey, RequestDigest: digest,
	})
	if err != nil {
		return wecomapp.ArchiveSyncReceipt{}, nil, err
	}
	return messageArchiveReceipt(row.ID, row.State, row.AcceptedEventID), append([]byte(nil), row.RequestDigest...), nil
}

func (*MessageArchiveRepository) AcceptMessageArchiveSync(ctx context.Context, receiptID int64, eventID eventport.EventID) (wecomapp.ArchiveSyncReceipt, []byte, error) {
	queries, err := messageArchiveQueries(ctx)
	if err != nil {
		return wecomapp.ArchiveSyncReceipt{}, nil, err
	}
	row, err := queries.AcceptMessageArchiveSyncReceipt(ctx, wecomdb.AcceptMessageArchiveSyncReceiptParams{ID: receiptID, AcceptedEventID: int64(eventID)})
	if err != nil {
		return wecomapp.ArchiveSyncReceipt{}, nil, err
	}
	return messageArchiveReceipt(row.ID, row.State, row.AcceptedEventID), append([]byte(nil), row.RequestDigest...), nil
}

func (*MessageArchiveRepository) MessageArchiveHealth(ctx context.Context) (wecomapp.ArchiveHealth, error) {
	queries, err := messageArchiveQueries(ctx)
	if err != nil {
		return wecomapp.ArchiveHealth{}, err
	}
	row, err := queries.MessageArchiveHealth(ctx)
	if err != nil {
		return wecomapp.ArchiveHealth{}, err
	}
	health := wecomapp.ArchiveHealth{RecordCount: row.RecordCount, AcceptedSyncCount: row.AcceptedSyncCount}
	if row.AcceptedSyncCount > 0 {
		acceptedAt, err := queries.ListMessageArchiveLastAcceptedAt(ctx)
		if err != nil || len(acceptedAt) != 1 || !acceptedAt[0].Valid {
			return wecomapp.ArchiveHealth{}, fmt.Errorf("message archive health accepted timestamp: %w", err)
		}
		value := acceptedAt[0].Time.UTC()
		health.LastAcceptedAt = &value
	}
	return health, nil
}

func (*MessageArchiveRepository) ListMessageArchive(ctx context.Context, query wecomapp.ArchiveQuery) ([]wecomapp.ArchiveMessage, int64, error) {
	queries, err := messageArchiveQueries(ctx)
	if err != nil {
		return nil, 0, err
	}
	if query.External {
		total, err := queries.CountMessageArchiveExternalRecords(ctx, wecomdb.CountMessageArchiveExternalRecordsParams{
			CustomerID: int64(query.CustomerID), ChatType: query.ChatType, StartedAt: pgTimestamp(query.StartedAt), WithUserid: query.WithUserID,
		})
		if err != nil {
			return nil, 0, err
		}
		rows, err := queries.ListMessageArchiveExternalRecords(ctx, wecomdb.ListMessageArchiveExternalRecordsParams{
			CustomerID: int64(query.CustomerID), ChatType: query.ChatType, StartedAt: pgTimestamp(query.StartedAt), WithUserid: query.WithUserID,
			RowLimit: query.Limit, RowOffset: query.Offset,
		})
		if err != nil {
			return nil, 0, err
		}
		return mapArchiveExternalRows(rows), total, nil
	}
	rows, err := queries.ListMessageArchiveRecords(ctx, wecomdb.ListMessageArchiveRecordsParams{
		CustomerID: int64(query.CustomerID), ChatType: query.ChatType, Keyword: query.Keyword, RowLimit: query.Limit, RowOffset: query.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	return mapArchiveRows(rows), int64(len(rows)), nil
}

func messageArchiveQueries(ctx context.Context) (*wecomdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return wecomdb.New(tx), nil
}

func messageArchiveReceipt(id int64, state string, eventID pgtype.Int8) wecomapp.ArchiveSyncReceipt {
	receipt := wecomapp.ArchiveSyncReceipt{ID: id, State: wecomapp.ArchiveSyncState(state)}
	if eventID.Valid {
		receipt.EventID = eventport.EventID(eventID.Int64)
	}
	return receipt
}

func mapArchiveRows(rows []wecomdb.ListMessageArchiveRecordsRow) []wecomapp.ArchiveMessage {
	result := make([]wecomapp.ArchiveMessage, 0, len(rows))
	for _, row := range rows {
		result = append(result, archiveMessage(
			row.ID, row.SourceMessageID, row.ExternalUserid, row.ChatType, row.OwnerUserid, row.Sender, row.Receiver,
			row.ChatID, row.Roomid, row.GroupName, row.MessageType, row.ContentMasked, row.SentAt,
		))
	}
	return result
}

func mapArchiveExternalRows(rows []wecomdb.ListMessageArchiveExternalRecordsRow) []wecomapp.ArchiveMessage {
	result := make([]wecomapp.ArchiveMessage, 0, len(rows))
	for _, row := range rows {
		result = append(result, archiveMessage(
			row.ID, row.SourceMessageID, row.ExternalUserid, row.ChatType, row.OwnerUserid, row.Sender, row.Receiver,
			row.ChatID, row.Roomid, row.GroupName, row.MessageType, row.ContentMasked, row.SentAt,
		))
	}
	return result
}

func archiveMessage(id int64, sourceID, externalID, chatType, owner, sender, receiver, chatID, roomID, groupName, messageType, content string, sentAt pgtype.Timestamptz) wecomapp.ArchiveMessage {
	value := time.Time{}
	if sentAt.Valid {
		value = sentAt.Time.UTC()
	}
	return wecomapp.ArchiveMessage{ID: strconv.FormatInt(id, 10), SourceMessageID: sourceID, ExternalUserID: externalID,
		ChatType: chatType, WithUserID: owner, Sender: sender, Receiver: receiver, ChatID: chatID, RoomID: roomID,
		GroupName: groupName, MessageType: messageType, Content: content, SentAt: value}
}

func pgTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
