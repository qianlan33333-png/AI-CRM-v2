package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type miniProgramMemory struct {
	mu       sync.Mutex
	receipts map[string]MiniProgramReceipt
	items    map[int64]mediaport.MiniProgramCard
	images   map[int64]bool
	nextID   int64
	fault    string
}

type miniProgramUOW struct{ state *miniProgramMemory }
type miniProgramStore struct{ state *miniProgramMemory }
type miniProgramResolver struct {
	result mediaport.MiniProgramThumbResolution
	err    error
	calls  int
}

func (u miniProgramUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	u.state.mu.Lock()
	defer u.state.mu.Unlock()
	receipts := make(map[string]MiniProgramReceipt, len(u.state.receipts))
	for key, value := range u.state.receipts {
		value.ResultSnapshot = append(json.RawMessage{}, value.ResultSnapshot...)
		receipts[key] = value
	}
	items := make(map[int64]mediaport.MiniProgramCard, len(u.state.items))
	for key, value := range u.state.items {
		items[key] = cloneMiniProgramCard(value)
	}
	nextID := u.state.nextID
	if err := fn(ctx); err != nil {
		u.state.receipts, u.state.items, u.state.nextID = receipts, items, nextID
		return err
	}
	return nil
}

func miniProgramReceiptKey(input MiniProgramReservation) string {
	return input.Operation + ":" + input.ActorScope + ":" + input.BusinessKey + ":" + string(input.KeyDigest[:])
}

func (store miniProgramStore) ReserveMiniProgram(_ context.Context, input MiniProgramReservation) (MiniProgramReceipt, bool, error) {
	if store.state.fault == "reserve" {
		return MiniProgramReceipt{}, false, errors.New("reserve failed")
	}
	key := miniProgramReceiptKey(input)
	if receipt, ok := store.state.receipts[key]; ok {
		return receipt, false, nil
	}
	receipt := MiniProgramReceipt{ID: int64(len(store.state.receipts) + 1), Operation: input.Operation, ActorScope: input.ActorScope,
		BusinessKey: input.BusinessKey, KeyDigest: input.KeyDigest, PayloadDigest: input.PayloadDigest, State: "in_progress"}
	store.state.receipts[key] = receipt
	return receipt, true, nil
}

func (store miniProgramStore) CompleteMiniProgram(_ context.Context, id int64, snapshot json.RawMessage, _ time.Time) (MiniProgramReceipt, error) {
	if store.state.fault == "complete" {
		return MiniProgramReceipt{}, errors.New("complete failed")
	}
	for key, receipt := range store.state.receipts {
		if receipt.ID == id {
			receipt.State, receipt.ResultSnapshot = "completed", append(json.RawMessage{}, snapshot...)
			store.state.receipts[key] = receipt
			return receipt, nil
		}
	}
	return MiniProgramReceipt{}, errors.New("receipt missing")
}

func (store miniProgramStore) ListMiniPrograms(_ context.Context, query mediaport.MiniProgramListQuery) ([]mediaport.MiniProgramCard, error) {
	items := make([]mediaport.MiniProgramCard, 0, len(store.state.items))
	needle := strings.ToLower(query.Search)
	for _, item := range store.state.items {
		if query.EnabledOnly && !item.Enabled {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Title+" "+item.AppID+" "+item.PagePath), needle) {
			continue
		}
		items = append(items, cloneMiniProgramCard(item))
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].UpdatedAt.Equal(items[right].UpdatedAt) {
			return items[left].ID > items[right].ID
		}
		return items[left].UpdatedAt.After(items[right].UpdatedAt)
	})
	start := min(int(query.Offset), len(items))
	end := min(start+int(query.Limit), len(items))
	return items[start:end], nil
}

func (store miniProgramStore) CountMiniPrograms(ctx context.Context, query mediaport.MiniProgramListQuery) (int64, error) {
	all, err := store.ListMiniPrograms(ctx, mediaport.MiniProgramListQuery{Limit: MaximumMiniProgramLimit, EnabledOnly: query.EnabledOnly, Search: query.Search})
	return int64(len(all)), err
}

func (store miniProgramStore) GetMiniProgram(_ context.Context, id int64) (mediaport.MiniProgramCard, error) {
	item, ok := store.state.items[id]
	if !ok {
		return mediaport.MiniProgramCard{}, ErrMiniProgramNotFound
	}
	return cloneMiniProgramCard(item), nil
}

