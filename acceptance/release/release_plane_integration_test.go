package release_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	authacceptance "github.com/qianlan33333-png/AI-CRM-v2/acceptance/auth"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	releaseapp "github.com/qianlan33333-png/AI-CRM-v2/internal/release/app"
	releaseport "github.com/qianlan33333-png/AI-CRM-v2/internal/release/port"
	releasestore "github.com/qianlan33333-png/AI-CRM-v2/internal/release/store"
)

var releaseDatabaseURL = flag.String("release-database-url", "", "isolated RP01 PostgreSQL 16.14 acceptance database")

func TestReleasePlanePG16LifecycleConcurrencyReplayAndGuards(t *testing.T) {
	if *releaseDatabaseURL == "" {
		t.Skip("-release-database-url is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *releaseDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	assertReleasePG16(t, ctx, pool)
	authFixture, err := authacceptance.OpenPostgreSQL(ctx, *releaseDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authFixture.Close)
	actorID, err := authFixture.SeedAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service := releaseapp.NewService(platformstore.NewUnitOfWork(pool), releasestore.NewRepository(pool))
	assertPrerequisiteSubjectAndSameCommitGuards(t, ctx, pool, service, actorID)

	first := registerReady(t, ctx, service, actorID, "first")
	preparedDetail, err := service.Detail(ctx, first.ID)
	if err != nil || preparedDetail.ActiveWorker != nil || !preparedDetail.Readiness.Ready || len(preparedDetail.Prerequisites) != 7 {
		t.Fatalf("prepared detail=%#v err=%v", preparedDetail, err)
	}
	assertPrerequisitesBoundToCandidate(t, ctx, pool, first.ID, 7)
	worker, winnerKey := concurrentStartWinner(t, ctx, service, first.ID, actorID)
	replayed, err := service.StartCutover(ctx, releaseapp.CandidateCommand{
		CandidateID: first.ID, ActorID: actorID, IdempotencyKey: winnerKey,
	})
	if err != nil || replayed.Generation != worker.Generation || replayed.Fence != worker.Fence {
		t.Fatalf("start replay=%#v err=%v, winner=%#v", replayed, err, worker)
	}

	second := registerReady(t, ctx, service, actorID, "second")
	if _, err = service.StartCutover(ctx, releaseapp.CandidateCommand{
		CandidateID: second.ID, ActorID: actorID, IdempotencyKey: "rp01-second-competing-start",
	}); !errors.Is(err, releaseport.ErrConflict) {
		t.Fatalf("global active worker conflict err=%v", err)
	}

	restarted, err := service.RestartCutover(ctx, releaseapp.WorkerCommand{
		CandidateID: first.ID, ActorID: actorID, Generation: worker.Generation, Fence: worker.Fence,
		IdempotencyKey: "rp01-restart-generation-key",
	})
	if err != nil || restarted.Generation != worker.Generation+1 || restarted.Fence == worker.Fence {
		t.Fatalf("restarted=%#v err=%v", restarted, err)
	}
	activeDetail, err := service.Detail(ctx, first.ID)
	if err != nil || activeDetail.ActiveWorker == nil || activeDetail.ActiveWorker.Generation != restarted.Generation {
		t.Fatalf("active detail=%#v err=%v", activeDetail, err)
	}
	rawDetail, err := json.Marshal(activeDetail)
	if err != nil || strings.Contains(string(rawDetail), restarted.Fence) || strings.Contains(string(rawDetail), `"Fence"`) {
		t.Fatalf("ordinary detail exposed fence: %s err=%v", rawDetail, err)
	}
	if _, err = service.CompleteStep(ctx, releaseapp.StepCommand{
		CandidateID: first.ID, ActorID: actorID, Generation: worker.Generation, Fence: worker.Fence,
		Step: releaseport.CutoverAnnounce, IdempotencyKey: "rp01-stale-worker-step-key",
	}); !errors.Is(err, releaseapp.ErrFence) {
		t.Fatalf("stale generation err=%v", err)
	}

	concurrentStepWinner(t, ctx, service, first.ID, actorID, restarted)
	for index, step := range releaseport.FixedCutoverSteps[1:] {
		entry, stepErr := service.CompleteStep(ctx, releaseapp.StepCommand{
			CandidateID: first.ID, ActorID: actorID, Generation: restarted.Generation, Fence: restarted.Fence,
			Step: step, IdempotencyKey: fmt.Sprintf("rp01-cutover-step-key-%02d", index+1),
		})
		if stepErr != nil || entry.Step != step {
			t.Fatalf("step=%s entry=%#v err=%v", step, entry, stepErr)
		}
	}
	activated, err := service.Activate(ctx, releaseapp.WorkerCommand{
		CandidateID: first.ID, ActorID: actorID, Generation: restarted.Generation, Fence: restarted.Fence,
		IdempotencyKey: "rp01-candidate-activate-key",
	})
	if err != nil || activated.State != releaseport.CandidateActivated {
		t.Fatalf("activated=%#v err=%v", activated, err)
	}
	activatedDetail, err := service.Detail(ctx, first.ID)
	if err != nil || activatedDetail.ActiveWorker != nil || len(activatedDetail.CutoverProgress) != len(releaseport.FixedCutoverSteps) {
		t.Fatalf("activated detail=%#v err=%v", activatedDetail, err)
	}

	secondWorker, err := service.StartCutover(ctx, releaseapp.CandidateCommand{
		CandidateID: second.ID, ActorID: actorID, IdempotencyKey: "rp01-second-start-after-retire",
	})
	if err != nil || secondWorker.Generation != 1 {
		t.Fatalf("second worker after retirement=%#v err=%v", secondWorker, err)
	}

	assertReleaseTamperingRejected(t, ctx, pool, first.ID)
	for index, kind := range []releaseport.RollbackCheckKind{
		releaseport.RollbackSchemaCompatibility,
		releaseport.RollbackDataReconciliation,
		releaseport.RollbackOutboundReconciliation,
	} {
		if _, err = service.RecordRollbackCheck(ctx, releaseapp.RollbackCheckCommand{
			CandidateID: first.ID, ActorID: actorID, Kind: kind, Passed: true,
			EvidenceSHA:    sha256Hex(fmt.Sprintf("rollback-%d", index)),
			IdempotencyKey: fmt.Sprintf("rp01-rollback-check-key-%02d", index),
		}); err != nil {
			t.Fatal(err)
		}
	}
	requested, err := service.RequestRollback(ctx, releaseapp.CandidateCommand{
		CandidateID: first.ID, ActorID: actorID, IdempotencyKey: "rp01-rollback-request-key",
	})
	if err != nil || requested.State != releaseport.CandidateRollbackPending {
		t.Fatalf("rollback request=%#v err=%v", requested, err)
	}
	if _, err = service.RecordRollbackCheck(ctx, releaseapp.RollbackCheckCommand{
		CandidateID: first.ID, ActorID: actorID, Kind: releaseport.RollbackExecutionReconciliation, Passed: true,
		EvidenceSHA: sha256Hex("rollback-execution-reconciliation"), IdempotencyKey: "rp01-rollback-execution-reconciliation-key",
	}); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := service.CompleteRollback(ctx, releaseapp.CandidateCommand{
		CandidateID: first.ID, ActorID: actorID, IdempotencyKey: "rp01-rollback-complete-key",
	})
	if err != nil || rolledBack.State != releaseport.CandidateRolledBack {
		t.Fatalf("rolled back=%#v err=%v", rolledBack, err)
	}
	rolledBackDetail, err := service.Detail(ctx, first.ID)
	if err != nil || rolledBackDetail.Candidate.State != releaseport.CandidateRolledBack || rolledBackDetail.ActiveWorker != nil {
		t.Fatalf("rolled back detail=%#v err=%v", rolledBackDetail, err)
	}

	var incomplete int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM release_operation_receipts WHERE state<>'completed' OR result_snapshot IS NULL OR completed_at IS NULL`).Scan(&incomplete); err != nil || incomplete != 0 {
		t.Fatalf("incomplete receipts=%d err=%v", incomplete, err)
	}
}

func assertPrerequisiteSubjectAndSameCommitGuards(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *releaseapp.Service, actorID int64) {
	t.Helper()
	probe, err := service.Register(ctx, releaseapp.RegisterCommand{
		CommitSHA: sha256Hex("subject-probe")[:40], ArtifactDigest: sha256Hex("subject-artifact"),
		ManifestDigest: sha256Hex("subject-manifest"), ConfigDigest: sha256Hex("subject-config"),
		TargetSchemaVersion: 74, ActorID: actorID, IdempotencyKey: "rp01-subject-probe-register-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO release_prerequisite_receipts(
candidate_id,candidate_commit_sha,candidate_artifact_digest,candidate_manifest_digest,candidate_config_digest,candidate_schema_version,kind,evidence_sha,recorded_by
) VALUES($1,$2,repeat('0',64),$3,$4,$5,'nightly',repeat('1',64),$6)`,
		probe.ID, probe.CommitSHA, probe.ManifestDigest, probe.ConfigDigest, probe.TargetSchemaVersion, actorID)
	assertSQLState(t, err, "23503")
	_, err = service.Register(ctx, releaseapp.RegisterCommand{
		CommitSHA: probe.CommitSHA, ArtifactDigest: sha256Hex("different-artifact"),
		ManifestDigest: probe.ManifestDigest, ConfigDigest: sha256Hex("different-config"),
		TargetSchemaVersion: probe.TargetSchemaVersion, ActorID: actorID,
		IdempotencyKey: "rp01-same-commit-different-subject",
	})
	if !errors.Is(err, releaseport.ErrConflict) {
		t.Fatalf("same commit different subject err=%v", err)
	}
}

func registerReady(t *testing.T, ctx context.Context, service *releaseapp.Service, actorID int64, suffix string) releaseport.Candidate {
	t.Helper()
	candidate, err := service.Register(ctx, releaseapp.RegisterCommand{
		CommitSHA: sha256Hex("main-" + suffix)[:40], ArtifactDigest: sha256Hex("artifact-" + suffix),
		ManifestDigest: sha256Hex("manifest-" + suffix), ConfigDigest: sha256Hex("config-" + suffix),
		TargetSchemaVersion: 74, ActorID: actorID, IdempotencyKey: "rp01-register-candidate-" + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []releaseport.PrerequisiteKind{
		releaseport.PrerequisiteNightly,
		releaseport.PrerequisiteBackupRestoreDrill,
		releaseport.PrerequisiteMigration,
		releaseport.PrerequisiteContactClosure,
		releaseport.PrerequisiteCampaignClosure,
		releaseport.PrerequisiteOutboundClosure,
		releaseport.PrerequisiteCommerceClosure,
	} {
		if _, err = service.RecordPrerequisite(ctx, releaseapp.ReceiptCommand{
			CandidateID: candidate.ID, ActorID: actorID, Kind: kind,
			EvidenceSHA:    sha256Hex(fmt.Sprintf("%s-%d", suffix, index)),
			IdempotencyKey: fmt.Sprintf("rp01-prerequisite-%s-%02d", suffix, index),
		}); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			_, duplicateErr := service.RecordPrerequisite(ctx, releaseapp.ReceiptCommand{
				CandidateID: candidate.ID, ActorID: actorID, Kind: kind,
				EvidenceSHA:    sha256Hex("replacement-" + suffix),
				IdempotencyKey: "rp01-prerequisite-replacement-" + suffix,
			})
			if !errors.Is(duplicateErr, releaseport.ErrConflict) {
				t.Fatalf("prerequisite replacement err=%v", duplicateErr)
			}
		}
	}
	prepared, err := service.Prepare(ctx, releaseapp.CandidateCommand{
		CandidateID: candidate.ID, ActorID: actorID, IdempotencyKey: "rp01-prepare-candidate-" + suffix,
	})
	if err != nil || prepared.State != releaseport.CandidatePrepared {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
	return prepared
}

func assertPrerequisitesBoundToCandidate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, candidateID int64, want int) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM release_prerequisite_receipts receipt
JOIN release_candidates candidate
  ON candidate.id=receipt.candidate_id
 AND candidate.commit_sha=receipt.candidate_commit_sha
 AND candidate.artifact_digest=receipt.candidate_artifact_digest
 AND candidate.manifest_digest=receipt.candidate_manifest_digest
 AND candidate.config_digest=receipt.candidate_config_digest
 AND candidate.target_schema_version=receipt.candidate_schema_version
WHERE receipt.candidate_id=$1`, candidateID).Scan(&count)
	if err != nil || count != want {
		t.Fatalf("bound prerequisite count=%d err=%v, want %d", count, err, want)
	}
}

func concurrentStartWinner(t *testing.T, ctx context.Context, service *releaseapp.Service, candidateID, actorID int64) (releaseport.WorkerLease, string) {
	t.Helper()
	type result struct {
		worker releaseport.WorkerLease
		key    string
		err    error
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			key := fmt.Sprintf("rp01-concurrent-start-key-%d", index)
			worker, err := service.StartCutover(ctx, releaseapp.CandidateCommand{
				CandidateID: candidateID, ActorID: actorID,
				IdempotencyKey: key,
			})
			results <- result{worker: worker, key: key, err: err}
		}()
	}
	wait.Wait()
	close(results)
	winners, failures := make([]result, 0, 1), 0
	for result := range results {
		if result.err == nil {
			winners = append(winners, result)
		} else {
			failures++
		}
	}
	if len(winners) != 1 || failures != 1 {
		t.Fatalf("concurrent starts winners=%d failures=%d", len(winners), failures)
	}
	return winners[0].worker, winners[0].key
}

func concurrentStepWinner(t *testing.T, ctx context.Context, service *releaseapp.Service, candidateID, actorID int64, worker releaseport.WorkerLease) {
	t.Helper()
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := service.CompleteStep(ctx, releaseapp.StepCommand{
				CandidateID: candidateID, ActorID: actorID, Generation: worker.Generation, Fence: worker.Fence,
				Step:           releaseport.CutoverAnnounce,
				IdempotencyKey: fmt.Sprintf("rp01-concurrent-step-key-%d", index),
			})
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	winners, failures := 0, 0
	for err := range results {
		if err == nil {
			winners++
		} else {
			failures++
		}
	}
	if winners != 1 || failures != 1 {
		t.Fatalf("concurrent steps winners=%d failures=%d", winners, failures)
	}
}

func assertReleaseTamperingRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, candidateID int64) {
	t.Helper()
	statements := []string{
		`UPDATE release_candidates SET artifact_digest=repeat('0',64) WHERE id=$1`,
		`DELETE FROM release_candidates WHERE id=$1`,
		`UPDATE release_prerequisite_receipts SET evidence_sha=repeat('2',64) WHERE candidate_id=$1`,
		`UPDATE release_cutover_journal SET completed_at=completed_at+interval '1 second' WHERE candidate_id=$1`,
		`DELETE FROM release_cutover_journal WHERE candidate_id=$1`,
		`UPDATE release_worker_leases SET fence=repeat('1',64) WHERE candidate_id=$1`,
		`UPDATE release_operation_receipts SET result_snapshot='{}'::jsonb WHERE action='candidate.activate' AND actor_id>0 AND $1::bigint>0`,
	}
	for _, statement := range statements {
		_, err := pool.Exec(ctx, statement, candidateID)
		assertSQLState(t, err, "55000")
	}
}

func assertReleasePG16(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var version int
	var waterline int64
	if err := pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&waterline); err != nil {
		t.Fatal(err)
	}
	if version != 160014 || waterline != 74 {
		t.Fatalf("server_version_num=%d waterline=%d, want 160014/74", version, waterline)
	}
}

func assertSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != want {
		t.Fatalf("error=%v SQLSTATE=%v, want %s", err, pgErr, want)
	}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
