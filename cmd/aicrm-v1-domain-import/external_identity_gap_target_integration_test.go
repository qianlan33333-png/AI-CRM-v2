package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var externalIdentityGapTargetPostgresDSN = flag.String("external-identity-gap-target-postgres-dsn", "", "isolated PostgreSQL DSN for external identity gap target rollback verification")

func TestExternalIdentityGapTargetPostgresBoundRollback(t *testing.T) {
	if *externalIdentityGapTargetPostgresDSN == "" {
		t.Skip("set -external-identity-gap-target-postgres-dsn for isolated rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *externalIdentityGapTargetPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	archives := identitystore.NewArchiveIdentityRepository()
	identities := identitystore.NewRepository()
	contacts := contactstore.HistoricalImportRepository{}
	rollback := errors.New("external identity gap target rollback")
	var importedIDs []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		at := time.Date(2026, 8, 28, 8, 30, 0, 123456000, time.UTC)
		first, createErr := externalIdentityGapTargetCustomer(txCtx, contacts, "archive identity root one", at)
		if createErr != nil {
			return fmt.Errorf("external-identity-gap-target stage=create-first-customer: %w", createErr)
		}
		second, createErr := externalIdentityGapTargetCustomer(txCtx, contacts, "archive identity root two", at)
		if createErr != nil {
			return fmt.Errorf("external-identity-gap-target stage=create-second-customer: %w", createErr)
		}

		boundInput := externalIdentityGapArchiveInput(&first, "wecom-corp:archive-bound", "external-bound", 1)
		bound, importErr := archives.ImportArchiveWeComIdentity(txCtx, boundInput)
		if importErr != nil || bound.CustomerID == nil || *bound.CustomerID != first || bound.BoundAt == nil {
			return fmt.Errorf("external-identity-gap-target stage=bound-import: %w", importErr)
		}
		importedIDs = append(importedIDs, bound.ID)
		if loaded, readErr := archives.ReadArchiveWeComIdentity(txCtx, bound.ID); readErr != nil || !reflect.DeepEqual(loaded, bound) {
			return fmt.Errorf("external-identity-gap-target stage=bound-read: %w", readErr)
		}
		if replay, replayErr := archives.ImportArchiveWeComIdentity(txCtx, boundInput); replayErr != nil || !reflect.DeepEqual(replay, bound) {
			return fmt.Errorf("external-identity-gap-target stage=bound-replay: %w", replayErr)
		}

		unboundInput := externalIdentityGapArchiveInput(nil, "wecom-corp:archive-unbound", "external-unbound", 2)
		unbound, importErr := archives.ImportArchiveWeComIdentity(txCtx, unboundInput)
		if importErr != nil {
			return fmt.Errorf("external-identity-gap-target stage=unbound-import: %w", importErr)
		}
		importedIDs = append(importedIDs, unbound.ID)
		unboundInput.CustomerID = &first
		if _, conflictErr := archives.ImportArchiveWeComIdentity(txCtx, unboundInput); !errors.Is(conflictErr, identityport.ErrHistoricalScopedIdentityConflict) {
			return fmt.Errorf("external-identity-gap-target stage=null-to-bound-conflict: %w", conflictErr)
		}
		if loaded, readErr := archives.ReadArchiveWeComIdentity(txCtx, unbound.ID); readErr != nil || loaded.CustomerID != nil || loaded.BoundAt != nil {
			return fmt.Errorf("external-identity-gap-target stage=null-to-bound-preserved: %w", readErr)
		}

		collisionInput := externalIdentityGapArchiveInput(&first, "wecom-corp:archive-collision", "external-collision", 3)
		collision, importErr := archives.ImportArchiveWeComIdentity(txCtx, collisionInput)
		if importErr != nil {
			return fmt.Errorf("external-identity-gap-target stage=first-bound-import: %w", importErr)
		}
		importedIDs = append(importedIDs, collision.ID)
		collisionInput.CustomerID = &second
		if _, conflictErr := archives.ImportArchiveWeComIdentity(txCtx, collisionInput); !errors.Is(conflictErr, identityport.ErrHistoricalScopedIdentityConflict) {
			return fmt.Errorf("external-identity-gap-target stage=different-customer-conflict: %w", conflictErr)
		}
		if loaded, readErr := archives.ReadArchiveWeComIdentity(txCtx, collision.ID); readErr != nil || loaded.CustomerID == nil || *loaded.CustomerID != first {
			return fmt.Errorf("external-identity-gap-target stage=different-customer-preserved: %w", readErr)
		}

		if err := externalIdentityGapTargetRejectsOrdinaryFloating(txCtx, archives, identities); err != nil {
			return err
		}
		if err := externalIdentityGapTargetRejectsOrdinaryBound(txCtx, archives, identities, first); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	for _, id := range importedIDs {
		err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
			_, readErr := archives.ReadArchiveWeComIdentity(txCtx, id)
			return readErr
		})
		if !errors.Is(err, identityport.ErrHistoricalScopedIdentityConflict) {
			t.Fatalf("rollback left archive ID=%d error=%v", id, err)
		}
	}
}

