package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

func (s *LegacyTagCatalogService) CreateGroup(ctx context.Context, c LegacyTagCommand) (LegacyTagGroup, LegacyTag, error) {
	if err := validLegacyCommand(c, c.GroupName, c.FirstTagName); err != nil {
		return LegacyTagGroup{}, LegacyTag{}, err
	}
	var group LegacyTagGroup
	var tag LegacyTag
	err := s.mutate(ctx, "group.create", c, func(tx context.Context) (any, error) {
		var err error
		group, err = s.store.CreateLegacyTagGroup(tx, strings.TrimSpace(c.GroupName))
		if err != nil {
			return nil, err
		}
		tag, err = s.store.CreateLegacyTag(tx, group.ID, strings.TrimSpace(c.FirstTagName))
		return map[string]any{"group": group, "tag": tag}, err
	})
	return group, tag, err
}
func (s *LegacyTagCatalogService) UpdateGroup(ctx context.Context, c LegacyTagCommand) (LegacyTagGroup, error) {
	if c.GroupID < 1 {
		return LegacyTagGroup{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c, c.GroupName); err != nil {
		return LegacyTagGroup{}, err
	}
	var out LegacyTagGroup
	err := s.mutate(ctx, "group.update", c, func(tx context.Context) (any, error) {
		var e error
		out, e = s.store.UpdateLegacyTagGroup(tx, c.GroupID, strings.TrimSpace(c.GroupName))
		return out, e
	})
	return out, err
}
func (s *LegacyTagCatalogService) ArchiveGroup(ctx context.Context, c LegacyTagCommand) (LegacyTagGroup, error) {
	if c.GroupID < 1 {
		return LegacyTagGroup{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c); err != nil {
		return LegacyTagGroup{}, err
	}
	var out LegacyTagGroup
	err := s.mutate(ctx, "group.archive", c, func(tx context.Context) (any, error) {
		var e error
		out, e = s.store.ArchiveLegacyTagGroup(tx, c.GroupID)
		return out, e
	})
	return out, err
}
func (s *LegacyTagCatalogService) CreateTag(ctx context.Context, c LegacyTagCommand) (LegacyTag, error) {
	if c.GroupID < 1 {
		return LegacyTag{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c, c.GroupName, c.TagName); err != nil {
		return LegacyTag{}, err
	}
	var out LegacyTag
	err := s.mutate(ctx, "tag.create", c, func(tx context.Context) (any, error) {
		var e error
		out, e = s.store.CreateLegacyTag(tx, c.GroupID, strings.TrimSpace(c.TagName))
		return out, e
	})
	return out, err
}
func (s *LegacyTagCatalogService) UpdateTag(ctx context.Context, c LegacyTagCommand) (LegacyTag, error) {
	if c.TagID < 1 {
		return LegacyTag{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c, c.TagName); err != nil {
		return LegacyTag{}, err
	}
	var out LegacyTag
	err := s.mutate(ctx, "tag.update", c, func(tx context.Context) (any, error) {
		var e error
		out, e = s.store.UpdateLegacyTag(tx, c.TagID, strings.TrimSpace(c.TagName))
		return out, e
	})
	return out, err
}
func (s *LegacyTagCatalogService) ArchiveTag(ctx context.Context, c LegacyTagCommand) (LegacyTag, error) {
	if c.TagID < 1 {
		return LegacyTag{}, ErrLegacyTagNotFound
	}
	if err := validLegacyCommand(c); err != nil {
		return LegacyTag{}, err
	}
	var out LegacyTag
	err := s.mutate(ctx, "tag.archive", c, func(tx context.Context) (any, error) {
		var e error
		out, e = s.store.ArchiveLegacyTag(tx, c.TagID)
		return out, e
	})
	return out, err
}

func (s *LegacyTagCatalogService) mutate(ctx context.Context, operation string, c LegacyTagCommand, apply func(context.Context) (any, error)) error {
	if !legacyTagsReady(s) || ctx == nil {
		return ErrLegacyTagUnavailable
	}
	now := s.now().UTC()
	if now.IsZero() {
		return ErrLegacyTagUnavailable
	}
	err := s.uow.Within(ctx, func(tx context.Context) error {
		result, err := apply(tx)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{"actor": c.Actor, "operation": operation, "result": result, "trace_id": strings.TrimSpace(c.TraceID)})
		if err != nil {
			return err
		}
		d := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", c.Actor, operation, c.IdempotencyKey)))
		_, err = s.events.Append(tx, eventport.Event{Type: "contact.tag_catalog_" + strings.ReplaceAll(operation, ".", "_"), Payload: payload, OccurredAt: now, IdempotencyKey: "tag-catalog:" + hex.EncodeToString(d[:])})
		return err
	})
	if err != nil {
		return classifyLegacyTagError(err)
	}
	return nil
}

func validLegacyCommand(c LegacyTagCommand, values ...string) error {
	if c.Actor < 1 || len(strings.TrimSpace(c.IdempotencyKey)) < 1 || len(c.IdempotencyKey) > 200 {
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
	if errors.Is(err, ErrInvalidLegacyTag) || errors.Is(err, ErrLegacyTagNotFound) {
		return err
	}
	return errors.Join(ErrLegacyTagUnavailable, err)
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
