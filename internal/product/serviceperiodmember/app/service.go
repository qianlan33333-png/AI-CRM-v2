package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

const (
	operationAdd           = "service_period_member.add"
	operationExpire        = "service_period_member.expire"
	operationRemove        = "service_period_member.remove"
	operationUpdateFields  = "service_period_member.update_fields"
	eventTypeMemberChanged = "service_period_member.changed"
)

type Service struct {
	uow    platformport.UnitOfWork
	store  memberport.Store
	events eventport.Appender
	codec  *CursorCodec
	now    func() time.Time
	random io.Reader
}

var _ memberport.Application = (*Service)(nil)

func NewService(uow platformport.UnitOfWork, store memberport.Store, events eventport.Appender, codec *CursorCodec) (*Service, error) {
	if nilDependency(uow) || nilDependency(store) || nilDependency(events) || codec == nil || len(codec.secret) < 32 {
		return nil, memberport.ErrUnavailable
	}
	return &Service{uow: uow, store: store, events: events, codec: codec, now: time.Now, random: rand.Reader}, nil
}

func (service *Service) Add(ctx context.Context, command memberport.AddCommand) (memberdomain.Member, error) {
	normalized, payload, err := normalizeAdd(command)
	if err != nil {
		return memberdomain.Member{}, err
	}
	if normalized.Source == memberdomain.SourcePaidOrder {
		return memberdomain.Member{}, memberport.ErrPaidOrderSourceBlocked
	}
	return service.mutate(ctx, operationAdd, normalized.ActorID, normalized.IdempotencyKey, payload, func(tx context.Context, now time.Time) (memberdomain.Member, error) {
		if normalized.ExpiresAt != nil && !normalized.ExpiresAt.After(now) {
			return memberdomain.Member{}, memberport.ErrInvalidInput
		}
		if err := service.requireFacts(tx, normalized.ServiceProductID, normalized.CustomerID); err != nil {
			return memberdomain.Member{}, err
		}
		memberRef, err := service.newMemberRef()
		if err != nil {
			return memberdomain.Member{}, err
		}
		created, err := service.store.Create(tx, memberport.CreateRecord{
			MemberRef: memberRef, ServiceProductID: normalized.ServiceProductID, CustomerID: normalized.CustomerID,
			Source: normalized.Source, StartsAt: now, ExpiresAt: cloneTime(normalized.ExpiresAt),
			Remark: cloneString(normalized.Remark), Alliance: cloneString(normalized.Alliance), CreatedAt: now,
		})
		if err != nil {
			return memberdomain.Member{}, err
		}
		if !created.Valid() || created.MemberRef != memberRef || created.ServiceProductID != normalized.ServiceProductID ||
			created.CustomerID != normalized.CustomerID || created.Source != normalized.Source || created.State != memberdomain.StateActive ||
			created.Version != 1 || !created.StartsAt.Equal(now) || !created.CreatedAt.Equal(now) || !created.UpdatedAt.Equal(now) ||
			!optionalTimeEqual(created.ExpiresAt, normalized.ExpiresAt) || !optionalStringEqual(created.Remark, normalized.Remark) ||
			!optionalStringEqual(created.Alliance, normalized.Alliance) {
			return memberdomain.Member{}, memberport.ErrUnavailable
		}
		return service.strictReadback(tx, created)
	})
}

func (service *Service) Expire(ctx context.Context, command memberport.TransitionCommand) (memberdomain.Member, error) {
	return service.transition(ctx, operationExpire, memberdomain.StateExpired, command)
}

func (service *Service) Remove(ctx context.Context, command memberport.TransitionCommand) (memberdomain.Member, error) {
	return service.transition(ctx, operationRemove, memberdomain.StateRemoved, command)
}

