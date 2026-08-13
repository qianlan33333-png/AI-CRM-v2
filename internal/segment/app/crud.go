package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const (
	SegmentDefaultLimit = int32(50)
	SegmentMaximumLimit = int32(200)
	segmentCursorMax    = 512
	segmentCursorOp     = "listSegments"
	memberCursorOp      = "listSegmentMembers"
)

var (
	ErrInvalidSegmentCommand  = errors.New("invalid segment command")
	ErrInvalidSegmentCursor   = errors.New("invalid segment cursor")
	ErrSegmentCommandConflict = errors.New("segment command conflict")
	ErrSegmentCRUDUnavailable = errors.New("segment CRUD unavailable")
)

type Operation string

const (
	OperationCreate Operation = "create"
	OperationUpdate Operation = "update"
)

type Receipt struct {
	ID              int64
	Operation       Operation
	ActorScope      string
	KeyDigest       [32]byte
	PayloadDigest   [32]byte
	State           string
	ResultSegmentID segmentport.SegmentID
}

type ReceiptReservation struct {
	Operation     Operation
	ActorScope    string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type SegmentPageQuery struct {
	AfterID *segmentport.SegmentID
	Limit   int32
}

type MemberRecord struct {
	ID             segmentport.CustomerID
	Name           string
	AvatarURL      *string
	Gender         *int16
	StageID        *int64
	OwnerStaffID   *int64
	ChannelID      *int64
	AddedAt        *time.Time
	LastInteractAt *time.Time
	IsDeleted      bool
	Extra          json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type MemberPage struct {
	Items      []MemberRecord
	NextCursor string
}

type CRUDStore interface {
	ListSegments(context.Context, SegmentPageQuery) ([]segmentport.Segment, error)
	GetSegment(context.Context, segmentport.SegmentID) (segmentport.Segment, error)
	LockSegment(context.Context, segmentport.SegmentID) (segmentport.Segment, error)
	CreateSegment(context.Context, segmentport.CreateCommand, time.Time) (segmentport.Segment, error)
	UpdateSegment(context.Context, segmentport.Segment, time.Time) (segmentport.Segment, error)
	ListMemberRecords(context.Context, segmentport.SegmentID, *segmentport.CustomerID, int32) ([]MemberRecord, error)
	ReserveReceipt(context.Context, ReceiptReservation) (Receipt, bool, error)
	CompleteReceipt(context.Context, int64, segmentport.SegmentID, time.Time) (Receipt, error)
}

type UpdateInput struct {
	SegmentID      segmentport.SegmentID
	Name           *string
	Definition     *segmentport.Definition
	RefreshMode    *segmentport.RefreshMode
	RefreshCron    *string
	RefreshCronSet bool
	Actor          segmentport.Actor
	IdempotencyKey string
}

type CRUDService struct {
	uow    platformport.UnitOfWork
	store  CRUDStore
	events eventport.Appender
	now    func() time.Time
}

func NewCRUDService(uow platformport.UnitOfWork, store CRUDStore, events eventport.Appender) *CRUDService {
	return &CRUDService{uow: uow, store: store, events: events, now: time.Now}
}

func (service *CRUDService) List(ctx context.Context, cursor string, limit int32) (segmentport.Page, error) {
	limit, err := normalizeLimit(limit)
	if err != nil || len(cursor) > segmentCursorMax {
		return segmentport.Page{}, ErrInvalidSegmentCursor
	}
	var after *segmentport.SegmentID
	if cursor != "" {
		id, decodeErr := decodeCursor(cursor, segmentCursorOp)
		if decodeErr != nil {
			return segmentport.Page{}, decodeErr
		}
		segmentID := segmentport.SegmentID(id)
		after = &segmentID
	}
	if !service.ready(ctx) {
		return segmentport.Page{}, ErrSegmentCRUDUnavailable
	}
	var rows []segmentport.Segment
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		rows, storeErr = service.store.ListSegments(txCtx, SegmentPageQuery{AfterID: after, Limit: limit + 1})
		return storeErr
	})
	if err != nil {
		return segmentport.Page{}, errors.Join(ErrSegmentCRUDUnavailable, err)
	}
	if len(rows) > int(limit)+1 {
		return segmentport.Page{}, ErrSegmentCRUDUnavailable
	}
	page := segmentport.Page{Items: rows}
	if len(rows) > int(limit) {
		page.Items = rows[:limit]
		page.NextCursor, err = encodeCursor(segmentCursorOp, int64(page.Items[len(page.Items)-1].ID))
	}
	if err != nil || !validSegments(page.Items) {
		return segmentport.Page{}, ErrSegmentCRUDUnavailable
	}
	return page, nil
}

