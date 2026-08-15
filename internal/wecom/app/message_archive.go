// Package app owns the local, OneID-attributed WeCom message-archive projection.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	MessageArchiveDefaultLimit = 20
	MessageArchiveMaximumLimit = 200
)

var (
	ErrInvalidMessageArchiveQuery = errors.New("invalid message archive query")
	ErrMessageArchiveUnavailable  = errors.New("message archive unavailable")
	ErrMessageArchiveNotFound     = errors.New("message archive customer not found")
	ErrInvalidArchiveSyncCommand  = errors.New("invalid archive sync command")
	ErrArchiveSyncConflict        = errors.New("archive sync idempotency conflict")
	ErrArchiveSyncFailed          = errors.New("archive sync request failed")
)

type ArchiveMessage struct {
	ID, SourceMessageID, ExternalUserID, ChatType, WithUserID string
	Sender, Receiver, ChatID, RoomID, GroupName, MessageType  string
	Content                                                   string
	SentAt                                                    time.Time
}

type ArchiveHealth struct {
	RecordCount, AcceptedSyncCount int64
	LastAcceptedAt                 *time.Time
}

type ArchiveQuery struct {
	CustomerID                    contactport.CustomerID
	ChatType, Keyword, WithUserID string
	StartedAt                     *time.Time
	Limit, Offset                 int32
	External                      bool
}

type ArchiveSyncCommand struct {
	Actor, IdempotencyKey, StartTime, EndTime, OwnerUserID, Cursor string
	Limit, MaxPages                                                int64
}

type ArchiveSyncState string

const ArchiveSyncAccepted ArchiveSyncState = "accepted"

type ArchiveSyncReceipt struct {
	ID      int64
	State   ArchiveSyncState
	EventID eventport.EventID
}

type MessageArchiveStore interface {
	ReserveMessageArchiveSync(context.Context, ArchiveSyncCommand, []byte) (ArchiveSyncReceipt, []byte, error)
	AcceptMessageArchiveSync(context.Context, int64, eventport.EventID) (ArchiveSyncReceipt, []byte, error)
	MessageArchiveHealth(context.Context) (ArchiveHealth, error)
	ListMessageArchive(context.Context, ArchiveQuery) ([]ArchiveMessage, int64, error)
}

type MessageArchiveService struct {
	uow    platformport.UnitOfWork
	store  MessageArchiveStore
	events eventport.Appender
	now    func() time.Time
}

func NewMessageArchiveService(uow platformport.UnitOfWork, store MessageArchiveStore, events eventport.Appender) *MessageArchiveService {
	return &MessageArchiveService{uow: uow, store: store, events: events, now: time.Now}
}

func (service *MessageArchiveService) Health(ctx context.Context) (ArchiveHealth, error) {
	if !messageArchiveReady(service) || ctx == nil {
		return ArchiveHealth{}, ErrMessageArchiveUnavailable
	}
	var health ArchiveHealth
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		health, err = service.store.MessageArchiveHealth(tx)
		return err
	})
	if err != nil || health.RecordCount < 0 || health.AcceptedSyncCount < 0 || (health.LastAcceptedAt != nil && health.LastAcceptedAt.IsZero()) {
		return ArchiveHealth{}, errors.Join(ErrMessageArchiveUnavailable, err)
	}
	return cloneArchiveHealth(health), nil
}

func (service *MessageArchiveService) List(ctx context.Context, query ArchiveQuery) ([]ArchiveMessage, int64, error) {
	if !validArchiveQuery(query) {
		return nil, 0, ErrInvalidMessageArchiveQuery
	}
	if !messageArchiveReady(service) || ctx == nil {
		return nil, 0, ErrMessageArchiveUnavailable
	}
	var records []ArchiveMessage
	var total int64
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		records, total, err = service.store.ListMessageArchive(tx, query)
		return err
	})
	if err != nil {
		return nil, 0, errors.Join(ErrMessageArchiveUnavailable, err)
	}
	if total < 0 || !validArchiveMessages(records, query) {
		return nil, 0, ErrMessageArchiveUnavailable
	}
	return cloneArchiveMessages(records), total, nil
}