func (service *Service) transition(ctx context.Context, operation string, target memberdomain.State, command memberport.TransitionCommand) (memberdomain.Member, error) {
	normalized, payload, err := normalizeTransition(command)
	if err != nil {
		return memberdomain.Member{}, err
	}
	return service.mutate(ctx, operation, normalized.ActorID, normalized.IdempotencyKey, payload, func(tx context.Context, now time.Time) (memberdomain.Member, error) {
		current, err := service.store.GetForUpdate(tx, normalized.ServiceProductID, normalized.MemberRef)
		if err != nil {
			return memberdomain.Member{}, err
		}
		if !current.Valid() || current.ServiceProductID != normalized.ServiceProductID || current.MemberRef != normalized.MemberRef {
			return memberdomain.Member{}, memberport.ErrUnavailable
		}
		if current.Version != normalized.ExpectedVersion {
			return memberdomain.Member{}, memberport.ErrConflict
		}
		if target == memberdomain.StateExpired && current.State != memberdomain.StateActive ||
			target == memberdomain.StateRemoved && current.State == memberdomain.StateRemoved {
			return memberdomain.Member{}, memberport.ErrConflict
		}
		updated, err := service.store.Transition(tx, memberport.TransitionRecord{
			ServiceProductID: normalized.ServiceProductID, MemberRef: normalized.MemberRef,
			ExpectedVersion: normalized.ExpectedVersion, Target: target, TransitionedAt: now,
		})
		if err != nil {
			return memberdomain.Member{}, err
		}
		if !updated.Valid() || updated.State != target || updated.Version != current.Version+1 || !sameImmutableMember(updated, current) ||
			(target == memberdomain.StateExpired && !optionalTimeEqual(updated.ExpiredAt, &now)) ||
			(target == memberdomain.StateRemoved && !optionalTimeEqual(updated.RemovedAt, &now)) {
			return memberdomain.Member{}, memberport.ErrUnavailable
		}
		return service.strictReadback(tx, updated)
	})
}

func (service *Service) UpdateFields(ctx context.Context, command memberport.UpdateFieldsCommand) (memberdomain.Member, error) {
	normalized, payload, err := normalizeUpdateFields(command)
	if err != nil {
		return memberdomain.Member{}, err
	}
	return service.mutate(ctx, operationUpdateFields, normalized.ActorID, normalized.IdempotencyKey, payload, func(tx context.Context, now time.Time) (memberdomain.Member, error) {
		current, err := service.store.GetForUpdate(tx, normalized.ServiceProductID, normalized.MemberRef)
		if err != nil {
			return memberdomain.Member{}, err
		}
		if !current.Valid() || current.ServiceProductID != normalized.ServiceProductID || current.MemberRef != normalized.MemberRef {
			return memberdomain.Member{}, memberport.ErrUnavailable
		}
		if current.Version != normalized.ExpectedVersion {
			return memberdomain.Member{}, memberport.ErrConflict
		}
		if current.State == memberdomain.StateRemoved {
			return memberdomain.Member{}, memberport.ErrConflict
		}
		updated, err := service.store.UpdateFields(tx, memberport.UpdateFieldsRecord{
			ServiceProductID: normalized.ServiceProductID, MemberRef: normalized.MemberRef,
			ExpectedVersion: normalized.ExpectedVersion, Remark: cloneString(normalized.Remark),
			Alliance: cloneString(normalized.Alliance), UpdatedAt: now,
		})
		if err != nil {
			return memberdomain.Member{}, err
		}
		if !updated.Valid() || updated.Version != current.Version+1 || !sameImmutableMember(updated, current) ||
			updated.State != current.State || !optionalStringEqual(updated.Remark, normalized.Remark) ||
			!optionalStringEqual(updated.Alliance, normalized.Alliance) {
			return memberdomain.Member{}, memberport.ErrUnavailable
		}
		return service.strictReadback(tx, updated)
	})
}

func (service *Service) Get(ctx context.Context, productID int64, memberRef string) (memberdomain.Member, error) {
	if !ready(service) || ctx == nil || productID < 1 || !memberdomain.ValidMemberRef(memberRef) {
		return memberdomain.Member{}, memberport.ErrInvalidInput
	}
	var member memberdomain.Member
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		member, err = service.store.Get(tx, productID, memberRef)
		return err
	})
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	if !member.Valid() || member.ServiceProductID != productID || member.MemberRef != memberRef {
		return memberdomain.Member{}, memberport.ErrUnavailable
	}
	return cloneMember(member), nil
}