func (service *CRUDService) Get(ctx context.Context, id segmentport.SegmentID) (segmentport.Segment, error) {
	if id <= 0 {
		return segmentport.Segment{}, ErrSegmentNotFound
	}
	if !service.ready(ctx) {
		return segmentport.Segment{}, ErrSegmentCRUDUnavailable
	}
	var segment segmentport.Segment
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		segment, storeErr = service.store.GetSegment(txCtx, id)
		return storeErr
	})
	if err != nil {
		return segmentport.Segment{}, classifyCRUDStoreError(err)
	}
	if !validSegment(segment) {
		return segmentport.Segment{}, ErrSegmentCRUDUnavailable
	}
	return cloneSegment(segment), nil
}

func (service *CRUDService) Create(ctx context.Context, command segmentport.CreateCommand) (segmentport.Segment, error) {
	normalized, payloadDigest, err := normalizeCreate(command)
	if err != nil {
		return segmentport.Segment{}, err
	}
	return service.mutate(ctx, OperationCreate, normalized.Actor, normalized.IdempotencyKey, payloadDigest, func(txCtx context.Context, now time.Time) (segmentport.Segment, error) {
		return service.store.CreateSegment(txCtx, normalized, now)
	})
}

func (service *CRUDService) Update(ctx context.Context, command segmentport.UpdateCommand) (segmentport.Segment, error) {
	return service.UpdateHTTP(ctx, UpdateInput{
		SegmentID: command.SegmentID, Name: command.Name, Definition: command.Definition,
		RefreshMode: command.RefreshMode, RefreshCron: command.RefreshCron,
		RefreshCronSet: command.RefreshCron != nil, Actor: command.Actor, IdempotencyKey: command.IdempotencyKey,
	})
}

func (service *CRUDService) UpdateHTTP(ctx context.Context, input UpdateInput) (segmentport.Segment, error) {
	normalized, payloadDigest, err := normalizeUpdate(input)
	if err != nil {
		return segmentport.Segment{}, err
	}
	return service.mutate(ctx, OperationUpdate, normalized.Actor, normalized.IdempotencyKey, payloadDigest, func(txCtx context.Context, now time.Time) (segmentport.Segment, error) {
		current, storeErr := service.store.LockSegment(txCtx, normalized.SegmentID)
		if storeErr != nil {
			return segmentport.Segment{}, storeErr
		}
		if normalized.Name != nil {
			current.Name = *normalized.Name
		}
		if normalized.Definition != nil {
			current.Definition = append(segmentport.Definition(nil), (*normalized.Definition)...)
		}
		if normalized.RefreshMode != nil {
			current.RefreshMode = *normalized.RefreshMode
		}
		if normalized.RefreshCronSet {
			current.RefreshCron = cloneString(normalized.RefreshCron)
		}
		cron, cronErr := CanonicalRefreshCron(current.RefreshMode, current.RefreshCron)
		if cronErr != nil {
			return segmentport.Segment{}, ErrInvalidSegmentCommand
		}
		current.RefreshCron = cron
		return service.store.UpdateSegment(txCtx, current, now)
	})
}

func (service *CRUDService) ListMembers(ctx context.Context, segmentID segmentport.SegmentID, cursor string, limit int32) (segmentport.MemberPage, error) {
	page, err := service.ListMemberRecords(ctx, segmentID, cursor, limit)
	if err != nil {
		return segmentport.MemberPage{}, err
	}
	result := segmentport.MemberPage{CustomerIDs: make([]segmentport.CustomerID, len(page.Items)), NextCursor: page.NextCursor}
	for index, member := range page.Items {
		result.CustomerIDs[index] = member.ID
	}
	return result, nil
}

