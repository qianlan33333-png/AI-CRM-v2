package store

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var _ identityport.HistoricalScopedWeComIdentityEvidenceReader = (*Repository)(nil)

func (r *Repository) LockHistoricalScopedWeComIdentityEvidence(ctx context.Context, id int64, sourceKeyHMAC []byte) (identityport.HistoricalScopedWeComIdentityEvidence, error) {
	if r == nil || ctx == nil || id < 1 || len(sourceKeyHMAC) != 32 {
		return identityport.HistoricalScopedWeComIdentityEvidence{}, identityapp.ErrInvalidIdentity
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return identityport.HistoricalScopedWeComIdentityEvidence{}, err
	}
	row, err := identitydb.New(tx).LockHistoricalScopedWeComIdentityEvidence(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return identityport.HistoricalScopedWeComIdentityEvidence{}, identityport.ErrHistoricalScopedIdentityConflict
	}
	if err != nil {
		return identityport.HistoricalScopedWeComIdentityEvidence{}, err
	}
	return historicalReferenceEvidence(row, sourceKeyHMAC)
}

func historicalReferenceEvidence(row identitydb.LockHistoricalScopedWeComIdentityEvidenceRow, key []byte) (identityport.HistoricalScopedWeComIdentityEvidence, error) {
	if len(key) != 32 || row.ID < 1 || !row.CustomerID.Valid || row.CustomerID.Int64 < 1 ||
		row.Kind != string(identityport.KindWeComExternalUserID) || row.FingerprintKeyVersion < 1 ||
		len(row.ReviewFingerprint) != 16 || !bytes.Equal(row.ReviewFingerprint, key[:16]) ||
		(row.Assurance != string(identityport.AssuranceDeclared) && row.Assurance != string(identityport.AssuranceVerified)) {
		return identityport.HistoricalScopedWeComIdentityEvidence{}, identityport.ErrHistoricalScopedIdentityConflict
	}
	normalized, err := identityapp.Normalize(identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: row.Scope, Value: row.NormalizedValue})
	if err != nil || normalized.Scope != row.Scope || normalized.NormalizedValue != row.NormalizedValue {
		return identityport.HistoricalScopedWeComIdentityEvidence{}, identityport.ErrHistoricalScopedIdentityConflict
	}
	return identityport.HistoricalScopedWeComIdentityEvidence{IdentityID: row.ID, Scope: row.Scope, ExternalUserID: row.NormalizedValue, Assurance: identityport.Assurance(row.Assurance), HMACKeyVersion: row.FingerprintKeyVersion}, nil
}
