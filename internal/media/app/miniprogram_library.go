package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	DefaultMiniProgramLimit int32 = 100
	MaximumMiniProgramLimit int32 = 500
)

var (
	ErrInvalidMiniProgramOperation = errors.New("invalid miniprogram library operation")
	ErrMiniProgramNotFound         = errors.New("miniprogram item not found")
	ErrMiniProgramInvalidReference = errors.New("thumb_image_id 对应的图片素材不存在")
	ErrMiniProgramConflict         = errors.New("miniprogram library operation conflict")
	ErrMiniProgramUnavailable      = errors.New("miniprogram library unavailable")
)

// MiniProgramContractError carries the frozen legacy 400 detail without
// making HTTP a domain concern.
type MiniProgramContractError struct{ Detail string }

func (err MiniProgramContractError) Error() string { return err.Detail }

type MiniProgramReceipt struct {
	ID                                        int64
	Operation, ActorScope, BusinessKey, State string
	KeyDigest, PayloadDigest                  [32]byte
	ResultSnapshot                            json.RawMessage
}

type MiniProgramReservation struct {
	Operation, ActorScope, BusinessKey string
	KeyDigest, PayloadDigest           [32]byte
	CreatedAt                          time.Time
}

// MiniProgramLibraryStore is the exact persistence seam for the later shared
// migration. A durable adapter must keep the business mutation and receipt in
// the UnitOfWork transaction; it must not call a provider.
type MiniProgramLibraryStore interface {
	ListMiniPrograms(context.Context, mediaport.MiniProgramListQuery) ([]mediaport.MiniProgramCard, error)
	CountMiniPrograms(context.Context, mediaport.MiniProgramListQuery) (int64, error)
	GetMiniProgram(context.Context, int64) (mediaport.MiniProgramCard, error)
	LockMiniProgram(context.Context, int64) (mediaport.MiniProgramCard, error)
	CreateMiniProgram(context.Context, mediaport.MiniProgramCard) (mediaport.MiniProgramCard, error)
	UpdateMiniProgram(context.Context, mediaport.MiniProgramCard) (mediaport.MiniProgramCard, error)
	HardDeleteMiniProgram(context.Context, int64) (mediaport.MiniProgramDeleteResult, error)
	ImageExists(context.Context, int64) (bool, error)
	ReserveMiniProgram(context.Context, MiniProgramReservation) (MiniProgramReceipt, bool, error)
	CompleteMiniProgram(context.Context, int64, json.RawMessage, time.Time) (MiniProgramReceipt, error)
}

// MiniProgramThumbnailResolver can inspect an existing local cache only. A
// valid result is cache/fake/disabled/staging and must declare both effect
// flags false. Calling a real provider is deliberately outside this slice.
type MiniProgramThumbnailResolver interface {
	ResolveMiniProgramThumbnail(context.Context, mediaport.MiniProgramCard) (mediaport.MiniProgramThumbResolution, error)
}

type MiniProgramLibraryService struct {
	uow      platformport.UnitOfWork
	store    MiniProgramLibraryStore
	resolver MiniProgramThumbnailResolver
	now      func() time.Time
}

func NewMiniProgramLibraryService(uow platformport.UnitOfWork, store MiniProgramLibraryStore, resolver MiniProgramThumbnailResolver) *MiniProgramLibraryService {
	return &MiniProgramLibraryService{uow: uow, store: store, resolver: resolver, now: time.Now}
}

