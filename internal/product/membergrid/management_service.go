package membergrid

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	mutationOperationCreate = "create"
	mutationOperationUpdate = "update"

	snapshotViewCreated         = "member_view.created"
	snapshotViewUpdated         = "member_view.updated"
	snapshotViewDeleted         = "member_view.deleted"
	snapshotCollaboratorCreated = "member_grid_collaborator.created"
	snapshotCollaboratorUpdated = "member_grid_collaborator.updated"
	snapshotCollaboratorDeleted = "member_grid_collaborator.deleted"
	snapshotExternalShareSet    = "member_grid.external_share.set"
)

type ManagementService struct {
	uow            platformport.UnitOfWork
	store          ManagementStore
	events         eventport.Appender
	externalShares ExternalShareStore
	shareIDs       ExternalShareIDFactory
	shareTokens    *ExternalShareTokenCodec
	now            func() time.Time
}

var _ ManagementApplication = (*ManagementService)(nil)

func NewManagementService(uow platformport.UnitOfWork, store ManagementStore, events eventport.Appender) (*ManagementService, error) {
	if nilDependency(uow) || nilDependency(store) || nilDependency(events) {
		return nil, errors.New("member grid management dependencies are required")
	}
	return &ManagementService{uow: uow, store: store, events: events, now: time.Now}, nil
}

// NewManagementServiceWithExternalShares keeps external sharing opt-in. The
// legacy constructor deliberately remains fail-closed until the caller has
// supplied the state store, fresh-ID factory, and token codec together.
func NewManagementServiceWithExternalShares(uow platformport.UnitOfWork, store ManagementStore, events eventport.Appender, externalShares ExternalShareStore, shareIDs ExternalShareIDFactory, shareTokens *ExternalShareTokenCodec) (*ManagementService, error) {
	service, err := NewManagementService(uow, store, events)
	if err != nil || nilDependency(externalShares) || nilDependency(shareIDs) || shareTokens == nil {
		return nil, errors.New("member grid external share dependencies are required")
	}
	service.externalShares = externalShares
	service.shareIDs = shareIDs
	service.shareTokens = shareTokens
	return service, nil
}

func (service *ManagementService) ShareSettings(ctx context.Context, serviceProductID int64) (ShareSettingsResponse, error) {
	if !managementReady(service) {
		return ShareSettingsResponse{}, ErrUnavailable
	}
	if ctx == nil || serviceProductID < 1 {
		return ShareSettingsResponse{}, ErrInvalidProductID
	}
	if err := ctx.Err(); err != nil {
		return ShareSettingsResponse{}, errors.Join(ErrUnavailable, err)
	}

	response := ShareSettingsResponse{
		ServiceProductID:                        serviceProductID,
		SavedViews:                              make([]SavedView, 0),
		Collaborators:                           make([]Collaborator, 0),
		ExternalShareSupported:                  false,
		ExternalShareEnabled:                    false,
		ExternalShareVersion:                    0,
		RealExternalCallExecuted:                false,
		CollaboratorEditIsLocalMetadataOnly:     true,
		CollaboratorEditGrantsCentralPermission: false,
	}
	var exists bool
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		exists, storeErr = service.store.ProductExists(txCtx, serviceProductID)
		if storeErr != nil || !exists {
			return storeErr
		}
		response.SavedViews, storeErr = service.store.ListSavedViews(txCtx, serviceProductID)
		if storeErr != nil {
			return storeErr
		}
		response.Collaborators, storeErr = service.store.ListCollaborators(txCtx, serviceProductID)
		if storeErr != nil || !externalShareManagementReady(service) {
			return storeErr
		}
		share, shareErr := service.externalShares.CurrentExternalShare(txCtx, serviceProductID)
		if shareErr != nil || !validExternalShare(share) || share.ServiceProductID != serviceProductID {
			return errors.Join(ErrUnavailable, shareErr)
		}
		response.ExternalShareSupported = true
		response.ExternalShareEnabled = share.Enabled
		response.ExternalShareVersion = share.Version
		return nil
	})
	if err != nil {
		return ShareSettingsResponse{}, classifyManagementError(err)
	}
	if !exists {
		return ShareSettingsResponse{}, ErrNotFound
	}
	if !validSavedViewList(response.SavedViews, serviceProductID) || !validCollaboratorList(response.Collaborators, serviceProductID) {
		return ShareSettingsResponse{}, ErrUnavailable
	}
	for index := range response.SavedViews {
		response.SavedViews[index] = cloneSavedView(response.SavedViews[index])
	}
	for index := range response.Collaborators {
		response.Collaborators[index] = cloneCollaborator(response.Collaborators[index])
	}
	return response, nil
}