func (service *Service) List(ctx context.Context, query memberport.ListQuery) (memberport.ListResult, error) {
	if !ready(service) || ctx == nil || !validFilter(query.Filter) || query.Limit < 1 || query.Limit > memberport.MaximumLimit {
		return memberport.ListResult{}, memberport.ErrInvalidInput
	}
	var after *memberport.Position
	if query.Cursor != "" {
		position, err := service.codec.Decode(query.Cursor, query.Filter)
		if err != nil {
			return memberport.ListResult{}, err
		}
		after = &position
	}
	var rows []memberdomain.Member
	var exists bool
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		exists, err = service.store.ServiceProductExists(tx, query.ServiceProductID)
		if err != nil || !exists {
			return err
		}
		rows, err = service.store.List(tx, memberport.StoreListQuery{Filter: query.Filter, Limit: query.Limit + 1, After: clonePosition(after)})
		return err
	})
	if err != nil {
		return memberport.ListResult{}, classify(err)
	}
	if !exists {
		return memberport.ListResult{}, memberport.ErrNotFound
	}
	if len(rows) > query.Limit+1 || !validRows(rows, query.Filter, after) {
		return memberport.ListResult{}, memberport.ErrUnavailable
	}
	hasMore := len(rows) > query.Limit
	if hasMore {
		rows = rows[:query.Limit]
	}
	result := memberport.ListResult{Items: cloneMembers(rows), Limit: query.Limit, HasMore: hasMore}
	if hasMore {
		last := rows[len(rows)-1]
		result.NextCursor, err = service.codec.Encode(query.Filter, memberport.Position{UpdatedAt: last.UpdatedAt, MemberRef: last.MemberRef})
		if err != nil {
			return memberport.ListResult{}, err
		}
	}
	return result, nil
}

func (service *Service) Export(ctx context.Context, query memberport.ExportQuery) (memberport.ExportResult, error) {
	if !ready(service) || ctx == nil || !validFilter(query.Filter) || !validExportColumns(query.Columns) {
		return memberport.ExportResult{}, memberport.ErrInvalidInput
	}
	var rows []memberdomain.Member
	var exists bool
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		exists, err = service.store.ServiceProductExists(tx, query.ServiceProductID)
		if err != nil || !exists {
			return err
		}
		rows, err = service.store.List(tx, memberport.StoreListQuery{Filter: query.Filter, Limit: memberport.MaximumExportRows + 1})
		return err
	})
	if err != nil {
		return memberport.ExportResult{}, classify(err)
	}
	if !exists {
		return memberport.ExportResult{}, memberport.ErrNotFound
	}
	if len(rows) > memberport.MaximumExportRows {
		return memberport.ExportResult{}, memberport.ErrExportTooLarge
	}
	if !validRows(rows, query.Filter, nil) {
		return memberport.ExportResult{}, memberport.ErrUnavailable
	}
	body, err := encodeCSV(query.Columns, rows)
	if err != nil {
		return memberport.ExportResult{}, memberport.ErrUnavailable
	}
	return memberport.ExportResult{Filename: "service-period-members.csv", ContentType: "text/csv; charset=utf-8", Body: body}, nil
}

func (service *Service) mutate(ctx context.Context, operation string, actorID int64, key string, payload []byte, mutation func(context.Context, time.Time) (memberdomain.Member, error)) (memberdomain.Member, error) {
	if !ready(service) || ctx == nil || actorID < 1 || !validIdempotencyKey(key) || len(payload) == 0 || !json.Valid(payload) || mutation == nil {
		return memberdomain.Member{}, memberport.ErrInvalidInput
	}
	now := service.now().UTC()
	if now.IsZero() {
		return memberdomain.Member{}, memberport.ErrUnavailable
	}
	reservation := memberport.ReceiptReservation{Operation: operation, ActorScope: fmt.Sprintf("service_period_members:actor:%d", actorID), KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now}
	var result memberdomain.Member
	err := service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, err := service.store.ReserveReceipt(tx, reservation)
		if err != nil {
			return err
		}
		if !validReceipt(receipt, reservation) {
			return memberport.ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return memberport.ErrConflict
		}
		if !owned {
			result, err = decodeSnapshot(receipt.ResultSnapshot)
			return err
		}
		result, err = mutation(tx, now)
		if err != nil {
			return err
		}
		if err = service.appendEvent(tx, operation, actorID, reservation.KeyDigest, result, now); err != nil {
			return err
		}
		snapshot, err := json.Marshal(result)
		if err != nil {
			return memberport.ErrUnavailable
		}
		completed, err := service.store.CompleteReceipt(tx, receipt.ID, snapshot, now)
		if err != nil || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return memberport.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	return cloneMember(result), nil
}

