package legacyaudience

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

var _ Application = (*Service)(nil)

type Service struct {
	uow      UnitOfWork
	repo     Repository
	segments SegmentReader
	events   EventAppender
	now      func() time.Time
}

func NewService(uow UnitOfWork, repo Repository, segments SegmentReader, events EventAppender) (*Service, error) {
	if nilInterface(uow) || nilInterface(repo) || nilInterface(segments) || nilInterface(events) {
		return nil, ErrUnavailable
	}
	return &Service{uow: uow, repo: repo, segments: segments, events: events, now: time.Now}, nil
}

func (service *Service) ListGroups(ctx context.Context) (GroupListResponse, error) {
	if !service.ready(ctx) {
		return GroupListResponse{}, ErrUnavailable
	}
	groups, err := service.repo.ListGroups(ctx)
	if err != nil {
		return GroupListResponse{}, classifyServiceError(err)
	}
	if err = validateGroups(groups); err != nil {
		return GroupListResponse{}, err
	}
	items := make([]Group, len(groups))
	copy(items, groups)
	return GroupListResponse{Items: items, Projection: localProjection()}, nil
}

func (service *Service) CreateGroup(ctx context.Context, input CreateGroupInput) (GroupMutationResponse, error) {
	if err := validateWriteCommon(input.Actor, input.IdempotencyKey, input.ExpectedVersion, true); err != nil {
		return GroupMutationResponse{}, err
	}
	name, err := normalizeGroupName(input.Name)
	if err != nil {
		return GroupMutationResponse{}, err
	}
	sortOrder, err := normalizeSortOrder(input.SortOrder)
	if err != nil {
		return GroupMutationResponse{}, err
	}
	payloadDigest, err := digestJSON(struct {
		Name            string `json:"name"`
		SortOrder       int32  `json:"sort_order"`
		ExpectedVersion int64  `json:"expected_version"`
	}{name, sortOrder, input.ExpectedVersion})
	if err != nil {
		return GroupMutationResponse{}, err
	}
	raw, err := service.executeMutation(ctx, OperationGroupCreate, input.Actor, input.IdempotencyKey, payloadDigest,
		func(txCtx context.Context, now time.Time) (any, *LocalEvent, error) {
			group, createErr := service.repo.InsertGroup(txCtx, name, sortOrder, input.Actor.AdminUserID, now)
			if createErr != nil {
				return nil, nil, createErr
			}
			if validationErr := validateGroup(group); validationErr != nil || group.Version != 1 || group.Name != name || group.SortOrder != sortOrder {
				return nil, nil, ErrUnavailable
			}
			response := GroupMutationResponse{Group: group, Projection: localProjection()}
			event, eventErr := mutationEvent("ai_audience.package_group.created", group.ID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return GroupMutationResponse{}, err
	}
	return decodeMutation[GroupMutationResponse](raw)
}

func (service *Service) UpdateGroup(ctx context.Context, input UpdateGroupInput) (GroupMutationResponse, error) {
	if input.GroupID <= 0 || (input.Name == nil && input.SortOrder == nil) {
		return GroupMutationResponse{}, ErrInvalidInput
	}
	if err := validateWriteCommon(input.Actor, input.IdempotencyKey, input.ExpectedVersion, false); err != nil {
		return GroupMutationResponse{}, err
	}
	var name *string
	if input.Name != nil {
		normalized, err := normalizeGroupName(*input.Name)
		if err != nil {
			return GroupMutationResponse{}, err
		}
		name = &normalized
	}
	var sortOrder *int32
	if input.SortOrder != nil {
		normalized, err := normalizeSortOrder(*input.SortOrder)
		if err != nil {
			return GroupMutationResponse{}, err
		}
		sortOrder = &normalized
	}
	payloadDigest, err := digestJSON(struct {
		GroupID         int64   `json:"group_id"`
		Name            *string `json:"name"`
		SortOrder       *int32  `json:"sort_order"`
		ExpectedVersion int64   `json:"expected_version"`
	}{input.GroupID, name, sortOrder, input.ExpectedVersion})
	if err != nil {
		return GroupMutationResponse{}, err
	}
	raw, err := service.executeMutation(ctx, OperationGroupUpdate, input.Actor, input.IdempotencyKey, payloadDigest,
		func(txCtx context.Context, now time.Time) (any, *LocalEvent, error) {
			current, lockErr := service.repo.LockGroup(txCtx, input.GroupID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validationErr := validateGroup(current); validationErr != nil {
				return nil, nil, validationErr
			}
			if current.Version != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			nextName, nextSort := current.Name, current.SortOrder
			if name != nil {
				nextName = *name
			}
			if sortOrder != nil {
				nextSort = *sortOrder
			}
			updated, updateErr := service.repo.UpdateGroup(txCtx, current, nextName, nextSort, input.Actor.AdminUserID, now)
			if updateErr != nil {
				return nil, nil, updateErr
			}
			if validationErr := validateGroup(updated); validationErr != nil || updated.Version != current.Version+1 {
				return nil, nil, ErrUnavailable
			}
			response := GroupMutationResponse{Group: updated, Projection: localProjection()}
			event, eventErr := mutationEvent("ai_audience.package_group.updated", updated.ID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return GroupMutationResponse{}, err
	}
	return decodeMutation[GroupMutationResponse](raw)
}

func (service *Service) DeleteGroup(ctx context.Context, input DeleteGroupInput) (GroupDeleteResponse, error) {
	if input.GroupID <= 0 {
		return GroupDeleteResponse{}, ErrInvalidInput
	}
	if err := validateWriteCommon(input.Actor, input.IdempotencyKey, input.ExpectedVersion, false); err != nil {
		return GroupDeleteResponse{}, err
	}
	payloadDigest, err := digestJSON(struct {
		GroupID         int64 `json:"group_id"`
		ExpectedVersion int64 `json:"expected_version"`
	}{input.GroupID, input.ExpectedVersion})
	if err != nil {
		return GroupDeleteResponse{}, err
	}
	raw, err := service.executeMutation(ctx, OperationGroupDelete, input.Actor, input.IdempotencyKey, payloadDigest,
		func(txCtx context.Context, now time.Time) (any, *LocalEvent, error) {
			current, lockErr := service.repo.LockGroup(txCtx, input.GroupID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validationErr := validateGroup(current); validationErr != nil {
				return nil, nil, validationErr
			}
			if current.Version != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			count, countErr := service.repo.CountPackagesInGroup(txCtx, current.ID)
			if countErr != nil {
				return nil, nil, countErr
			}
			if count < 0 {
				return nil, nil, ErrUnavailable
			}
			if count != 0 {
				return nil, nil, ErrGroupNotEmpty
			}
			if deleteErr := service.repo.DeleteGroup(txCtx, current.ID, current.Version); deleteErr != nil {
				return nil, nil, deleteErr
			}
			response := GroupDeleteResponse{
				GroupID: current.ID, Version: current.Version, Deleted: true, Projection: localProjection(),
			}
			event, eventErr := mutationEvent("ai_audience.package_group.deleted", current.ID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return GroupDeleteResponse{}, err
	}
	return decodeMutation[GroupDeleteResponse](raw)
}

func (service *Service) ListPackages(ctx context.Context, input ListPackagesInput) (PackageListResponse, error) {
	if !service.ready(ctx) {
		return PackageListResponse{}, ErrUnavailable
	}
	normalized, err := normalizePagination(input)
	if err != nil {
		return PackageListResponse{}, err
	}
	metadata, total, err := service.repo.ListPackageMetadata(ctx, normalized.GroupID, normalized.Limit, normalized.Offset)
	if err != nil {
		return PackageListResponse{}, classifyServiceError(err)
	}
	if total < 0 || len(metadata) > normalized.Limit || len(metadata) > 0 && int64(normalized.Offset+len(metadata)) > total {
		return PackageListResponse{}, ErrUnavailable
	}
	items := make([]PackageSummary, len(metadata))
	for index, record := range metadata {
		if err = validateMetadata(record); err != nil || index > 0 && metadata[index-1].SegmentID >= record.SegmentID {
			return PackageListResponse{}, ErrUnavailable
		}
		segment, getErr := service.segments.Get(ctx, segmentport.SegmentID(record.SegmentID))
		if getErr != nil {
			return PackageListResponse{}, errors.Join(ErrUnavailable, getErr)
		}
		item, composeErr := packageFrom(record, segment)
		if composeErr != nil {
			return PackageListResponse{}, composeErr
		}
		items[index] = packageSummary(item)
	}
	return PackageListResponse{
		Items: items, Limit: normalized.Limit, Offset: normalized.Offset, Total: total, Projection: localProjection(),
	}, nil
}

func (service *Service) GetPackage(ctx context.Context, packageID int64) (PackageDetailResponse, error) {
	if packageID <= 0 || !service.ready(ctx) {
		if packageID <= 0 {
			return PackageDetailResponse{}, ErrInvalidInput
		}
		return PackageDetailResponse{}, ErrUnavailable
	}
	metadata, err := service.repo.GetPackageMetadata(ctx, packageID)
	if err != nil {
		return PackageDetailResponse{}, classifyServiceError(err)
	}
	segment, err := service.segments.Get(ctx, segmentport.SegmentID(packageID))
	if err != nil {
		return PackageDetailResponse{}, errors.Join(ErrUnavailable, err)
	}
	item, err := packageFrom(metadata, segment)
	if err != nil {
		return PackageDetailResponse{}, err
	}
	return PackageDetailResponse{Package: item, Projection: localProjection()}, nil
}

func (service *Service) UpdatePackage(ctx context.Context, input UpdatePackageInput) (PackageMutationResponse, error) {
	if input.PackageID <= 0 || (input.Name == nil && input.Definition == nil && input.RefreshMode == nil && !input.RefreshCron.Set && !input.GroupID.Set) {
		return PackageMutationResponse{}, ErrInvalidInput
	}
	if err := validateWriteCommon(input.Actor, input.IdempotencyKey, input.ExpectedVersion, false); err != nil {
		return PackageMutationResponse{}, err
	}
	var name *string
	if input.Name != nil {
		normalized, err := normalizePackageName(*input.Name)
		if err != nil {
			return PackageMutationResponse{}, err
		}
		name = &normalized
	}
	var definition *segmentport.Definition
	if input.Definition != nil {
		canonical, err := canonicalDefinition(*input.Definition)
		if err != nil {
			return PackageMutationResponse{}, err
		}
		definition = &canonical
	}
	var refreshMode *segmentport.RefreshMode
	if input.RefreshMode != nil {
		if *input.RefreshMode != segmentport.RefreshModeManual && *input.RefreshMode != segmentport.RefreshModeScheduled {
			return PackageMutationResponse{}, ErrInvalidInput
		}
		value := *input.RefreshMode
		refreshMode = &value
	}
	refreshCron := OptionalString{Set: input.RefreshCron.Set, Value: cloneString(input.RefreshCron.Value)}
	groupID := OptionalInt64{Set: input.GroupID.Set, Value: cloneInt64(input.GroupID.Value)}
	if groupID.Set && groupID.Value != nil && *groupID.Value <= 0 {
		return PackageMutationResponse{}, ErrInvalidInput
	}
	payloadDigest, err := digestJSON(struct {
		PackageID       int64                    `json:"package_id"`
		Name            *string                  `json:"name"`
		Definition      *segmentport.Definition  `json:"definition"`
		RefreshMode     *segmentport.RefreshMode `json:"refresh_mode"`
		RefreshCronSet  bool                     `json:"refresh_cron_set"`
		RefreshCron     *string                  `json:"refresh_cron"`
		GroupIDSet      bool                     `json:"group_id_set"`
		GroupID         *int64                   `json:"group_id"`
		ExpectedVersion int64                    `json:"expected_version"`
	}{input.PackageID, name, definition, refreshMode, refreshCron.Set, refreshCron.Value, groupID.Set, groupID.Value, input.ExpectedVersion})
	if err != nil {
		return PackageMutationResponse{}, err
	}
	raw, err := service.executeMutation(ctx, OperationPackageUpdate, input.Actor, input.IdempotencyKey, payloadDigest,
		func(txCtx context.Context, now time.Time) (any, *LocalEvent, error) {
			current, lockErr := service.repo.LockPackage(txCtx, input.PackageID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validationErr := validateWriteModel(current); validationErr != nil {
				return nil, nil, validationErr
			}
			if current.Metadata.Lifecycle == PackageArchived {
				return nil, nil, ErrArchived
			}
			if current.Metadata.Version != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			if groupID.Set && groupID.Value != nil {
				group, groupErr := service.repo.LockGroup(txCtx, *groupID.Value)
				if groupErr != nil {
					return nil, nil, groupErr
				}
				if validationErr := validateGroup(group); validationErr != nil {
					return nil, nil, validationErr
				}
			}
			next := cloneWriteModel(current)
			if name != nil {
				next.Name = *name
			}
			if definition != nil {
				next.Definition = cloneDefinition(*definition)
			}
			if refreshMode != nil {
				next.RefreshMode = *refreshMode
			}
			if refreshCron.Set {
				next.RefreshCron = cloneString(refreshCron.Value)
			}
			if groupID.Set {
				next.Metadata.GroupID = cloneInt64(groupID.Value)
			}
			canonical, canonicalErr := canonicalDefinition(next.Definition)
			if canonicalErr != nil {
				return nil, nil, canonicalErr
			}
			next.Definition = canonical
			next.RefreshCron, canonicalErr = canonicalRefreshCron(next.RefreshMode, next.RefreshCron)
			if canonicalErr != nil {
				return nil, nil, canonicalErr
			}
			next.Metadata.Version = current.Metadata.Version + 1
			next.Metadata.UpdatedBy = input.Actor.AdminUserID
			next.Metadata.UpdatedAt = now
			updated, saveErr := service.repo.SavePackage(txCtx, current, next, input.ExpectedVersion, input.Actor.AdminUserID, now)
			if saveErr != nil {
				return nil, nil, saveErr
			}
			if validationErr := validateWriteModel(updated); validationErr != nil || updated.Metadata.Version != current.Metadata.Version+1 {
				return nil, nil, ErrUnavailable
			}
			response := PackageMutationResponse{Package: packageMutation(updated, nil), Projection: localProjection()}
			event, eventErr := mutationEvent("ai_audience.package.updated", updated.SegmentID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return PackageMutationResponse{}, err
	}
	return decodeMutation[PackageMutationResponse](raw)
}

func (service *Service) CopyPackage(ctx context.Context, input PackageCommand) (PackageMutationResponse, error) {
	if input.PackageID <= 0 {
		return PackageMutationResponse{}, ErrInvalidInput
	}
	if err := validateWriteCommon(input.Actor, input.IdempotencyKey, input.ExpectedVersion, false); err != nil {
		return PackageMutationResponse{}, err
	}
	payloadDigest, err := commandDigest(input)
	if err != nil {
		return PackageMutationResponse{}, err
	}
	raw, err := service.executeMutation(ctx, OperationPackageCopy, input.Actor, input.IdempotencyKey, payloadDigest,
		func(txCtx context.Context, now time.Time) (any, *LocalEvent, error) {
			source, lockErr := service.repo.LockPackage(txCtx, input.PackageID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validationErr := validateWriteModel(source); validationErr != nil {
				return nil, nil, validationErr
			}
			if source.Metadata.Lifecycle == PackageArchived {
				return nil, nil, ErrArchived
			}
			if source.Metadata.Version != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			if lockErr = service.repo.LockCopyNameNamespace(txCtx, source.Name); lockErr != nil {
				return nil, nil, lockErr
			}
			var copyName string
			for ordinal := 1; ordinal <= 10000; ordinal++ {
				candidate, nameErr := deterministicCopyName(source.Name, ordinal)
				if nameErr != nil {
					return nil, nil, nameErr
				}
				exists, existsErr := service.repo.PackageNameExists(txCtx, candidate)
				if existsErr != nil {
					return nil, nil, existsErr
				}
				if !exists {
					copyName = candidate
					break
				}
			}
			if copyName == "" {
				return nil, nil, ErrConflict
			}
			copied, copyErr := service.repo.InsertPackageCopy(txCtx, source, copyName, input.Actor.AdminUserID, now)
			if copyErr != nil {
				return nil, nil, copyErr
			}
			if validationErr := validateWriteModel(copied); validationErr != nil || copied.SegmentID == source.SegmentID ||
				copied.Name != copyName || copied.Metadata.Lifecycle != PackagePaused || copied.Metadata.Version != 1 ||
				copied.SegmentLifecycle != segmentport.LifecycleStatusActive || !bytes.Equal(copied.Definition, source.Definition) {
				return nil, nil, ErrUnavailable
			}
			zero := int64(0)
			response := PackageMutationResponse{Package: packageMutation(copied, &zero), Projection: localProjection()}
			event, eventErr := mutationEvent("ai_audience.package.copied", copied.SegmentID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return PackageMutationResponse{}, err
	}
	return decodeMutation[PackageMutationResponse](raw)
}

func (service *Service) PausePackage(ctx context.Context, input PackageCommand) (PackageMutationResponse, error) {
	return service.transitionPackage(ctx, input, OperationPackagePause, PackagePaused, "ai_audience.package.paused")
}

func (service *Service) ActivatePackage(ctx context.Context, input PackageCommand) (PackageMutationResponse, error) {
	return service.transitionPackage(ctx, input, OperationPackageActivate, PackageActive, "ai_audience.package.activated")
}

func (service *Service) transitionPackage(ctx context.Context, input PackageCommand, operation ReceiptOperation, target PackageLifecycle, eventType string) (PackageMutationResponse, error) {
	if input.PackageID <= 0 || target != PackagePaused && target != PackageActive {
		return PackageMutationResponse{}, ErrInvalidInput
	}
	if err := validateWriteCommon(input.Actor, input.IdempotencyKey, input.ExpectedVersion, false); err != nil {
		return PackageMutationResponse{}, err
	}
	payloadDigest, err := commandDigest(input)
	if err != nil {
		return PackageMutationResponse{}, err
	}
	raw, err := service.executeMutation(ctx, operation, input.Actor, input.IdempotencyKey, payloadDigest,
		func(txCtx context.Context, now time.Time) (any, *LocalEvent, error) {
			current, lockErr := service.repo.LockPackage(txCtx, input.PackageID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validationErr := validateWriteModel(current); validationErr != nil {
				return nil, nil, validationErr
			}
			if current.Metadata.Lifecycle == PackageArchived {
				return nil, nil, ErrArchived
			}
			if current.Metadata.Version != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			if current.Metadata.Lifecycle == target {
				response := PackageMutationResponse{Package: packageMutation(current, nil), Projection: localProjection()}
				return response, nil, nil
			}
			next := cloneWriteModel(current)
			next.Metadata.Lifecycle = target
			next.Metadata.Version++
			next.Metadata.UpdatedBy = input.Actor.AdminUserID
			next.Metadata.UpdatedAt = now
			updated, saveErr := service.repo.SavePackage(txCtx, current, next, input.ExpectedVersion, input.Actor.AdminUserID, now)
			if saveErr != nil {
				return nil, nil, saveErr
			}
			if validationErr := validateWriteModel(updated); validationErr != nil || updated.Metadata.Lifecycle != target || updated.Metadata.Version != current.Metadata.Version+1 {
				return nil, nil, ErrUnavailable
			}
			response := PackageMutationResponse{Package: packageMutation(updated, nil), Projection: localProjection()}
			event, eventErr := mutationEvent(eventType, updated.SegmentID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return PackageMutationResponse{}, err
	}
	return decodeMutation[PackageMutationResponse](raw)
}

func (service *Service) ArchivePackage(ctx context.Context, input PackageCommand) (PackageArchiveResponse, error) {
	if input.PackageID <= 0 {
		return PackageArchiveResponse{}, ErrInvalidInput
	}
	if err := validateWriteCommon(input.Actor, input.IdempotencyKey, input.ExpectedVersion, false); err != nil {
		return PackageArchiveResponse{}, err
	}
	payloadDigest, err := commandDigest(input)
	if err != nil {
		return PackageArchiveResponse{}, err
	}
	raw, err := service.executeMutation(ctx, OperationPackageArchive, input.Actor, input.IdempotencyKey, payloadDigest,
		func(txCtx context.Context, now time.Time) (any, *LocalEvent, error) {
			current, lockErr := service.repo.LockPackage(txCtx, input.PackageID)
			if lockErr != nil {
				return nil, nil, lockErr
			}
			if validationErr := validateWriteModel(current); validationErr != nil {
				return nil, nil, validationErr
			}
			if current.Metadata.Lifecycle == PackageArchived {
				response := PackageArchiveResponse{
					PackageID: current.SegmentID, Lifecycle: PackageArchived, Version: current.Metadata.Version,
					Archived: true, Projection: localProjection(),
				}
				return response, nil, nil
			}
			if current.Metadata.Version != input.ExpectedVersion {
				return nil, nil, ErrVersionConflict
			}
			next := cloneWriteModel(current)
			next.Metadata.Lifecycle = PackageArchived
			next.Metadata.Version++
			next.Metadata.UpdatedBy = input.Actor.AdminUserID
			next.Metadata.UpdatedAt = now
			next.SegmentLifecycle = segmentport.LifecycleStatusArchived
			updated, saveErr := service.repo.SavePackage(txCtx, current, next, input.ExpectedVersion, input.Actor.AdminUserID, now)
			if saveErr != nil {
				return nil, nil, saveErr
			}
			if validationErr := validateWriteModel(updated); validationErr != nil || updated.Metadata.Lifecycle != PackageArchived ||
				updated.SegmentLifecycle != segmentport.LifecycleStatusArchived || updated.Metadata.Version != current.Metadata.Version+1 {
				return nil, nil, ErrUnavailable
			}
			response := PackageArchiveResponse{
				PackageID: updated.SegmentID, Lifecycle: PackageArchived, Version: updated.Metadata.Version,
				Archived: true, Projection: localProjection(),
			}
			event, eventErr := mutationEvent("ai_audience.package.archived", updated.SegmentID, input.Actor, input.IdempotencyKey, now)
			return response, event, eventErr
		})
	if err != nil {
		return PackageArchiveResponse{}, err
	}
	return decodeMutation[PackageArchiveResponse](raw)
}

func commandDigest(input PackageCommand) ([32]byte, error) {
	return digestJSON(struct {
		PackageID       int64 `json:"package_id"`
		ExpectedVersion int64 `json:"expected_version"`
	}{input.PackageID, input.ExpectedVersion})
}

func (service *Service) executeMutation(
	ctx context.Context,
	operation ReceiptOperation,
	actor Actor,
	idempotencyKey string,
	payloadDigest [32]byte,
	apply func(context.Context, time.Time) (any, *LocalEvent, error),
) (json.RawMessage, error) {
	if !service.ready(ctx) || apply == nil {
		return nil, ErrUnavailable
	}
	now := service.now().UTC()
	if now.IsZero() {
		return nil, ErrUnavailable
	}
	reservation := ReceiptReservation{
		Operation: operation, ActorID: actor.AdminUserID, KeyDigest: sha256.Sum256([]byte(idempotencyKey)),
		PayloadDigest: payloadDigest, CreatedAt: now,
	}
	var result json.RawMessage
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, owned, reserveErr := service.repo.ReserveReceipt(txCtx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !validReceipt(receipt, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return ErrIdempotencyConflict
		}
		if owned && (receipt.State != "in_progress" || len(receipt.ResultJSON) != 0) {
			return ErrUnavailable
		}
		if !owned {
			if receipt.State != "completed" || len(receipt.ResultJSON) == 0 || !json.Valid(receipt.ResultJSON) {
				return ErrUnavailable
			}
			result = append(json.RawMessage(nil), receipt.ResultJSON...)
			return nil
		}
		value, event, applyErr := apply(txCtx, now)
		if applyErr != nil {
			return applyErr
		}
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil || len(encoded) == 0 || !json.Valid(encoded) {
			return errors.Join(ErrUnavailable, marshalErr)
		}
		if event != nil {
			if appendErr := service.events.Append(txCtx, *event); appendErr != nil {
				return appendErr
			}
		}
		completed, completeErr := service.repo.CompleteReceipt(txCtx, receipt.ID, encoded, now)
		if completeErr != nil {
			return completeErr
		}
		if !validReceipt(completed, reservation) || completed.State != "completed" || completed.ID != receipt.ID ||
			subtle.ConstantTimeCompare(completed.PayloadDigest[:], payloadDigest[:]) != 1 || !equalJSON(completed.ResultJSON, encoded) {
			return ErrUnavailable
		}
		result = append(json.RawMessage(nil), encoded...)
		return nil
	})
	if err != nil {
		return nil, classifyServiceError(err)
	}
	return result, nil
}

func equalJSON(left, right []byte) bool {
	if !json.Valid(left) || !json.Valid(right) {
		return false
	}
	var leftValue any
	var rightValue any
	leftDecoder := json.NewDecoder(bytes.NewReader(left))
	rightDecoder := json.NewDecoder(bytes.NewReader(right))
	leftDecoder.UseNumber()
	rightDecoder.UseNumber()
	if leftDecoder.Decode(&leftValue) != nil || rightDecoder.Decode(&rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func validReceipt(receipt Receipt, wanted ReceiptReservation) bool {
	return receipt.ID > 0 && receipt.Operation == wanted.Operation && receipt.ActorID == wanted.ActorID &&
		receipt.KeyDigest == wanted.KeyDigest && (receipt.State == "in_progress" || receipt.State == "completed")
}

func mutationEvent(eventType string, resourceID int64, actor Actor, key string, now time.Time) (*LocalEvent, error) {
	if eventType == "" || resourceID <= 0 || actor.AdminUserID <= 0 || now.IsZero() {
		return nil, ErrUnavailable
	}
	payload, err := json.Marshal(struct {
		ResourceID int64 `json:"resource_id"`
		ActorID    int64 `json:"actor_id"`
	}{resourceID, actor.AdminUserID})
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", eventType, actor.AdminUserID, key)))
	return &LocalEvent{
		Type: eventType, Payload: payload, OccurredAt: now,
		IdempotencyKey: "ai-audience:" + hex.EncodeToString(digest[:]),
	}, nil
}

func decodeMutation[T any](raw json.RawMessage) (T, error) {
	var value T
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return value, ErrUnavailable
	}
	return value, nil
}

func (service *Service) ready(ctx context.Context) bool {
	return ctx != nil && service != nil && !nilInterface(service.uow) && !nilInterface(service.repo) &&
		!nilInterface(service.segments) && !nilInterface(service.events) && service.now != nil
}

func classifyServiceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict),
		errors.Is(err, ErrVersionConflict), errors.Is(err, ErrIdempotencyConflict),
		errors.Is(err, ErrGroupNotEmpty), errors.Is(err, ErrArchived):
		return err
	default:
		return errors.Join(ErrUnavailable, err)
	}
}