func (service *CRUDService) ListMemberRecords(ctx context.Context, segmentID segmentport.SegmentID, cursor string, limit int32) (MemberPage, error) {
	limit, err := normalizeLimit(limit)
	if err != nil || segmentID <= 0 || len(cursor) > segmentCursorMax {
		return MemberPage{}, ErrInvalidSegmentCursor
	}
	var after *segmentport.CustomerID
	if cursor != "" {
		id, decodeErr := decodeCursor(cursor, memberCursorOp+":"+strconv.FormatInt(int64(segmentID), 10))
		if decodeErr != nil {
			return MemberPage{}, decodeErr
		}
		customerID := segmentport.CustomerID(id)
		after = &customerID
	}
	if !service.ready(ctx) {
		return MemberPage{}, ErrSegmentCRUDUnavailable
	}
	var rows []MemberRecord
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		if _, getErr := service.store.GetSegment(txCtx, segmentID); getErr != nil {
			return getErr
		}
		var storeErr error
		rows, storeErr = service.store.ListMemberRecords(txCtx, segmentID, after, limit+1)
		return storeErr
	})
	if err != nil {
		return MemberPage{}, classifyCRUDStoreError(err)
	}
	if len(rows) > int(limit)+1 || !validMembers(rows) {
		return MemberPage{}, ErrSegmentCRUDUnavailable
	}
	page := MemberPage{Items: rows}
	if len(rows) > int(limit) {
		page.Items = rows[:limit]
		page.NextCursor, err = encodeCursor(memberCursorOp+":"+strconv.FormatInt(int64(segmentID), 10), int64(page.Items[len(page.Items)-1].ID))
	}
	if err != nil {
		return MemberPage{}, ErrSegmentCRUDUnavailable
	}
	return page, nil
}

func (service *CRUDService) mutate(ctx context.Context, operation Operation, actor segmentport.Actor, key string, payloadDigest [32]byte, apply func(context.Context, time.Time) (segmentport.Segment, error)) (segmentport.Segment, error) {
	if !service.ready(ctx) || apply == nil {
		return segmentport.Segment{}, ErrSegmentCRUDUnavailable
	}
	now := service.now().UTC()
	if now.IsZero() {
		return segmentport.Segment{}, ErrSegmentCRUDUnavailable
	}
	reservation := ReceiptReservation{Operation: operation, ActorScope: string(actor), KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: payloadDigest, CreatedAt: now}
	var result segmentport.Segment
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveReceipt(txCtx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !validReceipt(receipt, reservation) {
			return ErrSegmentCRUDUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return ErrSegmentCommandConflict
		}
		if !owned {
			if receipt.State != "completed" || receipt.ResultSegmentID <= 0 {
				return ErrSegmentCRUDUnavailable
			}
			var getErr error
			result, getErr = service.store.GetSegment(txCtx, receipt.ResultSegmentID)
			return getErr
		}
		var applyErr error
		result, applyErr = apply(txCtx, now)
		if applyErr != nil {
			return applyErr
		}
		if !validSegment(result) {
			return ErrSegmentCRUDUnavailable
		}
		payload, marshalErr := json.Marshal(struct {
			SegmentID segmentport.SegmentID `json:"segment_id"`
		}{SegmentID: result.ID})
		if marshalErr != nil {
			return marshalErr
		}
		if _, appendErr := service.events.Append(txCtx, eventport.Event{
			Type: "segment." + string(operation) + "d", Payload: payload, OccurredAt: now,
			IdempotencyKey: eventIdempotencyKey(operation, actor, key),
		}); appendErr != nil {
			return appendErr
		}
		completed, completeErr := service.store.CompleteReceipt(txCtx, receipt.ID, result.ID, now)
		if completeErr != nil || completed.State != "completed" || completed.ResultSegmentID != result.ID {
			return errors.Join(ErrSegmentCRUDUnavailable, completeErr)
		}
		return nil
	})
	if err != nil {
		return segmentport.Segment{}, classifyCRUDStoreError(err)
	}
	return cloneSegment(result), nil
}