func (service *MiniProgramLibraryService) List(ctx context.Context, query mediaport.MiniProgramListQuery) (mediaport.MiniProgramPage, error) {
	if !miniProgramReady(service) {
		return mediaport.MiniProgramPage{}, ErrMiniProgramUnavailable
	}
	if query.Limit == 0 {
		query.Limit = DefaultMiniProgramLimit
	}
	query.Search = strings.TrimSpace(query.Search)
	// The frozen Postgres repository clamps rather than rejects pagination.
	// Keep that compatibility rule here so every later transport has one source
	// of truth, including values outside a normal UI range.
	if query.Limit < 1 {
		query.Limit = 1
	} else if query.Limit > MaximumMiniProgramLimit {
		query.Limit = MaximumMiniProgramLimit
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	page := mediaport.MiniProgramPage{Limit: query.Limit, Offset: query.Offset}
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		page.Items, err = service.store.ListMiniPrograms(tx, query)
		if err == nil {
			page.Total, err = service.store.CountMiniPrograms(tx, query)
		}
		return err
	})
	if err != nil || page.Total < 0 || len(page.Items) > int(query.Limit) {
		return mediaport.MiniProgramPage{}, classifyMiniProgram(err)
	}
	for _, item := range page.Items {
		if !domain.ValidMiniProgram(item, true) {
			return mediaport.MiniProgramPage{}, ErrMiniProgramUnavailable
		}
	}
	return page, nil
}

func (service *MiniProgramLibraryService) Get(ctx context.Context, id int64) (mediaport.MiniProgramCard, error) {
	if !miniProgramReady(service) || id < 1 {
		return mediaport.MiniProgramCard{}, ErrInvalidMiniProgramOperation
	}
	var item mediaport.MiniProgramCard
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		item, err = service.store.GetMiniProgram(tx, id)
		return err
	})
	if err != nil {
		return mediaport.MiniProgramCard{}, classifyMiniProgram(err)
	}
	if !domain.ValidMiniProgram(item, true) {
		return mediaport.MiniProgramCard{}, ErrMiniProgramUnavailable
	}
	return item, nil
}

func (service *MiniProgramLibraryService) Create(ctx context.Context, command mediaport.MiniProgramCreateCommand) (mediaport.MiniProgramMutationResult, error) {
	now, err := service.commandTime(command.Actor, command.IdempotencyKey)
	if err != nil {
		return mediaport.MiniProgramMutationResult{}, err
	}
	if missing := missingCreateFields(command.MiniProgramUpsert); len(missing) > 0 {
		return mediaport.MiniProgramMutationResult{}, MiniProgramContractError{Detail: "小程序素材缺少必填字段：" + strings.Join(missing, ", ")}
	}
	item, err := domain.NewMiniProgram(command.MiniProgramUpsert, command.Actor, now)
	if err != nil {
		return mediaport.MiniProgramMutationResult{}, ErrInvalidMiniProgramOperation
	}
	return service.mutate(ctx, "create", "create", command.Actor, command.IdempotencyKey, command.MiniProgramUpsert, now, func(tx context.Context) (mediaport.MiniProgramCard, error) {
		if err := service.validateThumbnailReference(tx, item.ThumbImageID); err != nil {
			return mediaport.MiniProgramCard{}, err
		}
		return service.store.CreateMiniProgram(tx, item)
	})
}

func (service *MiniProgramLibraryService) Update(ctx context.Context, command mediaport.MiniProgramUpdateCommand) (mediaport.MiniProgramMutationResult, error) {
	now, err := service.commandTime(command.Actor, command.IdempotencyKey)
	if err != nil || command.ID < 1 {
		return mediaport.MiniProgramMutationResult{}, ErrInvalidMiniProgramOperation
	}
	if err = validateUpdatedRequiredFields(command.MiniProgramUpsert); err != nil {
		return mediaport.MiniProgramMutationResult{}, err
	}
	return service.mutate(ctx, "update", fmt.Sprintf("%d", command.ID), command.Actor, command.IdempotencyKey, command.MiniProgramUpsert, now, func(tx context.Context) (mediaport.MiniProgramCard, error) {
		current, err := service.store.LockMiniProgram(tx, command.ID)
		if err != nil {
			return mediaport.MiniProgramCard{}, err
		}
		if !domain.ValidMiniProgram(current, true) {
			return mediaport.MiniProgramCard{}, ErrMiniProgramUnavailable
		}
		if domain.EmptyMiniProgramPatch(command.MiniProgramUpsert) {
			return current, nil
		}
		updated, err := domain.ApplyMiniProgramPatch(current, command.MiniProgramUpsert, command.Actor, now)
		if err != nil {
			return mediaport.MiniProgramCard{}, ErrInvalidMiniProgramOperation
		}
		if err = service.validateThumbnailReference(tx, updated.ThumbImageID); err != nil {
			return mediaport.MiniProgramCard{}, err
		}
		return service.store.UpdateMiniProgram(tx, updated)
	})
}

