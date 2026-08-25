package acceptancefixture

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	eerdb "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store/generated"
)

// CreatePE01Effect creates the EER-owned control fact required by the PE01
// fake-provider acceptance adapter.
func CreatePE01Effect(ctx context.Context, pool *pgxpool.Pool, kind, source, target, payload, policy, fingerprint, state string) (int64, error) {
	return eerdb.New(pool).CreatePE01AcceptanceEffect(ctx, eerdb.CreatePE01AcceptanceEffectParams{
		Kind: kind, SourceRefDigest: source, TargetRefDigest: target,
		PayloadDigest: payload, PolicyVersionHash: policy,
		EnvelopeFingerprint: fingerprint, State: state,
	})
}