func eventIdempotencyKey(operation Operation, actor segmentport.Actor, key string) string {
	digest := sha256.Sum256([]byte(string(actor) + "\x00" + key))
	return "segment." + string(operation) + ":" + hex.EncodeToString(digest[:])
}

func normalizeCreate(command segmentport.CreateCommand) (segmentport.CreateCommand, [32]byte, error) {
	_, err := validCommandCommon(command.Actor, command.IdempotencyKey)
	if err != nil || !validName(command.Name) {
		return segmentport.CreateCommand{}, [32]byte{}, ErrInvalidSegmentCommand
	}
	definition, err := canonicalDefinition(command.Definition)
	if err != nil {
		return segmentport.CreateCommand{}, [32]byte{}, err
	}
	cron, err := CanonicalRefreshCron(command.RefreshMode, command.RefreshCron)
	if err != nil {
		return segmentport.CreateCommand{}, [32]byte{}, ErrInvalidSegmentCommand
	}
	command.Definition, command.RefreshCron = definition, cron
	payload, _ := json.Marshal(struct {
		Name        string                  `json:"name"`
		Definition  json.RawMessage         `json:"definition"`
		RefreshMode segmentport.RefreshMode `json:"refresh_mode"`
		RefreshCron *string                 `json:"refresh_cron"`
	}{command.Name, json.RawMessage(definition), command.RefreshMode, cron})
	return command, sha256.Sum256(payload), nil
}

func normalizeUpdate(input UpdateInput) (UpdateInput, [32]byte, error) {
	if input.SegmentID <= 0 || (input.Name == nil && input.Definition == nil && input.RefreshMode == nil && !input.RefreshCronSet) {
		return UpdateInput{}, [32]byte{}, ErrInvalidSegmentCommand
	}
	if _, err := validCommandCommon(input.Actor, input.IdempotencyKey); err != nil {
		return UpdateInput{}, [32]byte{}, ErrInvalidSegmentCommand
	}
	if input.Name != nil && !validName(*input.Name) {
		return UpdateInput{}, [32]byte{}, ErrInvalidSegmentCommand
	}
	if input.Definition != nil {
		definition, err := canonicalDefinition(*input.Definition)
		if err != nil {
			return UpdateInput{}, [32]byte{}, err
		}
		input.Definition = &definition
	}
	if input.RefreshMode != nil && *input.RefreshMode != segmentport.RefreshModeManual && *input.RefreshMode != segmentport.RefreshModeScheduled {
		return UpdateInput{}, [32]byte{}, ErrInvalidSegmentCommand
	}
	if input.RefreshCron != nil {
		cron, err := CanonicalRefreshCron(segmentport.RefreshModeScheduled, input.RefreshCron)
		if err != nil {
			return UpdateInput{}, [32]byte{}, ErrInvalidSegmentCommand
		}
		input.RefreshCron = cron
	}
	payload, _ := json.Marshal(struct {
		SegmentID      segmentport.SegmentID    `json:"segment_id"`
		Name           *string                  `json:"name"`
		Definition     *segmentport.Definition  `json:"definition"`
		RefreshMode    *segmentport.RefreshMode `json:"refresh_mode"`
		RefreshCronSet bool                     `json:"refresh_cron_set"`
		RefreshCron    *string                  `json:"refresh_cron"`
	}{input.SegmentID, input.Name, input.Definition, input.RefreshMode, input.RefreshCronSet, input.RefreshCron})
	return input, sha256.Sum256(payload), nil
}

func canonicalDefinition(raw segmentport.Definition) (segmentport.Definition, error) {
	ast, err := dsl.Parse(raw)
	if err != nil {
		return nil, err
	}
	canonical, err := ast.CanonicalJSON()
	if err != nil {
		return nil, err
	}
	return segmentport.Definition(canonical), nil
}