func (service *MiniProgramLibraryService) Delete(ctx context.Context, command mediaport.MiniProgramDeleteCommand) (mediaport.MiniProgramDeleteResult, error) {
	now, err := service.commandTime(command.Actor, command.IdempotencyKey)
	if err != nil || command.ID < 1 {
		return mediaport.MiniProgramDeleteResult{}, ErrInvalidMiniProgramOperation
	}
	payload, err := json.Marshal(struct{ ID int64 }{command.ID})
	if err != nil {
		return mediaport.MiniProgramDeleteResult{}, ErrMiniProgramUnavailable
	}
	reservation := miniProgramReservation("delete", fmt.Sprintf("%d", command.ID), command.Actor, command.IdempotencyKey, payload, now)
	var result mediaport.MiniProgramDeleteResult
	err = service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveMiniProgram(tx, reservation)
		if reserveErr != nil || !validMiniProgramReceipt(receipt, reservation) {
			return ErrMiniProgramUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrMiniProgramConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !result.Deleted || !result.HardDeleted || result.ID != command.ID || !jsonSemanticEqual(receipt.ResultSnapshot, mustJSON(result)) {
				return ErrMiniProgramUnavailable
			}
			return nil
		}
		result, reserveErr = service.store.HardDeleteMiniProgram(tx, command.ID)
		if reserveErr != nil || !result.Deleted || !result.HardDeleted || result.ID != command.ID {
			return reserveErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return ErrMiniProgramUnavailable
		}
		completed, completeErr := service.store.CompleteMiniProgram(tx, receipt.ID, snapshot, now)
		if completeErr != nil || completed.State != "completed" || !jsonSemanticEqual(completed.ResultSnapshot, snapshot) {
			return ErrMiniProgramUnavailable
		}
		return nil
	})
	if err != nil {
		return mediaport.MiniProgramDeleteResult{}, classifyMiniProgram(err)
	}
	return result, nil
}

func (service *MiniProgramLibraryService) TestResolve(ctx context.Context, command mediaport.MiniProgramResolveCommand) (mediaport.MiniProgramThumbResolution, error) {
	now, err := service.commandTime(command.Actor, command.IdempotencyKey)
	if err != nil || command.ID < 1 {
		return mediaport.MiniProgramThumbResolution{}, ErrInvalidMiniProgramOperation
	}
	payload, err := json.Marshal(struct{ ID int64 }{command.ID})
	if err != nil {
		return mediaport.MiniProgramThumbResolution{}, ErrMiniProgramUnavailable
	}
	reservation := miniProgramReservation("test-resolve", fmt.Sprintf("%d", command.ID), command.Actor, command.IdempotencyKey, payload, now)
	var result mediaport.MiniProgramThumbResolution
	err = service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveMiniProgram(tx, reservation)
		if reserveErr != nil || !validMiniProgramReceipt(receipt, reservation) {
			return ErrMiniProgramUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrMiniProgramConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validThumbResolution(result) || !jsonSemanticEqual(receipt.ResultSnapshot, mustJSON(result)) {
				return ErrMiniProgramUnavailable
			}
			return nil
		}
		item, innerErr := service.store.LockMiniProgram(tx, command.ID)
		if innerErr != nil {
			return innerErr
		}
		result, item, innerErr = service.resolveThumbnail(tx, item)
		if innerErr != nil {
			return innerErr
		}
		if result.OK {
			if item, innerErr = service.store.UpdateMiniProgram(tx, item); innerErr != nil {
				return innerErr
			}
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return ErrMiniProgramUnavailable
		}
		completed, completeErr := service.store.CompleteMiniProgram(tx, receipt.ID, snapshot, now)
		if completeErr != nil || completed.State != "completed" || !jsonSemanticEqual(completed.ResultSnapshot, snapshot) {
			return ErrMiniProgramUnavailable
		}
		return nil
	})
	if err != nil {
		return mediaport.MiniProgramThumbResolution{}, classifyMiniProgram(err)
	}
	return result, nil
}

