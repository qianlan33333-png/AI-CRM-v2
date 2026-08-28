package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var archiveIdentityRepositoryPostgresDSN = flag.String("archive-identity-repository-postgres-dsn", "", "isolated PostgreSQL DSN for archive identity rollback verification")

func TestArchiveIdentityRepositoryRejectsInvalidInputAndMissingTransaction(t *testing.T) {
	repository := NewArchiveIdentityRepository()
	input := archiveIdentityInput()
	if _, err := repository.ImportArchiveWeComIdentity(context.Background(), input); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("missing transaction error=%v", err)
	}
	for name, change := range map[string]func(*identityport.ArchiveIdentityInput){
		"zero hmac": func(value *identityport.ArchiveIdentityInput) { value.SourceKeyHMAC = [32]byte{} },
		"zero fingerprint": func(value *identityport.ArchiveIdentityInput) {
			value.SourceKeyHMAC = [32]byte{}
			value.SourceKeyHMAC[31] = 1
		},
		"zero version":   func(value *identityport.ArchiveIdentityInput) { value.HMACKeyVersion = 0 },
		"invalid scope":  func(value *identityport.ArchiveIdentityInput) { value.Scope = "wecom-corp: bad" },
		"empty external": func(value *identityport.ArchiveIdentityInput) { value.ExternalUserID = "" },
		"zero customer": func(value *identityport.ArchiveIdentityInput) {
			customer := contactport.CustomerID(0)
			value.CustomerID = &customer
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := input
			change(&value)
			if _, err := repository.ImportArchiveWeComIdentity(context.Background(), value); !errors.Is(err, identityapp.ErrInvalidIdentity) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	if _, err := repository.ReadArchiveWeComIdentity(context.Background(), 1); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("missing read transaction error=%v", err)
	}
	if _, err := repository.ReadArchiveWeComIdentity(context.Background(), 0); !errors.Is(err, identityapp.ErrInvalidIdentity) {
		t.Fatalf("invalid ID error=%v", err)
	}
}

func TestArchiveIdentityFactRejectsNonHistoricalAndPreservesNullableBinding(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
	bound := at.Add(time.Minute)
	fingerprint := make([]byte, 16)
	fingerprint[0] = 9
	row := identitydb.Identity{ID: 7, CustomerID: pgtype.Int8{Int64: 42, Valid: true}, Kind: string(identityport.KindWeComExternalUserID), Scope: "wecom-corp:corp", NormalizedValue: "external", NormalizerVersion: identityapp.NormalizerVersion, Assurance: string(identityport.AssuranceDeclared), Source: "v1.archive_identity_gap", ReviewFingerprint: fingerprint, FingerprintKeyVersion: 2, CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, BoundAt: pgtype.Timestamptz{Time: bound, Valid: true}}
	fact, err := archiveIdentityFact(row)
	if err != nil || fact.CustomerID == nil || *fact.CustomerID != 42 || fact.Scope != "wecom-corp:corp" || fact.ExternalUserID != "external" || fact.ReviewFingerprint[0] != 9 || fact.CreatedAt != at.UTC().Truncate(time.Microsecond) || fact.BoundAt == nil || *fact.BoundAt != bound.UTC().Truncate(time.Microsecond) {
		t.Fatalf("fact=%+v error=%v", fact, err)
	}
	row.CustomerID, row.BoundAt = pgtype.Int8{}, pgtype.Timestamptz{}
	fact, err = archiveIdentityFact(row)
	if err != nil || fact.CustomerID != nil || fact.BoundAt != nil {
		t.Fatalf("unbound fact=%+v error=%v", fact, err)
	}
	for name, change := range map[string]func(*identitydb.Identity){
		"kind":           func(value *identitydb.Identity) { value.Kind = string(identityport.KindPhone) },
		"source":         func(value *identitydb.Identity) { value.Source = "identity.normalizer" },
		"assurance":      func(value *identitydb.Identity) { value.Assurance = string(identityport.AssuranceVerified) },
		"created":        func(value *identitydb.Identity) { value.CreatedAt = pgtype.Timestamptz{} },
		"bound mismatch": func(value *identitydb.Identity) { value.CustomerID = pgtype.Int8{Int64: 42, Valid: true} },
		"empty bound": func(value *identitydb.Identity) {
			value.CustomerID, value.BoundAt = pgtype.Int8{Int64: 42, Valid: true}, pgtype.Timestamptz{Valid: true}
		},
		"fingerprint": func(value *identitydb.Identity) { value.ReviewFingerprint = make([]byte, 16) },
	} {
		t.Run(name, func(t *testing.T) {
			value := row
			change(&value)
			if _, err := archiveIdentityFact(value); !errors.Is(err, identityport.ErrHistoricalScopedIdentityConflict) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestArchiveIdentityRepositoryPostgresRoundTripRollback(t *testing.T) {
	if *archiveIdentityRepositoryPostgresDSN == "" {
		t.Skip("set -archive-identity-repository-postgres-dsn for isolated rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *archiveIdentityRepositoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := NewArchiveIdentityRepository()
	rollback := errors.New("archive identity rollback")
	var unboundID int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		unbound, importErr := repository.ImportArchiveWeComIdentity(txCtx, archiveIdentityInput())
		if importErr != nil {
			return fmt.Errorf("archive-identity stage=unbound-import: %w", importErr)
		}
		unboundID = unbound.ID
		if loaded, readErr := repository.ReadArchiveWeComIdentity(txCtx, unbound.ID); readErr != nil || !reflect.DeepEqual(loaded, unbound) || loaded.CustomerID != nil || loaded.BoundAt != nil {
			return fmt.Errorf("archive-identity stage=unbound-read: %w", readErr)
		}
		if replay, replayErr := repository.ImportArchiveWeComIdentity(txCtx, archiveIdentityInput()); replayErr != nil || !reflect.DeepEqual(replay, unbound) {
			return fmt.Errorf("archive-identity stage=replay: %w", replayErr)
		}
		drift := archiveIdentityInput()
		drift.SourceKeyHMAC = archiveIdentityHMAC(2)
		if _, driftErr := repository.ImportArchiveWeComIdentity(txCtx, drift); !errors.Is(driftErr, identityport.ErrHistoricalScopedIdentityConflict) {
			return fmt.Errorf("archive-identity stage=drift: %w", driftErr)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	for _, id := range []int64{unboundID} {
		err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
			_, readErr := repository.ReadArchiveWeComIdentity(txCtx, id)
			return readErr
		})
		if !errors.Is(err, identityport.ErrHistoricalScopedIdentityConflict) {
			t.Fatalf("rollback left ID=%d error=%v", id, err)
		}
	}
}

func archiveIdentityInput() identityport.ArchiveIdentityInput {
	return identityport.ArchiveIdentityInput{Scope: "wecom-corp:corp", ExternalUserID: " external ", SourceKeyHMAC: archiveIdentityHMAC(1), HMACKeyVersion: 2}
}
func archiveIdentityHMAC(first byte) [32]byte { var value [32]byte; value[0] = first; return value }