// SetExternalShare persists only revocable share state. The raw token is
// derived after the non-secret mutation snapshot has been committed, allowing
// an idempotency replay of an enable transition without storing a bearer token
// in product_operation_receipts.
func (service *ManagementService) SetExternalShare(ctx context.Context, command SetExternalShareCommand) (SetExternalShareResult, error) {
	if !externalShareManagementReady(service) {
		return SetExternalShareResult{}, ErrUnavailable
	}
	normalized, payload, err := normalizeSetExternalShare(command)
	if err != nil {
		return SetExternalShareResult{}, err
	}
	snapshot, err := service.executeMutation(ctx, snapshotExternalShareSet, mutationOperationUpdate, normalized.ActorID, normalized.IdempotencyKey, payload, func(txCtx context.Context, _ time.Time) (mutationSnapshot, error) {
		if err := service.requireManagedProduct(txCtx, normalized.ServiceProductID); err != nil {
			return mutationSnapshot{}, err
		}
		current, currentErr := service.externalShares.CurrentExternalShare(txCtx, normalized.ServiceProductID)
		if currentErr != nil {
			return mutationSnapshot{}, currentErr
		}
		if !validExternalShare(current) || current.ServiceProductID != normalized.ServiceProductID {
			return mutationSnapshot{}, ErrUnavailable
		}
		if current.Version != normalized.ExpectedVersion {
			return mutationSnapshot{}, ErrConflict
		}
		if current.Enabled == normalized.Enabled {
			stable := cloneExternalShare(current)
			return mutationSnapshot{Kind: snapshotExternalShareSet, Status: http.StatusOK, ExternalShare: &stable}, nil
		}

		record := SetExternalShareRecord{
			ServiceProductID: normalized.ServiceProductID,
			Enabled:          normalized.Enabled,
			ExpectedVersion:  normalized.ExpectedVersion,
			ActorID:          normalized.ActorID,
			IdempotencyKey:   normalized.IdempotencyKey,
		}
		if normalized.Enabled {
			record.ShareID, currentErr = service.shareIDs.NewExternalShareID(txCtx)
			if currentErr != nil {
				return mutationSnapshot{}, currentErr
			}
			if !validExternalShareID(record.ShareID) {
				return mutationSnapshot{}, ErrUnavailable
			}
		}
		next, setErr := service.externalShares.SetExternalShare(txCtx, record)
		if setErr != nil {
			return mutationSnapshot{}, setErr
		}
		if !validExternalShare(next) || next.ServiceProductID != normalized.ServiceProductID || next.Enabled != normalized.Enabled ||
			next.ShareID != record.ShareID || next.Version != current.Version+1 {
			return mutationSnapshot{}, ErrUnavailable
		}
		next = cloneExternalShare(next)
		return mutationSnapshot{Kind: snapshotExternalShareSet, Status: http.StatusOK, ExternalShare: &next, TokenIssued: normalized.Enabled}, nil
	})
	if err != nil {
		return SetExternalShareResult{}, err
	}
	if snapshot.ExternalShare == nil || !validExternalShare(*snapshot.ExternalShare) {
		return SetExternalShareResult{}, ErrUnavailable
	}
	result := SetExternalShareResult{Share: cloneExternalShare(*snapshot.ExternalShare), TokenIssued: snapshot.TokenIssued}
	if result.TokenIssued {
		result.PublicToken, err = service.shareTokens.Issue(result.Share.ShareID)
		if err != nil {
			return SetExternalShareResult{}, ErrUnavailable
		}
	}
	return result, nil
}

func (service *ManagementService) CreateSavedView(ctx context.Context, command CreateSavedViewCommand) (SavedViewResponse, error) {
	normalized, payload, err := normalizeCreateSavedView(command)
	if err != nil {
		return SavedViewResponse{}, err
	}
	snapshot, err := service.executeMutation(ctx, snapshotViewCreated, mutationOperationCreate, normalized.ActorID, normalized.IdempotencyKey, payload, func(txCtx context.Context, now time.Time) (mutationSnapshot, error) {
		if err := service.requireManagedProduct(txCtx, normalized.ServiceProductID); err != nil {
			return mutationSnapshot{}, err
		}

		state := normalized.State
		sort := normalized.Sort
		columns := cloneColumnsSelection(normalized.Columns)
		if normalized.SourceViewID != nil {
			source, getErr := service.store.GetSavedViewForUpdate(txCtx, normalized.ServiceProductID, *normalized.SourceViewID)
			if getErr != nil {
				return mutationSnapshot{}, getErr
			}
			if !validSavedView(source) || source.ServiceProductID != normalized.ServiceProductID || source.ID != *normalized.SourceViewID {
				return mutationSnapshot{}, ErrUnavailable
			}
			state = source.State
			sort = source.Sort
			columns = cloneColumnsSelection(source.Columns)
		}

		created, createErr := service.store.CreateSavedView(txCtx, CreateSavedViewRecord{
			ServiceProductID: normalized.ServiceProductID,
			Name:             normalized.Name,
			State:            state,
			Sort:             sort,
			Columns:          columns,
			SourceViewID:     cloneOptionalID(normalized.SourceViewID),
			CreatedBy:        normalized.ActorID,
			CreatedAt:        now,
		})
		if createErr != nil {
			return mutationSnapshot{}, createErr
		}
		if !validSavedView(created) || created.ServiceProductID != normalized.ServiceProductID || created.Version != 1 ||
			created.Name != normalized.Name || created.State != state || created.Sort != sort ||
			!reflect.DeepEqual(created.Columns, columns) || !optionalIDEqual(created.SourceViewID, normalized.SourceViewID) ||
			created.CreatedBy != normalized.ActorID {
			return mutationSnapshot{}, ErrUnavailable
		}
		created = cloneSavedView(created)
		return mutationSnapshot{Kind: snapshotViewCreated, Status: http.StatusCreated, View: &created}, nil
	})
	if err != nil {
		return SavedViewResponse{}, err
	}
	if snapshot.View == nil || !createdViewMatchesCommand(*snapshot.View, normalized) {
		return SavedViewResponse{}, ErrUnavailable
	}
	return SavedViewResponse{OK: true, View: cloneSavedView(*snapshot.View)}, nil
}