func (service *MiniProgramLibraryService) mutate(ctx context.Context, operation, businessKey string, actor int64, key string, patch mediaport.MiniProgramUpsert, now time.Time, write func(context.Context) (mediaport.MiniProgramCard, error)) (mediaport.MiniProgramMutationResult, error) {
	payload, err := json.Marshal(struct {
		Operation, BusinessKey string
		Patch                  mediaport.MiniProgramUpsert
	}{operation, businessKey, patch})
	if err != nil {
		return mediaport.MiniProgramMutationResult{}, ErrMiniProgramUnavailable
	}
	reservation := miniProgramReservation(operation, businessKey, actor, key, payload, now)
	var result mediaport.MiniProgramMutationResult
	err = service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveMiniProgram(tx, reservation)
		if reserveErr != nil || !validMiniProgramReceipt(receipt, reservation) {
			return ErrMiniProgramUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrMiniProgramConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !domain.ValidMiniProgram(result.Item, true) || !jsonSemanticEqual(receipt.ResultSnapshot, mustJSON(result)) {
				return ErrMiniProgramUnavailable
			}
			return nil
		}
		item, innerErr := write(tx)
		if innerErr != nil {
			return innerErr
		}
		if !domain.ValidMiniProgram(item, true) {
			return ErrMiniProgramUnavailable
		}
		result.Item = item
		if resolveByDefault(patch) && item.ThumbImageID != nil {
			resolution, resolvedItem, resolveErr := service.resolveThumbnail(tx, item)
			result.ThumbResolve, item, innerErr = &resolution, resolvedItem, resolveErr
			if innerErr != nil {
				return innerErr
			}
			if result.ThumbResolve.OK {
				result.Item, innerErr = service.store.UpdateMiniProgram(tx, item)
				if innerErr != nil {
					return innerErr
				}
			}
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return ErrMiniProgramUnavailable
		}
		completed, completeErr := service.store.CompleteMiniProgram(tx, receipt.ID, snapshot, now)
		if completeErr != nil || completed.State != "completed" || !jsonSemanticEqual(completed.ResultSnapshot, snapshot) {
			return ErrMiniProgramUnavailable
		}
		return nil
	})
	if err != nil {
		return mediaport.MiniProgramMutationResult{}, classifyMiniProgram(err)
	}
	return result, nil
}

func (service *MiniProgramLibraryService) resolveThumbnail(ctx context.Context, item mediaport.MiniProgramCard) (mediaport.MiniProgramThumbResolution, mediaport.MiniProgramCard, error) {
	if item.ThumbImageID == nil {
		return mediaport.MiniProgramThumbResolution{OK: false, Error: "thumb_image_id is required before resolving WeCom media"}, item, nil
	}
	if service.resolver == nil {
		return mediaport.MiniProgramThumbResolution{OK: false, Error: "real_wecom_media_resolve_failed", ErrorMessage: "image_library must contain a real WeCom media_id before miniprogram material can be resolved in production", ThumbImageID: cloneMiniProgramID(item.ThumbImageID)}, item, nil
	}
	resolution, err := service.resolver.ResolveMiniProgramThumbnail(ctx, item)
	if err != nil {
		return mediaport.MiniProgramThumbResolution{}, item, err
	}
	if !safeMiniProgramResolution(resolution) {
		return mediaport.MiniProgramThumbResolution{OK: false, Error: "thumbnail_resolution_not_executed", ErrorMessage: "thumbnail resolution reported an external effect and was rejected", ThumbImageID: cloneMiniProgramID(item.ThumbImageID)}, item, nil
	}
	if !resolution.OK {
		resolution.ThumbImageID = cloneMiniProgramID(item.ThumbImageID)
		return resolution, item, nil
	}
	if strings.TrimSpace(resolution.ThumbMediaID) == "" {
		return mediaport.MiniProgramThumbResolution{OK: false, Error: "wecom media adapter unavailable", ThumbImageID: cloneMiniProgramID(item.ThumbImageID)}, item, nil
	}
	item.ThumbMediaID = truncateMiniProgramThumbMediaID(resolution.ThumbMediaID)
	resolution.ThumbMediaID = item.ThumbMediaID
	resolution.ThumbImageID = cloneMiniProgramID(item.ThumbImageID)
	return resolution, item, nil
}

