// Package app closes the W4 cursor-resume behavior without assigning an
// external identifier to Identity or Contact.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

const maxCursorAdvanceAttempts = 3

var (
	ErrInvalidCursorSync = errors.New("invalid WeCom cursor sync")
	ErrCursorSyncFailed  = errors.New("WeCom cursor sync failed")
	ErrCursorSyncDone    = errors.New("WeCom cursor sync is complete")
	ErrCursorAdvanced    = errors.New("WeCom cursor advanced concurrently")
)

// ExternalContactPageReader has no provider write method. The concrete W3
// client implements it with GET /cgi-bin/externalcontact/list.
type ExternalContactPageReader interface {
	ListExternalContacts(context.Context, string, string) (wecomclient.ExternalContactPage, error)
}

// SyncStateStore persists only the opaque resume cursor and completion fact.
// It neither stores profiles nor writes to Contact or Identity tables.
type SyncStateStore interface {
	LoadCursor(context.Context, string) (CursorState, error)
	AdvanceCursor(context.Context, string, string, string, bool) error
}

type SyncHandoff struct {
	FactID         string
	CorpID         string
	ExternalUserID string
	Payload        []byte
	OccurredAt     time.Time
}

type SyncHandoffStore interface {
	ReserveSyncFact(context.Context, SyncHandoff) (InboundReservation, error)
	MarkInboundQueued(context.Context, int64, int64) error
}

type SyncJobInserter interface {
	Insert(context.Context, InboundJobArgs) (int64, error)
}

type CursorState struct {
	Cursor    string
	Completed bool
}

// ExternalContactSyncService makes a page visible only after its successor
// cursor is persisted. A restarted service then resumes from that committed
// successor and a completed run never falls back to the first page.
type ExternalContactSyncService struct {
	uow     platformport.UnitOfWork
	reader  ExternalContactPageReader
	state   SyncStateStore
	handoff SyncHandoffStore
	jobs    SyncJobInserter
	corpID  string
	clock   func() time.Time
}

func NewExternalContactSyncService(
	uow platformport.UnitOfWork,
	reader ExternalContactPageReader,
	state SyncStateStore,
) *ExternalContactSyncService {
	return &ExternalContactSyncService{uow: uow, reader: reader, state: state}
}

func NewExternalContactSyncServiceWithHandoff(
	uow platformport.UnitOfWork,
	reader ExternalContactPageReader,
	state SyncStateStore,
	handoff SyncHandoffStore,
	jobs SyncJobInserter,
	corpID string,
	clock func() time.Time,
) (*ExternalContactSyncService, error) {
	if isNilDependency(uow) || isNilDependency(reader) || isNilDependency(state) ||
		isNilDependency(handoff) || isNilDependency(jobs) || !validCorpID(corpID) || clock == nil {
		return nil, ErrInvalidCursorSync
	}
	return &ExternalContactSyncService{uow: uow, reader: reader, state: state, handoff: handoff, jobs: jobs, corpID: corpID, clock: clock}, nil
}

func (service *ExternalContactSyncService) SyncNext(ctx context.Context, staffUserID string) (wecomclient.ExternalContactPage, error) {
	if service == nil || ctx == nil || !validStaffUserID(staffUserID) || isNilDependency(service.uow) ||
		isNilDependency(service.reader) || isNilDependency(service.state) {
		return wecomclient.ExternalContactPage{}, ErrInvalidCursorSync
	}
	key := "external_contact_list:" + staffUserID
	for attempt := 0; attempt < maxCursorAdvanceAttempts; attempt++ {
		state, err := service.state.LoadCursor(ctx, key)
		if err != nil {
			return wecomclient.ExternalContactPage{}, fmt.Errorf("%w: %w", ErrCursorSyncFailed, err)
		}
		if state.Completed {
			return wecomclient.ExternalContactPage{}, ErrCursorSyncDone
		}
		page, err := service.reader.ListExternalContacts(ctx, staffUserID, state.Cursor)
		if err != nil {
			return wecomclient.ExternalContactPage{}, fmt.Errorf("%w: %w", ErrCursorSyncFailed, err)
		}
		if !validPage(page) || (page.NextCursor != "" && page.NextCursor == state.Cursor) {
			return wecomclient.ExternalContactPage{}, ErrCursorSyncFailed
		}
		err = service.uow.Within(ctx, func(txCtx context.Context) error {
			if service.handoff != nil || service.jobs != nil {
				if service.handoff == nil || service.jobs == nil || service.corpID == "" || service.clock == nil {
					return ErrCursorSyncFailed
				}
				for index, externalUserID := range page.ExternalUserIDs {
					factID := syncFactID(service.corpID, key, state.Cursor, index, externalUserID)
					payload, marshalErr := json.Marshal(map[string]string{
						"source": "directory_sync", "sync_key": key, "external_userid": externalUserID,
					})
					if marshalErr != nil {
						return marshalErr
					}
					reservation, reserveErr := service.handoff.ReserveSyncFact(txCtx, SyncHandoff{
						FactID: factID, CorpID: service.corpID, ExternalUserID: externalUserID,
						Payload: payload, OccurredAt: service.clock().UTC(),
					})
					if reserveErr != nil {
						return reserveErr
					}
					if reservation.Inserted {
						jobID, insertErr := service.jobs.Insert(txCtx, InboundJobArgs{InboxID: reservation.ID})
						if insertErr != nil || jobID <= 0 {
							return errors.Join(ErrCursorSyncFailed, insertErr)
						}
						if queueErr := service.handoff.MarkInboundQueued(txCtx, reservation.ID, jobID); queueErr != nil {
							return queueErr
						}
					}
				}
			}
			return service.state.AdvanceCursor(txCtx, key, state.Cursor, page.NextCursor, page.NextCursor == "")
		})
		if errors.Is(err, ErrCursorAdvanced) {
			continue
		}
		if err != nil {
			return wecomclient.ExternalContactPage{}, fmt.Errorf("%w: %w", ErrCursorSyncFailed, err)
		}
		return page, nil
	}
	return wecomclient.ExternalContactPage{}, ErrCursorSyncFailed
}

func syncFactID(corpID, syncKey, cursor string, index int, externalUserID string) string {
	digest := sha256.Sum256([]byte("wecom.sync.v1\x00" + corpID + "\x00" + syncKey + "\x00" + cursor + "\x00" + strconv.Itoa(index) + "\x00" + externalUserID))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validStaffUserID(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

func validPage(page wecomclient.ExternalContactPage) bool {
	if len(page.NextCursor) > 512 || strings.TrimSpace(page.NextCursor) != page.NextCursor {
		return false
	}
	seen := make(map[string]struct{}, len(page.ExternalUserIDs))
	for _, externalUserID := range page.ExternalUserIDs {
		if externalUserID == "" || len(externalUserID) > 256 || strings.TrimSpace(externalUserID) != externalUserID {
			return false
		}
		if _, duplicate := seen[externalUserID]; duplicate {
			return false
		}
		seen[externalUserID] = struct{}{}
	}
	return true
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}
