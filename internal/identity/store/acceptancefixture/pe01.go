// Package acceptancefixture creates Identity-owned facts for isolated
// cross-domain acceptance tests.
package acceptancefixture

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
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
	_, err := pool.Exec(ctx, `
INSERT INTO identities (
  customer_id, kind, scope, normalized_value, normalizer_version,
  assurance, source, review_fingerprint, fingerprint_key_version, bound_at
) VALUES ($1,$2,$3,$4,1,'verified',$5,$6,1,now())`, customerID, kind, scope, value, source, fingerprint[:16])
	return err
}
