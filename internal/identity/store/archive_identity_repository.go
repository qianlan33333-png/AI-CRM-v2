package store

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type ArchiveIdentityRepository struct{}

var _ identityport.ArchiveIdentityTarget = (*ArchiveIdentityRepository)(nil)

func NewArchiveIdentityRepository() *ArchiveIdentityRepository { return &ArchiveIdentityRepository{} }

func (repository *ArchiveIdentityRepository) ImportArchiveWeComIdentity(ctx context.Context, input identityport.ArchiveIdentityInput) (identityport.ArchiveIdentityFact, error) {
	if repository == nil || input.HMACKeyVersion <= 0 || emptyArchiveIdentityHMAC(input.SourceKeyHMAC) || emptyArchiveIdentityFingerprint(input.SourceKeyHMAC[:16]) || (input.CustomerID != nil && *input.CustomerID <= 0) {
		return identityport.ArchiveIdentityFact{}, identityapp.ErrInvalidIdentity
	}
	normalized, err := identityapp.Normalize(identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: input.Scope, Value: input.ExternalUserID})
	if err != nil || identityapp.ValidateNormalized(normalized) != nil {
		return identityport.ArchiveIdentityFact{}, identityapp.ErrInvalidIdentity
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityport.ArchiveIdentityFact{}, err
	}
	customerID := pgtype.Int8{}
	if input.CustomerID != nil {
		customerID = pgtype.Int8{Int64: int64(*input.CustomerID), Valid: true}
	}
	row, err := identitydb.New(tx).ImportArchiveWeComIdentity(ctx, identitydb.ImportArchiveWeComIdentityParams{
		CustomerID: customerID, Scope: normalized.Scope, ExternalUserid: normalized.NormalizedValue,
		SourceKeyHmac: input.SourceKeyHMAC[:], FingerprintKeyVersion: input.HMACKeyVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return identityport.ArchiveIdentityFact{}, identityport.ErrHistoricalScopedIdentityConflict
	}
	if err != nil {
		return identityport.ArchiveIdentityFact{}, err
	}
	fact, err := archiveIdentityFact(row)
	if err != nil || fact.Scope != normalized.Scope || fact.ExternalUserID != normalized.NormalizedValue || fact.HMACKeyVersion != input.HMACKeyVersion || !bytes.Equal(fact.ReviewFingerprint[:], input.SourceKeyHMAC[:16]) || !sameArchiveIdentityCustomer(fact.CustomerID, input.CustomerID) {
		return identityport.ArchiveIdentityFact{}, identityport.ErrHistoricalScopedIdentityConflict
	}
	return fact, nil
}

func (repository *ArchiveIdentityRepository) ReadArchiveWeComIdentity(ctx context.Context, id int64) (identityport.ArchiveIdentityFact, error) {
	if repository == nil || id <= 0 {
		return identityport.ArchiveIdentityFact{}, identityapp.ErrInvalidIdentity
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityport.ArchiveIdentityFact{}, err
	}
	row, err := identitydb.New(tx).ReadArchiveWeComIdentity(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return identityport.ArchiveIdentityFact{}, identityport.ErrHistoricalScopedIdentityConflict
	}
	if err != nil {
		return identityport.ArchiveIdentityFact{}, err
	}
	return archiveIdentityFact(row)
}

func archiveIdentityFact(row identitydb.Identity) (identityport.ArchiveIdentityFact, error) {
	if row.ID <= 0 || row.Kind != string(identityport.KindWeComExternalUserID) || row.Assurance != string(identityport.AssuranceDeclared) || row.Source != "v1.archive_identity_gap" || row.NormalizerVersion != identityapp.NormalizerVersion || len(row.ReviewFingerprint) != 16 || emptyArchiveIdentityFingerprint(row.ReviewFingerprint) || row.FingerprintKeyVersion <= 0 || !row.CreatedAt.Valid || row.CreatedAt.Time.IsZero() || (row.CustomerID.Valid != row.BoundAt.Valid) || (row.BoundAt.Valid && row.BoundAt.Time.IsZero()) || (row.CustomerID.Valid && row.CustomerID.Int64 <= 0) {
		return identityport.ArchiveIdentityFact{}, identityport.ErrHistoricalScopedIdentityConflict
	}
	normalized, err := identityapp.Normalize(identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: row.Scope, Value: row.NormalizedValue})
	if err != nil || identityapp.ValidateNormalized(normalized) != nil || normalized.Scope != row.Scope || normalized.NormalizedValue != row.NormalizedValue {
		return identityport.ArchiveIdentityFact{}, identityport.ErrHistoricalScopedIdentityConflict
	}
	fact := identityport.ArchiveIdentityFact{
		Scope: row.Scope, ExternalUserID: row.NormalizedValue, HMACKeyVersion: row.FingerprintKeyVersion,
		ID: row.ID, Assurance: row.Assurance, Source: row.Source, NormalizerVersion: row.NormalizerVersion,
		CreatedAt: archiveIdentityTime(row.CreatedAt.Time),
	}
	copy(fact.ReviewFingerprint[:], row.ReviewFingerprint)
	if row.CustomerID.Valid {
		customerID := contactport.CustomerID(row.CustomerID.Int64)
		boundAt := archiveIdentityTime(row.BoundAt.Time)
		fact.CustomerID, fact.BoundAt = &customerID, &boundAt
	}
	return fact, nil
}

func archiveIdentityTime(value time.Time) time.Time     { return value.UTC().Truncate(time.Microsecond) }
func emptyArchiveIdentityHMAC(value [32]byte) bool      { return value == [32]byte{} }
func emptyArchiveIdentityFingerprint(value []byte) bool { return bytes.Equal(value, make([]byte, 16)) }
func sameArchiveIdentityCustomer(left, right *contactport.CustomerID) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}