func (service *MiniProgramLibraryService) validateThumbnailReference(ctx context.Context, id *int64) error {
	if id == nil {
		return nil
	}
	exists, err := service.store.ImageExists(ctx, *id)
	if err != nil {
		return err
	}
	if !exists {
		return ErrMiniProgramInvalidReference
	}
	return nil
}

func (service *MiniProgramLibraryService) commandTime(actor int64, key string) (time.Time, error) {
	if !miniProgramReady(service) || actor < 1 || len(key) < 16 || len(key) > 128 || strings.TrimSpace(key) != key {
		return time.Time{}, ErrInvalidMiniProgramOperation
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return time.Time{}, ErrMiniProgramUnavailable
	}
	return now, nil
}

func miniProgramReady(service *MiniProgramLibraryService) bool {
	return service != nil && service.uow != nil && service.store != nil && service.now != nil
}

func miniProgramReservation(operation, businessKey string, actor int64, key string, payload []byte, now time.Time) MiniProgramReservation {
	return MiniProgramReservation{Operation: operation, ActorScope: fmt.Sprintf("admin:%d", actor), BusinessKey: businessKey,
		KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now}
}

func validMiniProgramReceipt(receipt MiniProgramReceipt, reservation MiniProgramReservation) bool {
	return receipt.ID > 0 && receipt.Operation == reservation.Operation && receipt.ActorScope == reservation.ActorScope && receipt.BusinessKey == reservation.BusinessKey &&
		subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 && (receipt.State == "in_progress" || receipt.State == "completed")
}

func missingCreateFields(input mediaport.MiniProgramUpsert) []string {
	title := miniProgramInputText(input.Title)
	name := miniProgramInputText(input.Name)
	if title == "" {
		title = name
	}
	missing := make([]string, 0, 3)
	if miniProgramInputText(input.AppID) == "" {
		missing = append(missing, "appid")
	}
	if miniProgramInputText(input.PagePath) == "" {
		missing = append(missing, "pagepath")
	}
	if title == "" {
		missing = append(missing, "title")
	}
	return missing
}

func validateUpdatedRequiredFields(input mediaport.MiniProgramUpsert) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"appid", input.AppID}, {"pagepath", input.PagePath}, {"title", input.Title}} {
		if field.value != nil && miniProgramInputText(field.value) == "" {
			return MiniProgramContractError{Detail: "小程序素材字段不能为空：" + field.name}
		}
	}
	return nil
}

func resolveByDefault(input mediaport.MiniProgramUpsert) bool {
	return input.ResolveThumbMedia == nil || *input.ResolveThumbMedia
}

func safeMiniProgramResolution(resolution mediaport.MiniProgramThumbResolution) bool {
	if resolution.SideEffectExecuted || resolution.RealExternalCallExecuted {
		return false
	}
	switch resolution.AdapterMode {
	case "", "fake", "disabled", "staging", "cache":
		return true
	default:
		return false
	}
}

func validThumbResolution(resolution mediaport.MiniProgramThumbResolution) bool {
	return !resolution.SideEffectExecuted && !resolution.RealExternalCallExecuted && safeMiniProgramResolution(resolution) &&
		(!resolution.OK || strings.TrimSpace(resolution.ThumbMediaID) != "")
}

func classifyMiniProgram(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidMiniProgramOperation), errors.Is(err, ErrMiniProgramNotFound), errors.Is(err, ErrMiniProgramInvalidReference), errors.Is(err, ErrMiniProgramConflict):
		return err
	default:
		var contract MiniProgramContractError
		if errors.As(err, &contract) {
			return err
		}
		return ErrMiniProgramUnavailable
	}
}

func miniProgramInputText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func truncateMiniProgramThumbMediaID(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > domain.MaxMiniProgramThumbMediaIDRunes {
		runes = runes[:domain.MaxMiniProgramThumbMediaIDRunes]
	}
	return string(runes)
}

func cloneMiniProgramID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