func externalIdentityGapTargetCustomer(ctx context.Context, repository contactstore.HistoricalImportRepository, name string, at time.Time) (contactport.CustomerID, error) {
	id, err := repository.CreateCustomer(ctx, name, nil, nil, nil, at, at, at, at)
	if err != nil || id < 1 {
		return 0, err
	}
	return contactport.CustomerID(id), nil
}

func externalIdentityGapArchiveInput(customer *contactport.CustomerID, scope, external string, marker byte) identityport.ArchiveIdentityInput {
	var hmac [32]byte
	hmac[0] = marker
	return identityport.ArchiveIdentityInput{CustomerID: customer, Scope: scope, ExternalUserID: external, SourceKeyHMAC: hmac, HMACKeyVersion: 2}
}

func externalIdentityGapTargetRejectsOrdinaryFloating(ctx context.Context, archives *identitystore.ArchiveIdentityRepository, identities *identitystore.Repository) error {
	normalized, err := identityapp.Normalize(identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:ordinary-floating", Value: "ordinary-floating"})
	if err != nil {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-floating-normalize: %w", err)
	}
	id, created, err := identities.UpsertNormalized(ctx, normalized)
	if err != nil || !created || id < 1 {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-floating-create: %w", err)
	}
	if _, err = archives.ImportArchiveWeComIdentity(ctx, externalIdentityGapArchiveInput(nil, normalized.Scope, normalized.NormalizedValue, 4)); !errors.Is(err, identityport.ErrHistoricalScopedIdentityConflict) {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-floating-conflict: %w", err)
	}
	if replayID, replayCreated, replayErr := identities.UpsertNormalized(ctx, normalized); replayErr != nil || replayCreated || replayID != id {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-floating-preserved: %w", replayErr)
	}
	if _, err = archives.ReadArchiveWeComIdentity(ctx, id); !errors.Is(err, identityport.ErrHistoricalScopedIdentityConflict) {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-floating-not-history: %w", err)
	}
	return nil
}

func externalIdentityGapTargetRejectsOrdinaryBound(ctx context.Context, archives *identitystore.ArchiveIdentityRepository, identities *identitystore.Repository, customer contactport.CustomerID) error {
	normalized, err := identityapp.Normalize(identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:ordinary-bound", Value: "ordinary-bound"})
	if err != nil {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-bound-normalize: %w", err)
	}
	id, created, err := identities.UpsertNormalized(ctx, normalized)
	if err != nil || !created || id < 1 {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-bound-create: %w", err)
	}
	bound, err := identities.BindNormalized(ctx, normalized, int64(customer))
	if err != nil || bound.Status != identityport.BindBound || bound.IdentityID != id {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-bound-bind: %w", err)
	}
	if _, err = archives.ImportArchiveWeComIdentity(ctx, externalIdentityGapArchiveInput(&customer, normalized.Scope, normalized.NormalizedValue, 5)); !errors.Is(err, identityport.ErrHistoricalScopedIdentityConflict) {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-bound-conflict: %w", err)
	}
	if record, lookupErr := identities.LookupNormalized(ctx, normalized); lookupErr != nil || record.Conflict || record.CustomerID != int64(customer) {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-bound-preserved: %w", lookupErr)
	}
	if _, err = archives.ReadArchiveWeComIdentity(ctx, id); !errors.Is(err, identityport.ErrHistoricalScopedIdentityConflict) {
		return fmt.Errorf("external-identity-gap-target stage=ordinary-bound-not-history: %w", err)
	}
	return nil
}