func (service *ManagementService) UpdateSavedView(ctx context.Context, command UpdateSavedViewCommand) (SavedViewResponse, error) {
	if command.ViewID == 0 {
		return SavedViewResponse{}, ErrBuiltInView
	}
	normalized, payload, err := normalizeUpdateSavedView(command)
	if err != nil {
		return SavedViewResponse{}, err
	}
	snapshot, err := service.executeMutation(ctx, snapshotViewUpdated, mutationOperationUpdate, normalized.ActorID, normalized.IdempotencyKey, payload, func(txCtx context.Context, now time.Time) (mutationSnapshot, error) {
		if err := service.requireManagedProduct(txCtx, normalized.ServiceProductID); err != nil {
			return mutationSnapshot{}, err
		}
		current, getErr := service.store.GetSavedViewForUpdate(txCtx, normalized.ServiceProductID, normalized.ViewID)
		if getErr != nil {
			return mutationSnapshot{}, getErr
		}
		if !validSavedView(current) || current.ServiceProductID != normalized.ServiceProductID || current.ID != normalized.ViewID {
			return mutationSnapshot{}, ErrUnavailable
		}
		if current.Version != normalized.ExpectedVersion {
			return mutationSnapshot{}, ErrConflict
		}
		updated, updateErr := service.store.UpdateSavedView(txCtx, UpdateSavedViewRecord{
			ServiceProductID: normalized.ServiceProductID,
			ViewID:           normalized.ViewID,
			ExpectedVersion:  normalized.ExpectedVersion,
			Name:             normalized.Name,
			State:            normalized.State,
			Sort:             normalized.Sort,
			Columns:          cloneColumnsSelection(normalized.Columns),
			UpdatedAt:        now,
		})
		if updateErr != nil {
			return mutationSnapshot{}, updateErr
		}
		if !validSavedView(updated) || updated.ServiceProductID != current.ServiceProductID || updated.ID != current.ID ||
			updated.Version != current.Version+1 || updated.Name != normalized.Name || updated.State != normalized.State ||
			updated.Sort != normalized.Sort || !reflect.DeepEqual(updated.Columns, normalized.Columns) ||
			!optionalIDEqual(updated.SourceViewID, current.SourceViewID) || updated.CreatedBy != current.CreatedBy ||
			!updated.CreatedAt.Equal(current.CreatedAt) {
			return mutationSnapshot{}, ErrUnavailable
		}
		updated = cloneSavedView(updated)
		return mutationSnapshot{Kind: snapshotViewUpdated, Status: http.StatusOK, View: &updated}, nil
	})
	if err != nil {
		return SavedViewResponse{}, err
	}
	if snapshot.View == nil || !updatedViewMatchesCommand(*snapshot.View, normalized) {
		return SavedViewResponse{}, ErrUnavailable
	}
	return SavedViewResponse{OK: true, View: cloneSavedView(*snapshot.View)}, nil
}

func (service *ManagementService) DeleteSavedView(ctx context.Context, command DeleteSavedViewCommand) (DeleteSavedViewResponse, error) {
	if command.ViewID == 0 {
		return DeleteSavedViewResponse{}, ErrBuiltInView
	}
	normalized, payload, err := normalizeDeleteSavedView(command)
	if err != nil {
		return DeleteSavedViewResponse{}, err
	}
	snapshot, err := service.executeMutation(ctx, snapshotViewDeleted, mutationOperationUpdate, normalized.ActorID, normalized.IdempotencyKey, payload, func(txCtx context.Context, _ time.Time) (mutationSnapshot, error) {
		if err := service.requireManagedProduct(txCtx, normalized.ServiceProductID); err != nil {
			return mutationSnapshot{}, err
		}
		current, getErr := service.store.GetSavedViewForUpdate(txCtx, normalized.ServiceProductID, normalized.ViewID)
		if getErr != nil {
			return mutationSnapshot{}, getErr
		}
		if !validSavedView(current) || current.ServiceProductID != normalized.ServiceProductID || current.ID != normalized.ViewID {
			return mutationSnapshot{}, ErrUnavailable
		}
		if current.Version != normalized.ExpectedVersion {
			return mutationSnapshot{}, ErrConflict
		}
		deleted, deleteErr := service.store.DeleteSavedView(txCtx, normalized.ServiceProductID, normalized.ViewID, normalized.ExpectedVersion)
		if deleteErr != nil {
			return mutationSnapshot{}, deleteErr
		}
		if !reflect.DeepEqual(cloneSavedView(deleted), cloneSavedView(current)) {
			return mutationSnapshot{}, ErrUnavailable
		}
		deleted = cloneSavedView(deleted)
		return mutationSnapshot{Kind: snapshotViewDeleted, Status: http.StatusOK, View: &deleted, Deleted: true}, nil
	})
	if err != nil {
		return DeleteSavedViewResponse{}, err
	}
	if snapshot.View == nil || !deletedViewMatchesCommand(*snapshot.View, normalized) {
		return DeleteSavedViewResponse{}, ErrUnavailable
	}
	return DeleteSavedViewResponse{OK: true, Deleted: true, View: cloneSavedView(*snapshot.View)}, nil
}