func (service *Service) requireFacts(ctx context.Context, productID, customerID int64) error {
	productExists, err := service.store.LockServiceProductForMemberAdd(ctx, productID)
	if err != nil {
		return err
	}
	if !productExists {
		return memberport.ErrNotFound
	}
	customerExists, err := service.store.CustomerExists(ctx, customerID)
	if err != nil {
		return err
	}
	if !customerExists {
		return memberport.ErrNotFound
	}
	return nil
}

func (service *Service) strictReadback(ctx context.Context, expected memberdomain.Member) (memberdomain.Member, error) {
	actual, err := service.store.Get(ctx, expected.ServiceProductID, expected.MemberRef)
	if err != nil {
		return memberdomain.Member{}, err
	}
	if !actual.Valid() || !reflect.DeepEqual(cloneMember(actual), cloneMember(expected)) {
		return memberdomain.Member{}, memberport.ErrUnavailable
	}
	return cloneMember(actual), nil
}

func (service *Service) newMemberRef() (string, error) {
	raw := make([]byte, 16)
	if service.random == nil {
		return "", memberport.ErrUnavailable
	}
	if _, err := io.ReadFull(service.random, raw); err != nil {
		return "", memberport.ErrUnavailable
	}
	value := "spm_" + base64.RawURLEncoding.EncodeToString(raw)
	if !memberdomain.ValidMemberRef(value) {
		return "", memberport.ErrUnavailable
	}
	return value, nil
}

func (service *Service) appendEvent(ctx context.Context, operation string, actorID int64, keyDigest [32]byte, member memberdomain.Member, now time.Time) error {
	payload, err := json.Marshal(struct {
		Kind             string              `json:"kind"`
		Action           string              `json:"action"`
		ServiceProductID int64               `json:"service_product_id"`
		MemberRef        string              `json:"member_ref"`
		State            memberdomain.State  `json:"state"`
		Source           memberdomain.Source `json:"source"`
		Version          int64               `json:"version"`
		ActorID          int64               `json:"actor_id"`
	}{"service_period_member", strings.TrimPrefix(operation, "service_period_member."), member.ServiceProductID, member.MemberRef, member.State, member.Source, member.Version, actorID})
	if err != nil {
		return memberport.ErrUnavailable
	}
	digest := sha256.Sum256([]byte(operation + "\x00" + hex.EncodeToString(keyDigest[:])))
	_, err = service.events.Append(ctx, eventport.Event{Type: eventTypeMemberChanged, CustomerID: eventport.CustomerID(member.CustomerID), Payload: payload, OccurredAt: now, IdempotencyKey: operation + ":" + hex.EncodeToString(digest[:])})
	return err
}

func normalizeAdd(command memberport.AddCommand) (memberport.AddCommand, []byte, error) {
	if command.ServiceProductID < 1 || command.CustomerID < 1 || !command.Source.Valid() || command.ActorID < 1 || !validIdempotencyKey(command.IdempotencyKey) ||
		!memberdomain.ValidOptionalText(command.Remark, memberdomain.MaximumRemarkRunes) || !memberdomain.ValidOptionalText(command.Alliance, memberdomain.MaximumAllianceRunes) ||
		(command.ExpiresAt != nil && command.ExpiresAt.IsZero()) {
		return memberport.AddCommand{}, nil, memberport.ErrInvalidInput
	}
	command.ExpiresAt, command.Remark, command.Alliance = cloneTime(command.ExpiresAt), cloneString(command.Remark), cloneString(command.Alliance)
	payload, err := json.Marshal(commandPayload{ProductID: command.ServiceProductID, CustomerID: command.CustomerID, Source: command.Source, ExpiresAt: command.ExpiresAt, Remark: command.Remark, Alliance: command.Alliance})
	if err != nil {
		return memberport.AddCommand{}, nil, memberport.ErrInvalidInput
	}
	return command, payload, nil
}

