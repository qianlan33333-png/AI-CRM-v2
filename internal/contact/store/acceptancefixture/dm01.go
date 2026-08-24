package acceptancefixture

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

func ResetDM01(ctx context.Context, pool *pgxpool.Pool) error {
	return contactdb.New(pool).ResetDM01AcceptanceFixture(ctx)
}

func DeleteDM01Archives(ctx context.Context, pool *pgxpool.Pool) error {
	return contactdb.New(pool).DeleteDM01AcceptanceArchives(ctx)
}
func EditDM01CustomerName(ctx context.Context, pool *pgxpool.Pool, name string) error {
	return contactdb.New(pool).EditDM01AcceptanceCustomerName(ctx, name)
}

func CreateDM01StaleRun(ctx context.Context, pool *pgxpool.Pool, manifestDigest []byte, repositorySHA, snapshot string, upper time.Time, token []byte, generation int64, expires time.Time) (int64, error) {
	return contactdb.New(pool).CreateDM01AcceptanceStaleRun(ctx, contactdb.CreateDM01AcceptanceStaleRunParams{ManifestDigest: manifestDigest, RepositorySha: repositorySHA, SnapshotID: snapshot, UpperWatermark: pgtype.Timestamptz{Time: upper, Valid: true}, TokenHmac: token, Generation: generation, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}})
}

func SetDM01RunState(ctx context.Context, pool *pgxpool.Pool, runID int64, state string) error {
	return contactdb.New(pool).SetDM01AcceptanceRunState(ctx, contactdb.SetDM01AcceptanceRunStateParams{RunID: runID, State: state})
}

func CreateDM01ExpiredImportingRun(ctx context.Context, pool *pgxpool.Pool, manifestDigest []byte, repositorySHA, snapshot string, upper time.Time, token []byte) (int64, error) {
	return contactdb.New(pool).CreateDM01AcceptanceExpiredImportingRun(ctx, contactdb.CreateDM01AcceptanceExpiredImportingRunParams{ManifestDigest: manifestDigest, RepositorySha: repositorySHA, SnapshotID: snapshot, UpperWatermark: pgtype.Timestamptz{Time: upper, Valid: true}, TokenHmac: token})
}
