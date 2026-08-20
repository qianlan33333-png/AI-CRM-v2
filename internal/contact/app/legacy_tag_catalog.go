package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const LegacyTagLimit = 1000

var (
	ErrInvalidLegacyTag     = errors.New("invalid legacy tag catalog command")
	ErrLegacyTagNotFound    = errors.New("legacy tag catalog item not found")
	ErrLegacyTagReferenced  = errors.New("legacy tag catalog item is still referenced by customers")
	ErrLegacyTagConflict    = errors.New("legacy tag catalog idempotency command conflict")
	ErrLegacyTagUnavailable = errors.New("legacy tag catalog unavailable")
)

type LegacyTagGroup struct {
	ID        int64  `json:"group_id"`
	Name      string `json:"group_name"`
	SortOrder int32  `json:"sort_order"`
}
type LegacyTag struct {
	ID        int64  `json:"tag_id"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Name      string `json:"tag_name"`
	SortOrder int32  `json:"sort_order"`
}
type LegacyTagCatalog struct {
	Groups   []LegacyTagGroup `json:"groups"`
	Tags     []LegacyTag      `json:"tags"`
	SyncedAt time.Time        `json:"synced_at"`
}
type LegacyTagCommand struct {
	Actor                                                     int64
	IdempotencyKey, TraceID, GroupName, FirstTagName, TagName string
	GroupID, TagID                                            int64
	IDs                                                       []int64
}

type LocalTagReceiptState string

const (
	localTagReceiptInProgress LocalTagReceiptState = "in_progress"
	localTagReceiptCompleted  LocalTagReceiptState = "completed"
)

type LocalTagReceipt struct {
	ID             int64
	Operation      string
	Actor          int64
	IdempotencyKey string
	PayloadDigest  []byte
	State          LocalTagReceiptState
	ResultIDs      []int64
}

type LocalTagReceiptReservation struct {
	Operation      string
	Actor          int64
	IdempotencyKey string
	PayloadDigest  []byte
}

type LegacyTagCatalogStore interface {
	ListLegacyTagGroups(context.Context) ([]LegacyTagGroup, error)
	ListLegacyTags(context.Context) ([]LegacyTag, error)
	CreateLegacyTagGroup(context.Context, string) (LegacyTagGroup, error)
	CreateLegacyTag(context.Context, int64, string) (LegacyTag, error)
	UpdateLegacyTagGroup(context.Context, int64, string) (LegacyTagGroup, error)
	ArchiveLegacyTagGroup(context.Context, int64) (LegacyTagGroup, error)
	UpdateLegacyTag(context.Context, int64, string) (LegacyTag, error)
	ArchiveLegacyTag(context.Context, int64) (LegacyTag, error)
	GetLegacyTagGroup(context.Context, int64) (LegacyTagGroup, error)
	GetLegacyTag(context.Context, int64) (LegacyTag, error)
	ReorderLegacyTagGroups(context.Context, []int64) ([]LegacyTagGroup, error)
	ReorderLegacyTags(context.Context, []int64) ([]LegacyTag, error)
	CountLegacyTagReferences(context.Context, int64) (int64, error)
	CountLegacyTagGroupReferences(context.Context, int64) (int64, error)
	ReserveLocalTagReceipt(context.Context, LocalTagReceiptReservation) (LocalTagReceipt, bool, error)
	CompleteLocalTagReceipt(context.Context, int64, []int64, time.Time) (LocalTagReceipt, error)
}

type LegacyTagCatalogService struct {
	uow    platformport.UnitOfWork
	store  LegacyTagCatalogStore
	events eventport.Appender
	now    func() time.Time
}

func NewLegacyTagCatalogService(uow platformport.UnitOfWork, store LegacyTagCatalogStore, events eventport.Appender) *LegacyTagCatalogService {
	return &LegacyTagCatalogService{uow: uow, store: store, events: events, now: time.Now}
}

func (s *LegacyTagCatalogService) List(ctx context.Context) (LegacyTagCatalog, error) {
	if !legacyTagsReady(s) || ctx == nil {
		return LegacyTagCatalog{}, ErrLegacyTagUnavailable
	}
	var result LegacyTagCatalog
	err := s.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result.Groups, err = s.store.ListLegacyTagGroups(tx)
		if err != nil {
			return err
		}
		result.Tags, err = s.store.ListLegacyTags(tx)
		return err
	})
	if err != nil || result.Groups == nil || result.Tags == nil {
		return LegacyTagCatalog{}, errors.Join(ErrLegacyTagUnavailable, err)
	}
	result.SyncedAt = s.now().UTC()
	return result, nil
}

// GetGroup projects one active local tag group from the same bounded catalog
// snapshot used by the legacy management page. The HTTP route remains a
// shared-composition concern; this method deliberately performs no provider
// read or write.
func (s *LegacyTagCatalogService) GetGroup(ctx context.Context, groupID int64) (LegacyTagGroup, error) {
	if groupID < 1 {
		return LegacyTagGroup{}, ErrLegacyTagNotFound
	}
	catalog, err := s.List(ctx)
	if err != nil {
		return LegacyTagGroup{}, err
	}
	for _, group := range catalog.Groups {
		if group.ID == groupID {
			return group, nil
		}
	}
	return LegacyTagGroup{}, ErrLegacyTagNotFound
}

// GetTag projects one active local tag from the same bounded catalog snapshot
// as List. WeCom provider identifiers stay outside this Contact-owned DTO.
func (s *LegacyTagCatalogService) GetTag(ctx context.Context, tagID int64) (LegacyTag, error) {
	if tagID < 1 {
		return LegacyTag{}, ErrLegacyTagNotFound
	}
	catalog, err := s.List(ctx)
	if err != nil {
		return LegacyTag{}, err
	}
	for _, tag := range catalog.Tags {
		if tag.ID == tagID {
			return tag, nil
		}
	}
	return LegacyTag{}, ErrLegacyTagNotFound
}

func (s *LegacyTagCatalogService) CreateGroup(ctx context.Context, c LegacyTagCommand) (LegacyTagGroup, LegacyTag, error) {
	if err := validLegacyCommand(c, c.GroupName, c.FirstTagName); err != nil {
		return LegacyTagGroup{}, LegacyTag{}, err
	}
	result, err := s.mutate(ctx, "group_create", c, func(tx context.Context) (legacyTagMutationResult, error) {
		var group LegacyTagGroup
		var tag LegacyTag
		var err error
		group, err = s.store.CreateLegacyTagGroup(tx, strings.TrimSpace(c.GroupName))
		if err != nil {
			return legacyTagMutationResult{}, err
		}
		tag, err = s.store.CreateLegacyTag(tx, group.ID, strings.TrimSpace(c.FirstTagName))
		if err != nil {
			return legacyTagMutationResult{}, err
		}
		return legacyTagMutationResult{value: legacyTagGroupCreateResult{group: group, tag: tag}, resultIDs: []int64{group.ID, tag.ID}}, nil
	}, func(tx context.Context, ids []int64) (legacyTagMutationResult, error) {
		if len(ids) != 2 {
			return legacyTagMutationResult{}, ErrLegacyTagConflict
		}
		group, err := s.store.GetLegacyTagGroup(tx, ids[0])
		if err != nil {
			return legacyTagMutationResult{}, err
		}
		tag, err := s.store.GetLegacyTag(tx, ids[1])
		if err != nil || tag.GroupID != group.ID {
			return legacyTagMutationResult{}, errors.Join(ErrLegacyTagConflict, err)
		}
		return legacyTagMutationResult{value: legacyTagGroupCreateResult{group: group, tag: tag}, resultIDs: ids}, nil
	})
	if err != nil {
		return LegacyTagGroup{}, LegacyTag{}, err
	}
	pair, ok := result.value.(legacyTagGroupCreateResult)
	if !ok {
		return LegacyTagGroup{}, LegacyTag{}, ErrLegacyTagUnavailable
	}
	return pair.group, pair.tag, nil
}
func (s *LegacyTagCatalogService) UpdateGroup(ctx context.Context, c LegacyTagCommand) (LegacyTagGroup, error) {
	if c.GroupID < 1 {
		return LegacyTagGroup{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c, c.GroupName); err != nil {
		return LegacyTagGroup{}, err
	}
	result, err := s.mutate(ctx, "group_update", c, func(tx context.Context) (legacyTagMutationResult, error) {
		var e error
		out, e := s.store.UpdateLegacyTagGroup(tx, c.GroupID, strings.TrimSpace(c.GroupName))
		return legacyTagMutationResult{value: out, resultIDs: []int64{out.ID}}, e
	}, func(tx context.Context, ids []int64) (legacyTagMutationResult, error) {
		if len(ids) != 1 || ids[0] != c.GroupID {
			return legacyTagMutationResult{}, ErrLegacyTagConflict
		}
		out, err := s.store.GetLegacyTagGroup(tx, c.GroupID)
		return legacyTagMutationResult{value: out, resultIDs: ids}, err
	})
	if err != nil {
		return LegacyTagGroup{}, err
	}
	out, ok := result.value.(LegacyTagGroup)
	if !ok {
		return LegacyTagGroup{}, ErrLegacyTagUnavailable
	}
	return out, nil
}
func (s *LegacyTagCatalogService) ArchiveGroup(ctx context.Context, c LegacyTagCommand) (LegacyTagGroup, error) {
	if c.GroupID < 1 {
		return LegacyTagGroup{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c); err != nil {
		return LegacyTagGroup{}, err
	}
	result, err := s.mutate(ctx, "group_archive", c, func(tx context.Context) (legacyTagMutationResult, error) {
		references, countErr := s.store.CountLegacyTagGroupReferences(tx, c.GroupID)
		if countErr != nil {
			return legacyTagMutationResult{}, countErr
		}
		if references > 0 {
			return legacyTagMutationResult{}, ErrLegacyTagReferenced
		}
		var e error
		out, e := s.store.ArchiveLegacyTagGroup(tx, c.GroupID)
		return legacyTagMutationResult{value: out, resultIDs: []int64{out.ID}}, e
	}, func(tx context.Context, ids []int64) (legacyTagMutationResult, error) {
		if len(ids) != 1 || ids[0] != c.GroupID {
			return legacyTagMutationResult{}, ErrLegacyTagConflict
		}
		out, err := s.store.GetLegacyTagGroup(tx, c.GroupID)
		return legacyTagMutationResult{value: out, resultIDs: ids}, err
	})
	if err != nil {
		return LegacyTagGroup{}, err
	}
	out, ok := result.value.(LegacyTagGroup)
	if !ok {
		return LegacyTagGroup{}, ErrLegacyTagUnavailable
	}
	return out, nil
}
func (s *LegacyTagCatalogService) CreateTag(ctx context.Context, c LegacyTagCommand) (LegacyTag, error) {
	if c.GroupID < 1 {
		return LegacyTag{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c, c.GroupName, c.TagName); err != nil {
		return LegacyTag{}, err
	}
	result, err := s.mutate(ctx, "tag_create", c, func(tx context.Context) (legacyTagMutationResult, error) {
		var e error
		out, e := s.store.CreateLegacyTag(tx, c.GroupID, strings.TrimSpace(c.TagName))
		return legacyTagMutationResult{value: out, resultIDs: []int64{out.ID}}, e
	}, func(tx context.Context, ids []int64) (legacyTagMutationResult, error) {
		if len(ids) != 1 {
			return legacyTagMutationResult{}, ErrLegacyTagConflict
		}
		out, err := s.store.GetLegacyTag(tx, ids[0])
		return legacyTagMutationResult{value: out, resultIDs: ids}, err
	})
	if err != nil {
		return LegacyTag{}, err
	}
	out, ok := result.value.(LegacyTag)
	if !ok {
		return LegacyTag{}, ErrLegacyTagUnavailable
	}
	return out, nil
}
func (s *LegacyTagCatalogService) UpdateTag(ctx context.Context, c LegacyTagCommand) (LegacyTag, error) {
	if c.TagID < 1 {
		return LegacyTag{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c, c.TagName); err != nil {
		return LegacyTag{}, err
	}
	result, err := s.mutate(ctx, "tag_update", c, func(tx context.Context) (legacyTagMutationResult, error) {
		var e error
		out, e := s.store.UpdateLegacyTag(tx, c.TagID, strings.TrimSpace(c.TagName))
		return legacyTagMutationResult{value: out, resultIDs: []int64{out.ID}}, e
	}, func(tx context.Context, ids []int64) (legacyTagMutationResult, error) {
		if len(ids) != 1 || ids[0] != c.TagID {
			return legacyTagMutationResult{}, ErrLegacyTagConflict
		}
		out, err := s.store.GetLegacyTag(tx, c.TagID)
		return legacyTagMutationResult{value: out, resultIDs: ids}, err
	})
	if err != nil {
		return LegacyTag{}, err
	}
	out, ok := result.value.(LegacyTag)
	if !ok {
		return LegacyTag{}, ErrLegacyTagUnavailable
	}
	return out, nil
}
func (s *LegacyTagCatalogService) ArchiveTag(ctx context.Context, c LegacyTagCommand) (LegacyTag, error) {
	if c.TagID < 1 {
		return LegacyTag{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c); err != nil {
		return LegacyTag{}, err
	}
	result, err := s.mutate(ctx, "tag_archive", c, func(tx context.Context) (legacyTagMutationResult, error) {
		references, countErr := s.store.CountLegacyTagReferences(tx, c.TagID)
		if countErr != nil {
			return legacyTagMutationResult{}, countErr
		}
		if references > 0 {
			return legacyTagMutationResult{}, ErrLegacyTagReferenced
		}
		var e error
		out, e := s.store.ArchiveLegacyTag(tx, c.TagID)
		return legacyTagMutationResult{value: out, resultIDs: []int64{out.ID}}, e
	}, func(tx context.Context, ids []int64) (legacyTagMutationResult, error) {
		if len(ids) != 1 || ids[0] != c.TagID {
			return legacyTagMutationResult{}, ErrLegacyTagConflict
		}
		out, err := s.store.GetLegacyTag(tx, c.TagID)
		return legacyTagMutationResult{value: out, resultIDs: ids}, err
	})
	if err != nil {
		return LegacyTag{}, err
	}
	out, ok := result.value.(LegacyTag)
	if !ok {
		return LegacyTag{}, ErrLegacyTagUnavailable
	}
	return out, nil
}

func (s *LegacyTagCatalogService) ReorderGroups(ctx context.Context, c LegacyTagCommand) ([]LegacyTagGroup, error) {
	if err := validLegacyCommand(c); err != nil || !validLegacyIDs(c.IDs) {
		return nil, ErrInvalidLegacyTag
	}
	result, err := s.mutate(ctx, "group_reorder", c, func(tx context.Context) (legacyTagMutationResult, error) {
		current, err := s.store.ListLegacyTagGroups(tx)
		if err != nil || !sameLegacyIDs(groupIDs(current), c.IDs) {
			return legacyTagMutationResult{}, errors.Join(ErrLegacyTagConflict, err)
		}
		out, err := s.store.ReorderLegacyTagGroups(tx, c.IDs)
		return legacyTagMutationResult{value: out, resultIDs: groupIDs(out)}, err
	}, func(tx context.Context, ids []int64) (legacyTagMutationResult, error) {
		out, err := s.store.ListLegacyTagGroups(tx)
		if err != nil || !sameLegacyIDs(groupIDs(out), ids) {
			return legacyTagMutationResult{}, errors.Join(ErrLegacyTagConflict, err)
		}
		return legacyTagMutationResult{value: out, resultIDs: ids}, nil
	})
	if err != nil {
		return nil, err
	}
	out, ok := result.value.([]LegacyTagGroup)
	if !ok {
		return nil, ErrLegacyTagUnavailable
	}
	return out, nil
}

func (s *LegacyTagCatalogService) ReorderTags(ctx context.Context, c LegacyTagCommand) ([]LegacyTag, error) {
	if err := validLegacyCommand(c); err != nil || !validLegacyIDs(c.IDs) {
		return nil, ErrInvalidLegacyTag
	}
	result, err := s.mutate(ctx, "tag_reorder", c, func(tx context.Context) (legacyTagMutationResult, error) {
		current, err := s.store.ListLegacyTags(tx)
		if err != nil || !sameLegacyIDs(tagIDs(current), c.IDs) {
			return legacyTagMutationResult{}, errors.Join(ErrLegacyTagConflict, err)
		}
		out, err := s.store.ReorderLegacyTags(tx, c.IDs)
		return legacyTagMutationResult{value: out, resultIDs: tagIDs(out)}, err
	}, func(tx context.Context, ids []int64) (legacyTagMutationResult, error) {
		out, err := s.store.ListLegacyTags(tx)
		if err != nil || !sameLegacyIDs(tagIDs(out), ids) {
			return legacyTagMutationResult{}, errors.Join(ErrLegacyTagConflict, err)
		}
		return legacyTagMutationResult{value: out, resultIDs: ids}, nil
	})
	if err != nil {
		return nil, err
	}
	out, ok := result.value.([]LegacyTag)
	if !ok {
		return nil, ErrLegacyTagUnavailable
	}
	return out, nil
}

type legacyTagMutationResult struct {
	value     any
	resultIDs []int64
}
type legacyTagGroupCreateResult struct {
	group LegacyTagGroup
	tag   LegacyTag
}

func (s *LegacyTagCatalogService) mutate(
	ctx context.Context,
	operation string,
	c LegacyTagCommand,
	apply func(context.Context) (legacyTagMutationResult, error),
	replay func(context.Context, []int64) (legacyTagMutationResult, error),
) (legacyTagMutationResult, error) {
	if !legacyTagsReady(s) || ctx == nil {
		return legacyTagMutationResult{}, ErrLegacyTagUnavailable
	}
	now := s.now().UTC()
	if now.IsZero() {
		return legacyTagMutationResult{}, ErrLegacyTagUnavailable
	}
	payloadDigest := legacyTagPayloadDigest(operation, c)
	var result legacyTagMutationResult
	err := s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, err := s.store.ReserveLocalTagReceipt(tx, LocalTagReceiptReservation{Operation: operation, Actor: c.Actor, IdempotencyKey: c.IdempotencyKey, PayloadDigest: payloadDigest})
		if err != nil {
			return err
		}
		if receipt.ID <= 0 || receipt.Operation != operation || receipt.Actor != c.Actor || !equalLegacyDigest(receipt.PayloadDigest, payloadDigest) {
			return ErrLegacyTagConflict
		}
		if !owned {
			if receipt.State != localTagReceiptCompleted || len(receipt.ResultIDs) == 0 {
				return ErrLegacyTagConflict
			}
			result, err = replay(tx, receipt.ResultIDs)
			if err != nil || !sameLegacyIDs(result.resultIDs, receipt.ResultIDs) {
				return errors.Join(ErrLegacyTagConflict, err)
			}
			return nil
		}
		if receipt.State != localTagReceiptInProgress {
			return ErrLegacyTagConflict
		}
		result, err = apply(tx)
		if err != nil {
			return err
		}
		if !validLegacyResultIDs(result.resultIDs) {
			return ErrLegacyTagUnavailable
		}
		payload, err := json.Marshal(map[string]any{"actor": c.Actor, "operation": operation, "result": result.value, "trace_id": strings.TrimSpace(c.TraceID)})
		if err != nil {
			return err
		}
		_, err = s.events.Append(tx, eventport.Event{Type: "contact.tag_catalog_" + operation, Payload: payload, OccurredAt: now, IdempotencyKey: "tag-catalog:" + hex.EncodeToString(payloadDigest)})
		if err != nil {
			return err
		}
		completed, err := s.store.CompleteLocalTagReceipt(tx, receipt.ID, result.resultIDs, now)
		if err != nil || completed.ID != receipt.ID || completed.State != localTagReceiptCompleted || !sameLegacyIDs(completed.ResultIDs, result.resultIDs) {
			return errors.Join(ErrLegacyTagUnavailable, err)
		}
		return nil
	})
	if err != nil {
		return legacyTagMutationResult{}, classifyLegacyTagError(err)
	}
	return result, nil
}

func validLegacyCommand(c LegacyTagCommand, values ...string) error {
	if c.Actor < 1 || len(c.IdempotencyKey) < 16 || len(c.IdempotencyKey) > 128 || strings.TrimSpace(c.IdempotencyKey) != c.IdempotencyKey {
		return ErrInvalidLegacyTag
	}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || len([]rune(v)) > 200 {
			return ErrInvalidLegacyTag
		}
	}
	return nil
}
func classifyLegacyTagError(err error) error {
	if errors.Is(err, ErrInvalidLegacyTag) || errors.Is(err, ErrLegacyTagNotFound) || errors.Is(err, ErrLegacyTagReferenced) || errors.Is(err, ErrLegacyTagConflict) {
		return err
	}
	return errors.Join(ErrLegacyTagUnavailable, err)
}

func legacyTagPayloadDigest(operation string, command LegacyTagCommand) []byte {
	payload, _ := json.Marshal(struct {
		Operation string  `json:"operation"`
		Actor     int64   `json:"actor"`
		GroupID   int64   `json:"group_id"`
		TagID     int64   `json:"tag_id"`
		GroupName string  `json:"group_name"`
		FirstTag  string  `json:"first_tag_name"`
		TagName   string  `json:"tag_name"`
		IDs       []int64 `json:"ids"`
	}{operation, command.Actor, command.GroupID, command.TagID, strings.TrimSpace(command.GroupName), strings.TrimSpace(command.FirstTagName), strings.TrimSpace(command.TagName), command.IDs})
	digest := sha256.Sum256(payload)
	return digest[:]
}

func equalLegacyDigest(left, right []byte) bool {
	return len(left) == sha256.Size && len(right) == sha256.Size && string(left) == string(right)
}

func validLegacyIDs(ids []int64) bool {
	if len(ids) == 0 {
		return false
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return false
		}
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func validLegacyResultIDs(ids []int64) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if id <= 0 {
			return false
		}
	}
	return true
}

func sameLegacyIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func groupIDs(groups []LegacyTagGroup) []int64 {
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}

func tagIDs(tags []LegacyTag) []int64 {
	ids := make([]int64, 0, len(tags))
	for _, tag := range tags {
		ids = append(ids, tag.ID)
	}
	return ids
}
func legacyTagsReady(s *LegacyTagCatalogService) bool {
	return s != nil && !nilLegacyTagDependency(s.uow) && !nilLegacyTagDependency(s.store) && !nilLegacyTagDependency(s.events) && s.now != nil
}
func nilLegacyTagDependency(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}