func (store miniProgramStore) LockMiniProgram(ctx context.Context, id int64) (mediaport.MiniProgramCard, error) {
	if store.state.fault == "lock" {
		return mediaport.MiniProgramCard{}, errors.New("lock failed")
	}
	return store.GetMiniProgram(ctx, id)
}

func (store miniProgramStore) CreateMiniProgram(_ context.Context, item mediaport.MiniProgramCard) (mediaport.MiniProgramCard, error) {
	if store.state.fault == "fact" {
		return mediaport.MiniProgramCard{}, errors.New("create failed")
	}
	store.state.nextID++
	item.ID = store.state.nextID
	store.state.items[item.ID] = cloneMiniProgramCard(item)
	return cloneMiniProgramCard(item), nil
}

func (store miniProgramStore) UpdateMiniProgram(_ context.Context, item mediaport.MiniProgramCard) (mediaport.MiniProgramCard, error) {
	if store.state.fault == "fact" {
		return mediaport.MiniProgramCard{}, errors.New("write failed")
	}
	if _, ok := store.state.items[item.ID]; !ok {
		return mediaport.MiniProgramCard{}, ErrMiniProgramNotFound
	}
	store.state.items[item.ID] = cloneMiniProgramCard(item)
	return cloneMiniProgramCard(item), nil
}

func (store miniProgramStore) HardDeleteMiniProgram(_ context.Context, id int64) (mediaport.MiniProgramDeleteResult, error) {
	if store.state.fault == "fact" {
		return mediaport.MiniProgramDeleteResult{}, errors.New("removal failed")
	}
	if _, ok := store.state.items[id]; !ok {
		return mediaport.MiniProgramDeleteResult{}, ErrMiniProgramNotFound
	}
	delete(store.state.items, id)
	return mediaport.MiniProgramDeleteResult{ID: id, Deleted: true, HardDeleted: true}, nil
}

func (store miniProgramStore) ImageExists(_ context.Context, id int64) (bool, error) {
	if store.state.fault == "image" {
		return false, errors.New("image lookup failed")
	}
	return store.state.images[id], nil
}

func (resolver *miniProgramResolver) ResolveMiniProgramThumbnail(_ context.Context, _ mediaport.MiniProgramCard) (mediaport.MiniProgramThumbResolution, error) {
	resolver.calls++
	return resolver.result, resolver.err
}

func newMiniProgramService() (*MiniProgramLibraryService, *miniProgramMemory, *miniProgramResolver) {
	state := &miniProgramMemory{receipts: map[string]MiniProgramReceipt{}, items: map[int64]mediaport.MiniProgramCard{}, images: map[int64]bool{19: true}}
	resolver := &miniProgramResolver{result: mediaport.MiniProgramThumbResolution{OK: true, ThumbMediaID: "fake_thumb_19", Source: "image_library_cache", AdapterMode: "fake"}}
	service := NewMiniProgramLibraryService(miniProgramUOW{state}, miniProgramStore{state}, resolver)
	service.now = func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) }
	return service, state, resolver
}

func miniProgramString(value string) *string { return &value }
func miniProgramBool(value bool) *bool       { return &value }
func miniProgramID(value int64) *int64       { return &value }

func validMiniProgramCreate() mediaport.MiniProgramCreateCommand {
	return mediaport.MiniProgramCreateCommand{MiniProgramUpsert: mediaport.MiniProgramUpsert{
		Title: miniProgramString("小程序卡片"), AppID: miniProgramString("wx_demo"), PagePath: miniProgramString("pages/index"), ThumbImageID: miniProgramID(19),
		Description: miniProgramString("accepted but not stored"), Tags: &[]string{"accepted", "but-not-stored"},
	}, Actor: 7, IdempotencyKey: "miniprogram-create-key-0001"}
}

func TestMiniProgramCreateDefaultResolveAndInputOnlyFields(t *testing.T) {
	service, state, resolver := newMiniProgramService()
	result, err := service.Create(context.Background(), validMiniProgramCreate())
	if err != nil || result.Item.ID != 1 || result.Item.Name != "小程序卡片" || result.Item.Title != "小程序卡片" || result.Item.ThumbMediaID != "fake_thumb_19" ||
		result.ThumbResolve == nil || !result.ThumbResolve.OK || result.ThumbResolve.SideEffectExecuted || result.ThumbResolve.RealExternalCallExecuted || resolver.calls != 1 {
		t.Fatalf("result=%#v calls=%d err=%v", result, resolver.calls, err)
	}
	if !result.Item.Enabled || result.Item.CreatedAt.IsZero() || result.Item.UpdatedAt.IsZero() || len(state.items) != 1 {
		t.Fatalf("item=%#v stored=%#v", result.Item, state.items)
	}
	encoded, marshalErr := json.Marshal(result.Item)
	if marshalErr != nil || strings.Contains(string(encoded), "description") || strings.Contains(string(encoded), "tags") {
		t.Fatalf("input-only fields leaked: %s err=%v", encoded, marshalErr)
	}
}