// RequestSync records only the accepted command boundary. It deliberately has
// no provider client, worker dispatch, or automatic retry path.
func (service *MessageArchiveService) RequestSync(ctx context.Context, command ArchiveSyncCommand) (ArchiveSyncReceipt, error) {
	if !validArchiveSyncCommand(command) {
		return ArchiveSyncReceipt{}, ErrInvalidArchiveSyncCommand
	}
	if !messageArchiveReady(service) || ctx == nil {
		return ArchiveSyncReceipt{}, ErrArchiveSyncFailed
	}
	digest := archiveSyncDigest(command)
	var accepted ArchiveSyncReceipt
	err := service.uow.Within(ctx, func(tx context.Context) error {
		receipt, storedDigest, err := service.store.ReserveMessageArchiveSync(tx, command, digest[:])
		if err != nil {
			return err
		}
		if receipt.ID <= 0 || len(storedDigest) != len(digest) || !sameArchiveDigest(storedDigest, digest[:]) {
			return ErrArchiveSyncConflict
		}
		switch receipt.State {
		case ArchiveSyncAccepted:
			if receipt.EventID <= 0 {
				return ErrArchiveSyncFailed
			}
			accepted = receipt
			return nil
		case "reserved":
			payload, err := json.Marshal(map[string]any{"receipt_id": receipt.ID, "state": ArchiveSyncAccepted})
			if err != nil {
				return err
			}
			eventID, err := service.events.Append(tx, eventport.Event{
				Type: "wecom.message_archive_sync_accepted", Payload: payload, OccurredAt: service.now().UTC(),
				IdempotencyKey: fmt.Sprintf("wecom.message_archive_sync.accepted:%d", receipt.ID),
			})
			if err != nil || eventID <= 0 {
				return errors.Join(ErrArchiveSyncFailed, err)
			}
			acceptedReceipt, acceptedDigest, err := service.store.AcceptMessageArchiveSync(tx, receipt.ID, eventID)
			if err != nil || acceptedReceipt.ID != receipt.ID || acceptedReceipt.State != ArchiveSyncAccepted ||
				acceptedReceipt.EventID != eventID || !sameArchiveDigest(acceptedDigest, digest[:]) {
				return errors.Join(ErrArchiveSyncFailed, err)
			}
			accepted = acceptedReceipt
			return nil
		default:
			return ErrArchiveSyncFailed
		}
	})
	if err != nil {
		if errors.Is(err, ErrArchiveSyncConflict) || errors.Is(err, ErrInvalidArchiveSyncCommand) {
			return ArchiveSyncReceipt{}, err
		}
		return ArchiveSyncReceipt{}, errors.Join(ErrArchiveSyncFailed, err)
	}
	return accepted, nil
}

func validArchiveQuery(query ArchiveQuery) bool {
	if query.CustomerID <= 0 || query.Limit < 1 || query.Limit > MessageArchiveMaximumLimit || query.Offset < 0 ||
		(query.ChatType != "" && query.ChatType != "private" && query.ChatType != "group") ||
		(query.StartedAt != nil && query.StartedAt.IsZero()) || !validArchiveText(query.Keyword, 200) ||
		!validArchiveText(query.WithUserID, 256) {
		return false
	}
	return !query.External || query.StartedAt != nil && query.ChatType != ""
}

func validArchiveMessages(records []ArchiveMessage, query ArchiveQuery) bool {
	if int64(len(records)) > int64(query.Limit) {
		return false
	}
	for index, record := range records {
		if record.ID == "" || record.SourceMessageID == "" || record.ExternalUserID == "" ||
			(record.ChatType != "private" && record.ChatType != "group") || record.MessageType == "" || record.SentAt.IsZero() ||
			!validArchiveText(record.Content, 20_000) || !validArchiveText(record.WithUserID, 256) ||
			!validArchiveText(record.Sender, 1024) || !validArchiveText(record.Receiver, 1024) {
			return false
		}
		if query.External && query.StartedAt != nil && record.SentAt.Before(*query.StartedAt) {
			return false
		}
		if query.ChatType != "" && record.ChatType != query.ChatType {
			return false
		}
		if query.Keyword != "" && !strings.Contains(strings.ToLower(record.Content), strings.ToLower(query.Keyword)) {
			return false
		}
		if index > 0 && query.External && records[index-1].SentAt.After(record.SentAt) {
			return false
		}
		if index > 0 && !query.External && records[index-1].SentAt.Before(record.SentAt) {
			return false
		}
	}
	return true
}

func validArchiveSyncCommand(command ArchiveSyncCommand) bool {
	return validArchiveText(command.Actor, 200) && len(command.IdempotencyKey) >= 16 && len(command.IdempotencyKey) <= 128 &&
		strings.TrimSpace(command.IdempotencyKey) == command.IdempotencyKey && validArchiveText(command.StartTime, 64) &&
		validArchiveText(command.EndTime, 64) && validArchiveText(command.OwnerUserID, 256) && validArchiveText(command.Cursor, 512) &&
		command.Limit > 0 && command.MaxPages > 0
}

func validArchiveText(value string, maximum int) bool {
	return len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func archiveSyncDigest(command ArchiveSyncCommand) [32]byte {
	return sha256.Sum256([]byte(strings.Join([]string{
		command.Actor, command.StartTime, command.EndTime, command.OwnerUserID, command.Cursor,
		fmt.Sprintf("%d", command.Limit), fmt.Sprintf("%d", command.MaxPages),
	}, "\x00")))
}

func sameArchiveDigest(left, right []byte) bool {
	return len(left) == 32 && len(right) == 32 && string(left) == string(right)
}

func cloneArchiveMessages(records []ArchiveMessage) []ArchiveMessage {
	return append([]ArchiveMessage(nil), records...)
}

func cloneArchiveHealth(health ArchiveHealth) ArchiveHealth {
	if health.LastAcceptedAt != nil {
		value := health.LastAcceptedAt.UTC()
		health.LastAcceptedAt = &value
	}
	return health
}

func messageArchiveReady(service *MessageArchiveService) bool {
	return service != nil && !nilMessageArchiveDependency(service.uow) && !nilMessageArchiveDependency(service.store) &&
		!nilMessageArchiveDependency(service.events) && service.now != nil
}

func nilMessageArchiveDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}