func (service *ManagementService) CreateCollaborator(ctx context.Context, command CreateCollaboratorCommand) (CollaboratorResponse, error) {
	normalized, payload, err := normalizeCreateCollaborator(command)
	if err != nil {
		return CollaboratorResponse{}, err
	}
	snapshot, err := service.executeMutation(ctx, snapshotCollaboratorCreated, mutationOperationCreate, normalized.ActorID, normalized.IdempotencyKey, payload, func(txCtx context.Context, now time.Time) (mutationSnapshot, error) {
		if err := service.requireManagedProduct(txCtx, normalized.ServiceProductID); err != nil {
			return mutationSnapshot{}, err
		}
		active, activeErr := service.store.ActiveStaffExists(txCtx, normalized.StaffID)
		if activeErr != nil {
			return mutationSnapshot{}, activeErr
		}
		if !active {
			return mutationSnapshot{}, ErrInactiveStaff
		}
		created, createErr := service.store.CreateCollaborator(txCtx, CreateCollaboratorRecord{
			ServiceProductID: normalized.ServiceProductID,
			StaffID:          normalized.StaffID,
			Permission:       normalized.Permission,
			InvitedBy:        normalized.ActorID,
			CreatedAt:        now,
		})
		if createErr != nil {
			return mutationSnapshot{}, createErr
		}
		if !validCollaborator(created) || created.ServiceProductID != normalized.ServiceProductID || created.StaffID != normalized.StaffID ||
			created.Permission != normalized.Permission || created.InvitedBy != normalized.ActorID || created.Version != 1 {
			return mutationSnapshot{}, ErrUnavailable
		}
		created = cloneCollaborator(created)
		return mutationSnapshot{Kind: snapshotCollaboratorCreated, Status: http.StatusCreated, Collaborator: &created}, nil
	})
	if err != nil {
		return CollaboratorResponse{}, err
	}
	if snapshot.Collaborator == nil || !createdCollaboratorMatchesCommand(*snapshot.Collaborator, normalized) {
		return CollaboratorResponse{}, ErrUnavailable
	}
	return collaboratorResponse(*snapshot.Collaborator), nil
}

func (service *ManagementService) UpdateCollaborator(ctx context.Context, command UpdateCollaboratorCommand) (CollaboratorResponse, error) {
	normalized, payload, err := normalizeUpdateCollaborator(command)
	if err != nil {
		return CollaboratorResponse{}, err
	}
	snapshot, err := service.executeMutation(ctx, snapshotCollaboratorUpdated, mutationOperationUpdate, normalized.ActorID, normalized.IdempotencyKey, payload, func(txCtx context.Context, now time.Time) (mutationSnapshot, error) {
		if err := service.requireManagedProduct(txCtx, normalized.ServiceProductID); err != nil {
			return mutationSnapshot{}, err
		}
		current, getErr := service.store.GetCollaboratorForUpdate(txCtx, normalized.ServiceProductID, normalized.CollaboratorID)
		if getErr != nil {
			return mutationSnapshot{}, getErr
		}
		if !validCollaborator(current) || current.ServiceProductID != normalized.ServiceProductID || current.ID != normalized.CollaboratorID {
			return mutationSnapshot{}, ErrUnavailable
		}
		if current.Version != normalized.ExpectedVersion {
			return mutationSnapshot{}, ErrConflict
		}
		active, activeErr := service.store.ActiveStaffExists(txCtx, current.StaffID)
		if activeErr != nil {
			return mutationSnapshot{}, activeErr
		}
		if !active {
			return mutationSnapshot{}, ErrInactiveStaff
		}
		updated, updateErr := service.store.UpdateCollaborator(txCtx, UpdateCollaboratorRecord{
			ServiceProductID: normalized.ServiceProductID,
			CollaboratorID:   normalized.CollaboratorID,
			ExpectedVersion:  normalized.ExpectedVersion,
			Permission:       normalized.Permission,
			UpdatedAt:        now,
		})
		if updateErr != nil {
			return mutationSnapshot{}, updateErr
		}
		if !validCollaborator(updated) || updated.ServiceProductID != current.ServiceProductID || updated.ID != current.ID ||
			updated.Version != current.Version+1 || updated.Permission != normalized.Permission || updated.StaffID != current.StaffID ||
			updated.InvitedBy != current.InvitedBy || !updated.CreatedAt.Equal(current.CreatedAt) {
			return mutationSnapshot{}, ErrUnavailable
		}
		updated = cloneCollaborator(updated)
		return mutationSnapshot{Kind: snapshotCollaboratorUpdated, Status: http.StatusOK, Collaborator: &updated}, nil
	})
	if err != nil {
		return CollaboratorResponse{}, err
	}
	if snapshot.Collaborator == nil || !updatedCollaboratorMatchesCommand(*snapshot.Collaborator, normalized) {
		return CollaboratorResponse{}, ErrUnavailable
	}
	return collaboratorResponse(*snapshot.Collaborator), nil
}