func TestMiniProgramFailureToResolvePreservesWriteAndFailsClosed(t *testing.T) {
	service, state, resolver := newMiniProgramService()
	resolver.result = mediaport.MiniProgramThumbResolution{OK: false, Error: "real_wecom_media_resolve_failed", ErrorMessage: "cache missing", AdapterMode: "disabled"}
	result, err := service.Create(context.Background(), validMiniProgramCreate())
	if err != nil || result.ThumbResolve == nil || result.ThumbResolve.OK || result.Item.ID != 1 || result.Item.ThumbMediaID != "" || len(state.items) != 1 {
		t.Fatalf("result=%#v items=%#v err=%v", result, state.items, err)
	}
	if result.ThumbResolve.SideEffectExecuted || result.ThumbResolve.RealExternalCallExecuted {
		t.Fatalf("unsafe resolution=%#v", result.ThumbResolve)
	}
	resolver.result = mediaport.MiniProgramThumbResolution{OK: true, ThumbMediaID: "provider-id", AdapterMode: "staging", SideEffectExecuted: true}
	updated, err := service.Update(context.Background(), mediaport.MiniProgramUpdateCommand{ID: 1, MiniProgramUpsert: mediaport.MiniProgramUpsert{ResolveThumbMedia: miniProgramBool(true)}, Actor: 7, IdempotencyKey: "miniprogram-update-key-0001"})
	if err != nil || updated.ThumbResolve == nil || updated.ThumbResolve.OK || updated.Item.ThumbMediaID != "" || updated.ThumbResolve.Error != "thumbnail_resolution_not_executed" {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
}

func TestMiniProgramUpdateAliasesDefaultsAndPhysicalDeleteReplay(t *testing.T) {
	service, state, _ := newMiniProgramService()
	created, err := service.Create(context.Background(), validMiniProgramCreate())
	if err != nil {
		t.Fatal(err)
	}
	disabled, name, page := false, "", "pages/next"
	updated, err := service.Update(context.Background(), mediaport.MiniProgramUpdateCommand{ID: created.Item.ID, MiniProgramUpsert: mediaport.MiniProgramUpsert{Name: &name, PagePath: &page, Enabled: &disabled, ResolveThumbMedia: miniProgramBool(false)}, Actor: 9, IdempotencyKey: "miniprogram-update-key-0002"})
	if err != nil || updated.Item.Name != "" || updated.Item.PagePath != page || updated.Item.Enabled || updated.ThumbResolve != nil || updated.Item.Version != 2 || updated.Item.UpdatedBy != 9 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	key := "miniprogram-delete-key-0001"
	deleted, err := service.Delete(context.Background(), mediaport.MiniProgramDeleteCommand{ID: created.Item.ID, Actor: 9, IdempotencyKey: key})
	if err != nil || !deleted.Deleted || !deleted.HardDeleted || len(state.items) != 0 {
		t.Fatalf("deleted=%#v items=%#v err=%v", deleted, state.items, err)
	}
	replay, err := service.Delete(context.Background(), mediaport.MiniProgramDeleteCommand{ID: created.Item.ID, Actor: 9, IdempotencyKey: key})
	if err != nil || !reflect.DeepEqual(replay, deleted) || len(state.items) != 0 {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	if _, err = service.Get(context.Background(), created.Item.ID); !errors.Is(err, ErrMiniProgramNotFound) {
		t.Fatalf("deleted item get err=%v", err)
	}
}

func TestMiniProgramPaginationSearchAndBoundaries(t *testing.T) {
	service, state, _ := newMiniProgramService()
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	for id, item := range map[int64]mediaport.MiniProgramCard{
		1: {ID: 1, Name: "A", Title: "第一", AppID: "wx-a", PagePath: "pages/a", Enabled: true, CreatedBy: 1, UpdatedBy: 1, Version: 1, CreatedAt: base, UpdatedAt: base},
		2: {ID: 2, Name: "B", Title: "第二", AppID: "wx-b", PagePath: "pages/b", Enabled: true, CreatedBy: 1, UpdatedBy: 1, Version: 1, CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
		3: {ID: 3, Name: "C", Title: "第三", AppID: "wx-c", PagePath: "pages/c", Enabled: false, CreatedBy: 1, UpdatedBy: 1, Version: 1, CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
		4: {ID: 4, Name: "B2", Title: "第四", AppID: "wx-b2", PagePath: "pages/d", Enabled: true, CreatedBy: 1, UpdatedBy: 1, Version: 1, CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
	} {
		state.items[id] = item
		state.nextID = id
	}
	page, err := service.List(context.Background(), mediaport.MiniProgramListQuery{Limit: 2, Offset: 0, EnabledOnly: true})
	if err != nil || page.Total != 3 || len(page.Items) != 2 || page.Items[0].ID != 4 || page.Items[1].ID != 2 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	search, err := service.List(context.Background(), mediaport.MiniProgramListQuery{Limit: 100, EnabledOnly: false, Search: "WX-B"})
	if err != nil || search.Total != 2 || search.Items[0].ID != 4 || search.Items[1].ID != 2 {
		t.Fatalf("search=%#v err=%v", search, err)
	}
	clamped, err := service.List(context.Background(), mediaport.MiniProgramListQuery{Limit: 501, Offset: -1})
	if err != nil || clamped.Limit != MaximumMiniProgramLimit || clamped.Offset != 0 {
		t.Fatalf("clamped=%#v err=%v", clamped, err)
	}
}

func TestMiniProgramReceiptUOWRollbackAndConcurrentReplay(t *testing.T) {
	service, state, resolver := newMiniProgramService()
	state.fault = "complete"
	if _, err := service.Create(context.Background(), validMiniProgramCreate()); !errors.Is(err, ErrMiniProgramUnavailable) || len(state.items) != 0 || len(state.receipts) != 0 {
		t.Fatalf("rollback items=%#v receipts=%#v err=%v", state.items, state.receipts, err)
	}
	state.fault, resolver.calls = "", 0
	const callers = 12
	results := make(chan mediaport.MiniProgramMutationResult, callers)
	errorsOut := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := service.Create(context.Background(), validMiniProgramCreate())
			results <- result
			errorsOut <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if result.Item.ID != 1 || result.ThumbResolve == nil || !result.ThumbResolve.OK {
			t.Fatalf("concurrent result=%#v", result)
		}
	}
	if len(state.items) != 1 || len(state.receipts) != 1 || resolver.calls != 1 {
		t.Fatalf("items=%d receipts=%d resolver=%d", len(state.items), len(state.receipts), resolver.calls)
	}
}

func TestMiniProgramContractErrorsAndResolveDoesNotCheckReachability(t *testing.T) {
	service, state, _ := newMiniProgramService()
	_, err := service.Create(context.Background(), mediaport.MiniProgramCreateCommand{MiniProgramUpsert: mediaport.MiniProgramUpsert{Title: miniProgramString("only title")}, Actor: 7, IdempotencyKey: "miniprogram-create-key-0002"})
	var contract MiniProgramContractError
	if !errors.As(err, &contract) || contract.Detail != "小程序素材缺少必填字段：appid, pagepath" {
		t.Fatalf("missing fields err=%v", err)
	}
	created, err := service.Create(context.Background(), validMiniProgramCreate())
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	_, err = service.Update(context.Background(), mediaport.MiniProgramUpdateCommand{ID: created.Item.ID, MiniProgramUpsert: mediaport.MiniProgramUpsert{AppID: &empty}, Actor: 7, IdempotencyKey: "miniprogram-update-key-0003"})
	if !errors.As(err, &contract) || contract.Detail != "小程序素材字段不能为空：appid" {
		t.Fatalf("empty appid err=%v", err)
	}
	badImage := int64(23)
	_, err = service.Update(context.Background(), mediaport.MiniProgramUpdateCommand{ID: created.Item.ID, MiniProgramUpsert: mediaport.MiniProgramUpsert{ThumbImageID: &badImage}, Actor: 7, IdempotencyKey: "miniprogram-update-key-0004"})
	if !errors.Is(err, ErrMiniProgramInvalidReference) {
		t.Fatalf("missing image err=%v", err)
	}
	// A page path is only stored metadata. No resolver receives it as a URL or
	// performs network reachability validation.
	if state.items[created.Item.ID].PagePath != "pages/index" {
		t.Fatalf("unexpected page-path mutation=%#v", state.items[created.Item.ID])
	}
}

func cloneMiniProgramCard(item mediaport.MiniProgramCard) mediaport.MiniProgramCard {
	if item.ThumbImageID != nil {
		copy := *item.ThumbImageID
		item.ThumbImageID = &copy
	}
	return item
}
