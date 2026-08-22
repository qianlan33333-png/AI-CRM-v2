package store

import (
	"context"
	"encoding/json"
	"time"

	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

var _ productapp.LocalProductLifecycleStore = (*CatalogRepository)(nil)

// localProductLifecycleUpdater is structural. The SQL source is checked in in
// this phase, while sqlc output stays centrally owned and is materialized only
// after the migration/route lane is released.
type localProductLifecycleUpdater interface {
	UpdateLocalProductLifecycle(context.Context, []byte) (int64, error)
}

type localProductSafeDeleter interface {
	DeleteLocalProductIfSafe(context.Context, []byte) (int64, error)
}

type localProductDeleteReferenceLocker interface {
	LockLocalProductDeleteReferences(context.Context) error
}

func (repository *CatalogRepository) UpdateLocalProductLifecycle(ctx context.Context, command productapp.LocalProductLifecycleStoreUpdate, now time.Time) (productport.Product, error) {
	if repository == nil || command.ID < 1 || command.ExpectedVersion < 1 || now.IsZero() || !json.Valid(command.LegacyAdminProjection) ||
		(command.LocalLifecycle != productport.LocalProductDraft && command.LocalLifecycle != productport.LocalProductDisabled && command.LocalLifecycle != productport.LocalProductEnabled) {
		return productport.Product{}, productapp.ErrInvalidProduct
	}
	querySet, err := queries(ctx)
	if err != nil {
		return productport.Product{}, unavailable(err)
	}
	updater, ok := any(querySet).(localProductLifecycleUpdater)
	if !ok {
		// The checked-in query source is the contract; the generated method is
		// intentionally left for Lane E/central generation.
		return productport.Product{}, productapp.ErrUnavailable
	}
	payload, err := json.Marshal(struct {
		ProductID             int64           `json:"product_id"`
		ExpectedVersion       int64           `json:"expected_version"`
		LocalLifecycle        string          `json:"local_lifecycle"`
		LegacyAdminProjection json.RawMessage `json:"legacy_admin_projection"`
		UpdatedAt             time.Time       `json:"updated_at"`
	}{int64(command.ID), command.ExpectedVersion, string(command.LocalLifecycle), command.LegacyAdminProjection, now})
	if err != nil {
		return productport.Product{}, productapp.ErrUnavailable
	}
	affected, err := updater.UpdateLocalProductLifecycle(ctx, payload)
	if err != nil {
		return productport.Product{}, unavailable(err)
	}
	if affected != 1 {
		return productport.Product{}, productapp.ErrConflict
	}
	updated, err := repository.Get(ctx, command.ID)
	if err != nil {
		return productport.Product{}, err
	}
	if updated.Version != command.ExpectedVersion+1 || !jsonEqual(updated.LegacyAdminProjection, command.LegacyAdminProjection) || updated.UpdatedAt.IsZero() {
		return productport.Product{}, productapp.ErrUnavailable
	}
	return updated, nil
}

func (repository *CatalogRepository) DeleteLocalProductIfSafe(ctx context.Context, id productport.ID, expectedVersion int64) (bool, error) {
	if repository == nil || id < 1 || expectedVersion < 1 {
		return false, productapp.ErrInvalidProduct
	}
	querySet, err := queries(ctx)
	if err != nil {
		return false, unavailable(err)
	}
	deleter, ok := any(querySet).(localProductSafeDeleter)
	if !ok {
		return false, productapp.ErrUnavailable
	}
	locker, ok := any(querySet).(localProductDeleteReferenceLocker)
	if !ok {
		return false, productapp.ErrUnavailable
	}
	if err := locker.LockLocalProductDeleteReferences(ctx); err != nil {
		return false, unavailable(err)
	}
	payload, err := json.Marshal(struct {
		ProductID       int64 `json:"product_id"`
		ExpectedVersion int64 `json:"expected_version"`
	}{int64(id), expectedVersion})
	if err != nil {
		return false, productapp.ErrUnavailable
	}
	deleted, err := deleter.DeleteLocalProductIfSafe(ctx, payload)
	if err != nil {
		return false, unavailable(err)
	}
	if deleted < 0 || deleted > 1 {
		return false, productapp.ErrUnavailable
	}
	return deleted == 1, nil
}