func (service *ManagementService) DeleteCollaborator(ctx context.Context, command DeleteCollaboratorCommand) (DeleteCollaboratorResponse, error) {
	normalized, payload, err := normalizeDeleteCollaborator(command)
	if err != nil {
		return DeleteCollaboratorResponse{}, err
	}
	snapshot, err := service.executeMutation(ctx, snapshotCollaboratorDeleted, mutationOperationUpdate, normalized.ActorID, normalized.IdempotencyKey, payload, func(txCtx context.Context, _ time.Time) (mutationSnapshot, error) {
		if err := service.requireManagedProduct(txCtx, normalized.ServiceProductID); err != nil {
			return mutationSnapshot{}, err
		}
		current, getErr := service.store.GetCollaboratorForUpdate(txCtx, normalized.ServiceProductID, normalized.CollaboratorID)
		if getErr != nil {
			return mutationSnapshot{}, getErr
		}
		if !validCollaborator(current) || current.ServiceProductID != normalized.ServiceProductID || current.ID != normalized.CollaboratorID {
			return mutationSnapshot{}, ErrUnavailable
		}
		if current.Version != normalized.ExpectedVersion {
			return mutationSnapshot{}, ErrConflict
		}
		deleted, deleteErr := service.store.DeleteCollaborator(txCtx, normalized.ServiceProductID, normalized.CollaboratorID, normalized.ExpectedVersion)
		if deleteErr != nil {
			return mutationSnapshot{}, deleteErr
		}
		if !reflect.DeepEqual(cloneCollaborator(deleted), cloneCollaborator(current)) {
			return mutationSnapshot{}, ErrUnavailable
		}
		deleted = cloneCollaborator(deleted)
		return mutationSnapshot{Kind: snapshotCollaboratorDeleted, Status: http.StatusOK, Collaborator: &deleted, Deleted: true}, nil
	})
	if err != nil {
		return DeleteCollaboratorResponse{}, err
	}
	if snapshot.Collaborator == nil || !deletedCollaboratorMatchesCommand(*snapshot.Collaborator, normalized) {
		return DeleteCollaboratorResponse{}, ErrUnavailable
	}
	return DeleteCollaboratorResponse{
		OK: true, Deleted: true, Collaborator: cloneCollaborator(*snapshot.Collaborator),
		EditPermissionIsLocalMetadataOnly: true, GrantsCentralProductsPermission: false,
	}, nil
}

func (service *ManagementService) executeMutation(
	ctx context.Context,
	kind string,
	operation string,
	actorID int64,
	idempotencyKey string,
	payload []byte,
	mutate func(context.Context, time.Time) (mutationSnapshot, error),
) (mutationSnapshot, error) {
	if !managementReady(service) || mutate == nil {
		return mutationSnapshot{}, ErrUnavailable
	}
	if ctx == nil || actorID < 1 || !validIdempotencyKey(idempotencyKey) || len(payload) == 0 || !json.Valid(payload) {
		return mutationSnapshot{}, ErrInvalidManagementInput
	}
	if err := ctx.Err(); err != nil {
		return mutationSnapshot{}, errors.Join(ErrUnavailable, err)
	}
	now := service.now().UTC()
	if now.IsZero() {
		return mutationSnapshot{}, ErrUnavailable
	}
	reservation := MutationReceiptReservation{
		Operation:     operation,
		ActorScope:    fmt.Sprintf("membergrid:%s:actor:%d", kind, actorID),
		KeyDigest:     sha256.Sum256([]byte(idempotencyKey)),
		PayloadDigest: sha256.Sum256(payload),
		CreatedAt:     now,
	}

	var result mutationSnapshot
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveMutationReceipt(txCtx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !validMutationReceipt(receipt, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || len(receipt.ResultSnapshot) == 0 {
				return ErrUnavailable
			}
			var valid bool
			result, valid = decodeMutationSnapshot(receipt.ResultSnapshot, kind)
			if !valid {
				return ErrUnavailable
			}
			return nil
		}
		if receipt.State != "in_progress" || len(receipt.ResultSnapshot) != 0 {
			return ErrUnavailable
		}

		result, reserveErr = mutate(txCtx, now)
		if reserveErr != nil {
			return reserveErr
		}
		if !validMutationSnapshot(result, kind) {
			return ErrUnavailable
		}
		eventPayload, marshalErr := json.Marshal(struct {
			Kind    string           `json:"kind"`
			ActorID int64            `json:"actor_id"`
			Result  mutationSnapshot `json:"result"`
		}{Kind: "service_period_member_grid", ActorID: actorID, Result: result})
		if marshalErr != nil {
			return ErrUnavailable
		}
		_, appendErr := service.events.Append(txCtx, eventport.Event{
			Type:           eventport.EvProductUpdated,
			Payload:        eventPayload,
			OccurredAt:     now,
			IdempotencyKey: "product.member_grid." + kind + ":" + hex.EncodeToString(reservation.KeyDigest[:]),
		})
		if appendErr != nil {
			return appendErr
		}
		encoded, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return ErrUnavailable
		}
		completed, completeErr := service.store.CompleteMutationReceipt(txCtx, receipt.ID, encoded, now)
		if completeErr != nil {
			return completeErr
		}
		if !validMutationReceipt(completed, reservation) ||
			subtle.ConstantTimeCompare(completed.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 ||
			completed.State != "completed" || !jsonEquivalentSnapshot(completed.ResultSnapshot, encoded) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return mutationSnapshot{}, classifyManagementError(err)
	}
	return cloneMutationSnapshot(result), nil
}

