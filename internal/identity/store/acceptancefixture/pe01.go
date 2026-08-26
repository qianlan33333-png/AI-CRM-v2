// Package acceptancefixture creates Identity-owned facts for isolated
// cross-domain acceptance tests.
package acceptancefixture

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
)

var ErrInvalidVerifiedIdentityFixture = errors.New("invalid verified identity fixture")

func CreatePE01VerifiedMPOpenID(ctx context.Context, pool *pgxpool.Pool, customerID int64, scope, openID string) error {
	return createVerifiedIdentity(ctx, pool, customerID, "mp_openid", scope, openID, "pe01-acceptance")
}

func CreateCampaignDispatchVerifiedExternalUserID(ctx context.Context, pool *pgxpool.Pool, customerID int64, corpID, externalUserID string) error {
	return createVerifiedIdentity(ctx, pool, customerID, "wecom_external_userid", "wecom-corp:"+corpID, externalUserID, "campaign-dispatch-acceptance")
}

func createVerifiedIdentity(ctx context.Context, pool *pgxpool.Pool, customerID int64, kind, scope, value, source string) error {
	if ctx == nil || pool == nil || customerID < 1 || kind == "" || scope == "" || value == "" || source == "" {
		return ErrInvalidVerifiedIdentityFixture
	}
	fingerprint := sha256.Sum256([]byte(source + ":" + scope + ":" + value))
	return identitydb.New(pool).CreateVerifiedIdentityFixture(ctx, identitydb.CreateVerifiedIdentityFixtureParams{
		CustomerID:        customerID,
		Kind:              kind,
		Scope:             scope,
		NormalizedValue:   value,
		Source:            source,
		ReviewFingerprint: fingerprint[:16],
	})
}