func normalizeTransition(command memberport.TransitionCommand) (memberport.TransitionCommand, []byte, error) {
	if command.ServiceProductID < 1 || !memberdomain.ValidMemberRef(command.MemberRef) || command.ExpectedVersion < 1 || command.ActorID < 1 || !validIdempotencyKey(command.IdempotencyKey) {
		return memberport.TransitionCommand{}, nil, memberport.ErrInvalidInput
	}
	payload, err := json.Marshal(commandPayload{ProductID: command.ServiceProductID, MemberRef: command.MemberRef, ExpectedVersion: command.ExpectedVersion})
	if err != nil {
		return memberport.TransitionCommand{}, nil, memberport.ErrInvalidInput
	}
	return command, payload, nil
}

func normalizeUpdateFields(command memberport.UpdateFieldsCommand) (memberport.UpdateFieldsCommand, []byte, error) {
	if command.ServiceProductID < 1 || !memberdomain.ValidMemberRef(command.MemberRef) || command.ExpectedVersion < 1 || command.ActorID < 1 || !validIdempotencyKey(command.IdempotencyKey) ||
		!memberdomain.ValidOptionalText(command.Remark, memberdomain.MaximumRemarkRunes) || !memberdomain.ValidOptionalText(command.Alliance, memberdomain.MaximumAllianceRunes) {
		return memberport.UpdateFieldsCommand{}, nil, memberport.ErrInvalidInput
	}
	command.Remark, command.Alliance = cloneString(command.Remark), cloneString(command.Alliance)
	payload, err := json.Marshal(commandPayload{ProductID: command.ServiceProductID, MemberRef: command.MemberRef, ExpectedVersion: command.ExpectedVersion, Remark: command.Remark, Alliance: command.Alliance})
	if err != nil {
		return memberport.UpdateFieldsCommand{}, nil, memberport.ErrInvalidInput
	}
	return command, payload, nil
}

type commandPayload struct {
	ProductID       int64               `json:"service_product_id"`
	CustomerID      int64               `json:"customer_id,omitempty"`
	MemberRef       string              `json:"member_ref,omitempty"`
	ExpectedVersion int64               `json:"expected_version,omitempty"`
	Source          memberdomain.Source `json:"source,omitempty"`
	ExpiresAt       *time.Time          `json:"expires_at,omitempty"`
	Remark          *string             `json:"remark,omitempty"`
	Alliance        *string             `json:"alliance,omitempty"`
}

func validFilter(filter memberport.Filter) bool {
	return filter.ServiceProductID > 0 && (filter.State == nil || filter.State.Valid()) && (filter.Source == nil || filter.Source.Valid())
}

func validRows(rows []memberdomain.Member, filter memberport.Filter, after *memberport.Position) bool {
	seen := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		if !row.Valid() || row.ServiceProductID != filter.ServiceProductID || filter.State != nil && row.State != *filter.State || filter.Source != nil && row.Source != *filter.Source {
			return false
		}
		if _, duplicate := seen[row.MemberRef]; duplicate {
			return false
		}
		seen[row.MemberRef] = struct{}{}
		if index == 0 && after != nil && !before(row, after.UpdatedAt, after.MemberRef) {
			return false
		}
		if index > 0 && !before(row, rows[index-1].UpdatedAt, rows[index-1].MemberRef) {
			return false
		}
	}
	return true
}

func before(row memberdomain.Member, updatedAt time.Time, memberRef string) bool {
	return row.UpdatedAt.Before(updatedAt) || row.UpdatedAt.Equal(updatedAt) && row.MemberRef < memberRef
}

func validReceipt(receipt memberport.Receipt, reservation memberport.ReceiptReservation) bool {
	return receipt.ID > 0 && receipt.Operation == reservation.Operation && receipt.ActorScope == reservation.ActorScope && subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 &&
		(receipt.State == "reserved" && len(receipt.ResultSnapshot) == 0 || receipt.State == "completed" && len(receipt.ResultSnapshot) > 0 && json.Valid(receipt.ResultSnapshot))
}

func decodeSnapshot(raw []byte) (memberdomain.Member, error) {
	var member memberdomain.Member
	if json.Unmarshal(raw, &member) != nil || !member.Valid() {
		return memberdomain.Member{}, memberport.ErrUnavailable
	}
	canonical, err := json.Marshal(member)
	if err != nil || !jsonEquivalent(canonical, raw) {
		return memberdomain.Member{}, memberport.ErrUnavailable
	}
	return member, nil
}