func validCommandCommon(actor segmentport.Actor, key string) (int64, error) {
	if len(key) < 16 || len(key) > 128 || strings.TrimSpace(key) != key {
		return 0, ErrInvalidSegmentCommand
	}
	text := strings.TrimPrefix(string(actor), "admin:")
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 || text == string(actor) {
		return 0, ErrInvalidSegmentCommand
	}
	return id, nil
}

func validName(name string) bool {
	return name != "" && strings.TrimSpace(name) != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 200
}

type cursorPayload struct {
	Version   int    `json:"v"`
	Operation string `json:"operation"`
	Sort      string `json:"sort"`
	ID        int64  `json:"id"`
}

func encodeCursor(operation string, id int64) (string, error) {
	if operation == "" || id <= 0 {
		return "", ErrSegmentCRUDUnavailable
	}
	payload, err := json.Marshal(cursorPayload{1, operation, "id_asc", id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(raw, operation string) (int64, error) {
	if raw == "" || len(raw) > segmentCursorMax || strings.Contains(raw, "=") {
		return 0, ErrInvalidSegmentCursor
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return 0, ErrInvalidSegmentCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if decoder.Decode(&payload) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) || payload.Version != 1 || payload.Operation != operation || payload.Sort != "id_asc" || payload.ID <= 0 {
		return 0, ErrInvalidSegmentCursor
	}
	return payload.ID, nil
}

func normalizeLimit(limit int32) (int32, error) {
	if limit == 0 {
		return SegmentDefaultLimit, nil
	}
	if limit < 1 || limit > SegmentMaximumLimit {
		return 0, ErrInvalidSegmentCursor
	}
	return limit, nil
}

func (service *CRUDService) ready(ctx context.Context) bool {
	return ctx != nil && service != nil && !nilCRUD(service.uow) && !nilCRUD(service.store) && !nilCRUD(service.events) && service.now != nil
}

func nilCRUD(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface || v.Kind() == reflect.Func || v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil()
}

func classifyCRUDStoreError(err error) error {
	switch {
	case errors.Is(err, ErrSegmentNotFound), errors.Is(err, ErrInvalidSegmentCommand), errors.Is(err, ErrInvalidSegmentCursor), errors.Is(err, ErrSegmentCommandConflict):
		return err
	default:
		return errors.Join(ErrSegmentCRUDUnavailable, err)
	}
}

func validReceipt(receipt Receipt, wanted ReceiptReservation) bool {
	return receipt.ID > 0 && receipt.Operation == wanted.Operation && receipt.ActorScope == wanted.ActorScope &&
		receipt.KeyDigest == wanted.KeyDigest && (receipt.State == "in_progress" || receipt.State == "completed")
}

func validSegments(items []segmentport.Segment) bool {
	for index, item := range items {
		if !validSegment(item) || (index > 0 && items[index-1].ID >= item.ID) {
			return false
		}
	}
	return true
}

func validSegment(item segmentport.Segment) bool {
	if item.ID <= 0 || !validName(item.Name) || item.MemberCount < 0 || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.CreatedAt.After(item.UpdatedAt) ||
		(item.RefreshStatus != segmentport.RefreshStatusIdle && item.RefreshStatus != segmentport.RefreshStatusRunning && item.RefreshStatus != segmentport.RefreshStatusFailed) {
		return false
	}
	definition, err := canonicalDefinition(item.Definition)
	if err != nil {
		return false
	}
	item.Definition = definition
	_, err = CanonicalRefreshCron(item.RefreshMode, item.RefreshCron)
	return err == nil
}

func validMembers(items []MemberRecord) bool {
	for index, item := range items {
		if item.ID <= 0 || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.CreatedAt.After(item.UpdatedAt) || len(item.Extra) == 0 || !json.Valid(item.Extra) ||
			(index > 0 && items[index-1].ID >= item.ID) {
			return false
		}
	}
	return true
}

func cloneSegment(item segmentport.Segment) segmentport.Segment {
	item.Definition = append(segmentport.Definition(nil), item.Definition...)
	item.RefreshCron = cloneString(item.RefreshCron)
	if item.RefreshedAt != nil {
		value := *item.RefreshedAt
		item.RefreshedAt = &value
	}
	return item
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