func (service *ManagementService) requireManagedProduct(ctx context.Context, productID int64) error {
	exists, err := service.store.ProductExists(ctx, productID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func normalizeCreateSavedView(command CreateSavedViewCommand) (CreateSavedViewCommand, []byte, error) {
	command.Columns = cloneColumnsSelection(command.Columns)
	command.SourceViewID = cloneOptionalID(command.SourceViewID)
	if command.ServiceProductID < 1 || command.ExpectedVersion != 0 || !validViewName(command.Name) || command.ActorID < 1 || !validIdempotencyKey(command.IdempotencyKey) {
		return CreateSavedViewCommand{}, nil, ErrInvalidManagementInput
	}
	if command.SourceViewID == nil {
		if !command.State.validLegacySavedViewState() || !command.Sort.valid() || !validColumnSelection(command.Columns) {
			return CreateSavedViewCommand{}, nil, ErrInvalidManagementInput
		}
	} else if *command.SourceViewID < 1 || command.State != "" || command.Sort != "" || len(command.Columns) != 0 {
		return CreateSavedViewCommand{}, nil, ErrInvalidManagementInput
	}
	payload, err := json.Marshal(struct {
		Action           string      `json:"action"`
		ServiceProductID int64       `json:"service_product_id"`
		ExpectedVersion  int64       `json:"expected_version"`
		Name             string      `json:"name"`
		State            StateFilter `json:"state,omitempty"`
		Sort             ViewSort    `json:"sort,omitempty"`
		Columns          []string    `json:"columns,omitempty"`
		SourceViewID     *int64      `json:"source_view_id,omitempty"`
	}{"create_saved_view", command.ServiceProductID, command.ExpectedVersion, command.Name, command.State, command.Sort, command.Columns, command.SourceViewID})
	if err != nil {
		return CreateSavedViewCommand{}, nil, ErrUnavailable
	}
	return command, payload, nil
}

func normalizeUpdateSavedView(command UpdateSavedViewCommand) (UpdateSavedViewCommand, []byte, error) {
	command.Columns = cloneColumnsSelection(command.Columns)
	if command.ServiceProductID < 1 || command.ViewID < 1 || command.ExpectedVersion < 1 || !validViewName(command.Name) ||
		!command.State.validLegacySavedViewState() || !command.Sort.valid() || !validColumnSelection(command.Columns) || command.ActorID < 1 ||
		!validIdempotencyKey(command.IdempotencyKey) {
		return UpdateSavedViewCommand{}, nil, ErrInvalidManagementInput
	}
	payload, err := json.Marshal(struct {
		Action           string      `json:"action"`
		ServiceProductID int64       `json:"service_product_id"`
		ViewID           int64       `json:"view_id"`
		ExpectedVersion  int64       `json:"expected_version"`
		Name             string      `json:"name"`
		State            StateFilter `json:"state"`
		Sort             ViewSort    `json:"sort"`
		Columns          []string    `json:"columns"`
	}{"update_saved_view", command.ServiceProductID, command.ViewID, command.ExpectedVersion, command.Name, command.State, command.Sort, command.Columns})
	if err != nil {
		return UpdateSavedViewCommand{}, nil, ErrUnavailable
	}
	return command, payload, nil
}

func normalizeDeleteSavedView(command DeleteSavedViewCommand) (DeleteSavedViewCommand, []byte, error) {
	if command.ServiceProductID < 1 || command.ViewID < 1 || command.ExpectedVersion < 1 || command.ActorID < 1 || !validIdempotencyKey(command.IdempotencyKey) {
		return DeleteSavedViewCommand{}, nil, ErrInvalidManagementInput
	}
	payload, err := json.Marshal(struct {
		Action           string `json:"action"`
		ServiceProductID int64  `json:"service_product_id"`
		ViewID           int64  `json:"view_id"`
		ExpectedVersion  int64  `json:"expected_version"`
	}{"delete_saved_view", command.ServiceProductID, command.ViewID, command.ExpectedVersion})
	if err != nil {
		return DeleteSavedViewCommand{}, nil, ErrUnavailable
	}
	return command, payload, nil
}

func normalizeCreateCollaborator(command CreateCollaboratorCommand) (CreateCollaboratorCommand, []byte, error) {
	if command.ServiceProductID < 1 || command.ExpectedVersion != 0 || command.StaffID < 1 || !command.Permission.valid() ||
		command.ActorID < 1 || !validIdempotencyKey(command.IdempotencyKey) {
		return CreateCollaboratorCommand{}, nil, ErrInvalidManagementInput
	}
	payload, err := json.Marshal(struct {
		Action           string                 `json:"action"`
		ServiceProductID int64                  `json:"service_product_id"`
		ExpectedVersion  int64                  `json:"expected_version"`
		StaffID          int64                  `json:"staff_id"`
		Permission       CollaboratorPermission `json:"permission"`
	}{"create_collaborator", command.ServiceProductID, command.ExpectedVersion, command.StaffID, command.Permission})
	if err != nil {
		return CreateCollaboratorCommand{}, nil, ErrUnavailable
	}
	return command, payload, nil
}

func normalizeUpdateCollaborator(command UpdateCollaboratorCommand) (UpdateCollaboratorCommand, []byte, error) {
	if command.ServiceProductID < 1 || command.CollaboratorID < 1 || command.ExpectedVersion < 1 || !command.Permission.valid() ||
		command.ActorID < 1 || !validIdempotencyKey(command.IdempotencyKey) {
		return UpdateCollaboratorCommand{}, nil, ErrInvalidManagementInput
	}
	payload, err := json.Marshal(struct {
		Action           string                 `json:"action"`
		ServiceProductID int64                  `json:"service_product_id"`
		CollaboratorID   int64                  `json:"collaborator_id"`
		ExpectedVersion  int64                  `json:"expected_version"`
		Permission       CollaboratorPermission `json:"permission"`
	}{"update_collaborator", command.ServiceProductID, command.CollaboratorID, command.ExpectedVersion, command.Permission})
	if err != nil {
		return UpdateCollaboratorCommand{}, nil, ErrUnavailable
	}
	return command, payload, nil
}

func normalizeDeleteCollaborator(command DeleteCollaboratorCommand) (DeleteCollaboratorCommand, []byte, error) {
	if command.ServiceProductID < 1 || command.CollaboratorID < 1 || command.ExpectedVersion < 1 || command.ActorID < 1 ||
		!validIdempotencyKey(command.IdempotencyKey) {
		return DeleteCollaboratorCommand{}, nil, ErrInvalidManagementInput
	}
	payload, err := json.Marshal(struct {
		Action           string `json:"action"`
		ServiceProductID int64  `json:"service_product_id"`
		CollaboratorID   int64  `json:"collaborator_id"`
		ExpectedVersion  int64  `json:"expected_version"`
	}{"delete_collaborator", command.ServiceProductID, command.CollaboratorID, command.ExpectedVersion})
	if err != nil {
		return DeleteCollaboratorCommand{}, nil, ErrUnavailable
	}
	return command, payload, nil
}

func normalizeSetExternalShare(command SetExternalShareCommand) (SetExternalShareCommand, []byte, error) {
	if !validSetExternalShareCommand(command) {
		return SetExternalShareCommand{}, nil, ErrInvalidManagementInput
	}
	payload, err := json.Marshal(struct {
		Action           string `json:"action"`
		ServiceProductID int64  `json:"service_product_id"`
		Enabled          bool   `json:"enabled"`
		ExpectedVersion  int64  `json:"expected_version"`
	}{"set_external_share", command.ServiceProductID, command.Enabled, command.ExpectedVersion})
	if err != nil {
		return SetExternalShareCommand{}, nil, ErrUnavailable
	}
	return command, payload, nil
}

func validSavedView(view SavedView) bool {
	if view.ID < 1 || view.ServiceProductID < 1 || !validViewName(view.Name) || !view.State.validLegacySavedViewState() || !view.Sort.valid() ||
		!validColumnSelection(view.Columns) || view.Version < 1 || view.CreatedBy < 1 || view.CreatedAt.IsZero() ||
		view.UpdatedAt.IsZero() || view.UpdatedAt.Before(view.CreatedAt) {
		return false
	}
	return view.SourceViewID == nil || *view.SourceViewID > 0
}

func validCollaborator(collaborator Collaborator) bool {
	return collaborator.ID > 0 && collaborator.ServiceProductID > 0 && collaborator.StaffID > 0 && collaborator.Permission.valid() &&
		collaborator.Version > 0 && collaborator.InvitedBy > 0 && !collaborator.CreatedAt.IsZero() && !collaborator.UpdatedAt.IsZero() &&
		!collaborator.UpdatedAt.Before(collaborator.CreatedAt)
}

func validSavedViewList(views []SavedView, productID int64) bool {
	var previous int64
	for _, view := range views {
		if !validSavedView(view) || view.ServiceProductID != productID || view.ID <= previous {
			return false
		}
		previous = view.ID
	}
	return true
}

func validCollaboratorList(collaborators []Collaborator, productID int64) bool {
	var previous int64
	seenStaff := make(map[int64]struct{}, len(collaborators))
	for _, collaborator := range collaborators {
		if !validCollaborator(collaborator) || collaborator.ServiceProductID != productID || collaborator.ID <= previous {
			return false
		}
		if _, duplicate := seenStaff[collaborator.StaffID]; duplicate {
			return false
		}
		seenStaff[collaborator.StaffID] = struct{}{}
		previous = collaborator.ID
	}
	return true
}

func validMutationReceipt(receipt MutationReceipt, reservation MutationReceiptReservation) bool {
	return receipt.ID > 0 && receipt.Operation == reservation.Operation && receipt.ActorScope == reservation.ActorScope &&
		subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 &&
		(receipt.State == "in_progress" || receipt.State == "completed")
}

func validMutationSnapshot(snapshot mutationSnapshot, kind string) bool {
	if snapshot.Kind != kind {
		return false
	}
	switch kind {
	case snapshotViewCreated:
		return snapshot.Status == http.StatusCreated && snapshot.View != nil && snapshot.Collaborator == nil && snapshot.ExternalShare == nil && !snapshot.TokenIssued && !snapshot.Deleted && validSavedView(*snapshot.View)
	case snapshotViewUpdated:
		return snapshot.Status == http.StatusOK && snapshot.View != nil && snapshot.Collaborator == nil && snapshot.ExternalShare == nil && !snapshot.TokenIssued && !snapshot.Deleted && validSavedView(*snapshot.View)
	case snapshotViewDeleted:
		return snapshot.Status == http.StatusOK && snapshot.View != nil && snapshot.Collaborator == nil && snapshot.ExternalShare == nil && !snapshot.TokenIssued && snapshot.Deleted && validSavedView(*snapshot.View)
	case snapshotCollaboratorCreated:
		return snapshot.Status == http.StatusCreated && snapshot.Collaborator != nil && snapshot.View == nil && snapshot.ExternalShare == nil && !snapshot.TokenIssued && !snapshot.Deleted && validCollaborator(*snapshot.Collaborator)
	case snapshotCollaboratorUpdated:
		return snapshot.Status == http.StatusOK && snapshot.Collaborator != nil && snapshot.View == nil && snapshot.ExternalShare == nil && !snapshot.TokenIssued && !snapshot.Deleted && validCollaborator(*snapshot.Collaborator)
	case snapshotCollaboratorDeleted:
		return snapshot.Status == http.StatusOK && snapshot.Collaborator != nil && snapshot.View == nil && snapshot.ExternalShare == nil && !snapshot.TokenIssued && snapshot.Deleted && validCollaborator(*snapshot.Collaborator)
	case snapshotExternalShareSet:
		return snapshot.Status == http.StatusOK && snapshot.View == nil && snapshot.Collaborator == nil && snapshot.ExternalShare != nil && !snapshot.Deleted &&
			validExternalShare(*snapshot.ExternalShare) && (!snapshot.TokenIssued || snapshot.ExternalShare.Enabled)
	default:
		return false
	}
}

func decodeMutationSnapshot(raw json.RawMessage, kind string) (mutationSnapshot, bool) {
	if len(raw) == 0 || !json.Valid(raw) {
		return mutationSnapshot{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot mutationSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return mutationSnapshot{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || !validMutationSnapshot(snapshot, kind) {
		return mutationSnapshot{}, false
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil || !jsonEquivalentSnapshot(raw, encoded) {
		return mutationSnapshot{}, false
	}
	return cloneMutationSnapshot(snapshot), true
}

func createdViewMatchesCommand(view SavedView, command CreateSavedViewCommand) bool {
	if !validSavedView(view) || view.ServiceProductID != command.ServiceProductID || view.Name != command.Name ||
		view.Version != 1 || view.CreatedBy != command.ActorID || !optionalIDEqual(view.SourceViewID, command.SourceViewID) {
		return false
	}
	if command.SourceViewID != nil {
		return true
	}
	return view.State == command.State && view.Sort == command.Sort && reflect.DeepEqual(view.Columns, command.Columns)
}

func updatedViewMatchesCommand(view SavedView, command UpdateSavedViewCommand) bool {
	return validSavedView(view) && view.ServiceProductID == command.ServiceProductID && view.ID == command.ViewID &&
		view.Version == command.ExpectedVersion+1 && view.Name == command.Name && view.State == command.State &&
		view.Sort == command.Sort && reflect.DeepEqual(view.Columns, command.Columns)
}

func deletedViewMatchesCommand(view SavedView, command DeleteSavedViewCommand) bool {
	return validSavedView(view) && view.ServiceProductID == command.ServiceProductID && view.ID == command.ViewID &&
		view.Version == command.ExpectedVersion
}

func createdCollaboratorMatchesCommand(collaborator Collaborator, command CreateCollaboratorCommand) bool {
	return validCollaborator(collaborator) && collaborator.ServiceProductID == command.ServiceProductID &&
		collaborator.StaffID == command.StaffID && collaborator.Permission == command.Permission &&
		collaborator.Version == 1 && collaborator.InvitedBy == command.ActorID
}

func updatedCollaboratorMatchesCommand(collaborator Collaborator, command UpdateCollaboratorCommand) bool {
	return validCollaborator(collaborator) && collaborator.ServiceProductID == command.ServiceProductID &&
		collaborator.ID == command.CollaboratorID && collaborator.Version == command.ExpectedVersion+1 &&
		collaborator.Permission == command.Permission
}

func deletedCollaboratorMatchesCommand(collaborator Collaborator, command DeleteCollaboratorCommand) bool {
	return validCollaborator(collaborator) && collaborator.ServiceProductID == command.ServiceProductID &&
		collaborator.ID == command.CollaboratorID && collaborator.Version == command.ExpectedVersion
}

func cloneMutationSnapshot(snapshot mutationSnapshot) mutationSnapshot {
	if snapshot.View != nil {
		view := cloneSavedView(*snapshot.View)
		snapshot.View = &view
	}
	if snapshot.Collaborator != nil {
		collaborator := cloneCollaborator(*snapshot.Collaborator)
		snapshot.Collaborator = &collaborator
	}
	if snapshot.ExternalShare != nil {
		share := cloneExternalShare(*snapshot.ExternalShare)
		snapshot.ExternalShare = &share
	}
	return snapshot
}

func optionalIDEqual(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func collaboratorResponse(collaborator Collaborator) CollaboratorResponse {
	return CollaboratorResponse{
		OK: true, Collaborator: cloneCollaborator(collaborator), EditPermissionIsLocalMetadataOnly: true,
		GrantsCentralProductsPermission: false,
	}
}

func jsonEquivalentSnapshot(left, right []byte) bool {
	if !json.Valid(left) || !json.Valid(right) {
		return false
	}
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func classifyManagementError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidManagementInput), errors.Is(err, ErrInvalidProductID), errors.Is(err, ErrNotFound),
		errors.Is(err, ErrConflict), errors.Is(err, ErrBuiltInView), errors.Is(err, ErrInactiveStaff),
		errors.Is(err, ErrAuthenticationRequired), errors.Is(err, ErrPermissionDenied), errors.Is(err, ErrCSRFRejected):
		return err
	default:
		return errors.Join(ErrUnavailable, err)
	}
}

func managementReady(service *ManagementService) bool {
	return service != nil && !nilDependency(service.uow) && !nilDependency(service.store) && !nilDependency(service.events) && service.now != nil
}

func externalShareManagementReady(service *ManagementService) bool {
	return managementReady(service) && !nilDependency(service.externalShares) && !nilDependency(service.shareIDs) && service.shareTokens != nil
}