func validExportColumns(columns []memberport.ExportColumn) bool {
	if len(columns) == 0 || len(columns) > 9 {
		return false
	}
	allowed := map[memberport.ExportColumn]struct{}{memberport.ExportMemberRef: {}, memberport.ExportCustomerID: {}, memberport.ExportState: {}, memberport.ExportSource: {}, memberport.ExportStartsAt: {}, memberport.ExportExpiresAt: {}, memberport.ExportExpiredAt: {}, memberport.ExportRemovedAt: {}, memberport.ExportVersion: {}}
	seen := make(map[memberport.ExportColumn]struct{}, len(columns))
	for _, column := range columns {
		if _, ok := allowed[column]; !ok {
			return false
		}
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}

func encodeCSV(columns []memberport.ExportColumn, rows []memberdomain.Member) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("\xef\xbb\xbf")
	writer := csv.NewWriter(&buffer)
	header := make([]string, len(columns))
	for index, column := range columns {
		header[index] = string(column)
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, member := range rows {
		record := make([]string, len(columns))
		for index, column := range columns {
			record[index] = safeCSVCell(exportValue(column, member))
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func exportValue(column memberport.ExportColumn, member memberdomain.Member) string {
	switch column {
	case memberport.ExportMemberRef:
		return member.MemberRef
	case memberport.ExportCustomerID:
		return strconv.FormatInt(member.CustomerID, 10)
	case memberport.ExportState:
		return string(member.State)
	case memberport.ExportSource:
		return string(member.Source)
	case memberport.ExportStartsAt:
		return member.StartsAt.UTC().Format(time.RFC3339Nano)
	case memberport.ExportExpiresAt:
		return formatTime(member.ExpiresAt)
	case memberport.ExportExpiredAt:
		return formatTime(member.ExpiredAt)
	case memberport.ExportRemovedAt:
		return formatTime(member.RemovedAt)
	case memberport.ExportVersion:
		return strconv.FormatInt(member.Version, 10)
	default:
		return ""
	}
}

func safeCSVCell(value string) string {
	if value == "" {
		return value
	}
	first, _ := utf8.DecodeRuneInString(value)
	if strings.ContainsRune("=+-@\t\r", first) {
		return "'" + value
	}
	return value
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
func validIdempotencyKey(value string) bool {
	return len(value) >= 16 && len(value) <= 128 && strings.TrimSpace(value) == value && utf8.ValidString(value)
}
func ready(service *Service) bool {
	return service != nil && !nilDependency(service.uow) && !nilDependency(service.store) && !nilDependency(service.events) && service.codec != nil && service.now != nil && service.random != nil
}
func classify(err error) error {
	if errors.Is(err, memberport.ErrInvalidInput) || errors.Is(err, memberport.ErrNotFound) || errors.Is(err, memberport.ErrConflict) || errors.Is(err, memberport.ErrPaidOrderSourceBlocked) || errors.Is(err, memberport.ErrExportTooLarge) {
		return err
	}
	return errors.Join(memberport.ErrUnavailable, err)
}
func sameImmutableMember(left, right memberdomain.Member) bool {
	return left.MemberRef == right.MemberRef && left.ServiceProductID == right.ServiceProductID && left.CustomerID == right.CustomerID && left.Source == right.Source && left.StartsAt.Equal(right.StartsAt) && optionalTimeEqual(left.ExpiresAt, right.ExpiresAt) && left.CreatedAt.Equal(right.CreatedAt)
}
func cloneMember(value memberdomain.Member) memberdomain.Member {
	value.ExpiresAt, value.ExpiredAt, value.RemovedAt, value.Remark, value.Alliance = cloneTime(value.ExpiresAt), cloneTime(value.ExpiredAt), cloneTime(value.RemovedAt), cloneString(value.Remark), cloneString(value.Alliance)
	return value
}
func cloneMembers(values []memberdomain.Member) []memberdomain.Member {
	result := make([]memberdomain.Member, len(values))
	for index := range values {
		result[index] = cloneMember(values[index])
	}
	return result
}
func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
func clonePosition(value *memberport.Position) *memberport.Position {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.UpdatedAt = cloned.UpdatedAt.UTC()
	return &cloned
}
func optionalTimeEqual(left, right *time.Time) bool {
	return left == nil && right == nil || left != nil && right != nil && left.Equal(*right)
}
func optionalStringEqual(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
func jsonEquivalent(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
