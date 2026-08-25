package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestExternalPushReconcileRepositoryPostgreSQLVerifiesBindingAndSeparatesReceiptFacts(t *testing.T) {
	databaseURL := os.Getenv("AICRM_SURVEY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AICRM_SURVEY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}

	repository := NewExternalPushRepository()
	uow := platformstore.NewUnitOfWork(pool)
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	err = uow.Within(ctx, func(tx context.Context) error {
		db, err := platformstore.TxFromContext(tx)
		if err != nil {
			return err
		}
		questionnaireID, submissionID, effectID, attemptID, err := externalPushReconcileFixture(tx, db, now)
		if err != nil {
			return err
		}
		evidence := sha256.Sum256([]byte("operator evidence"))
		command := surveyapp.ExternalPushReconcileCommand{
			QuestionnaireID: surveyport.ID(questionnaireID), SubmissionID: submissionID,
			Lease:          eer.Lease{EffectID: effectRef(effectID), Generation: 2, Fence: 5, ExpiresAt: now.Add(time.Minute)},
			EvidenceDigest: evidence, ProviderAccepted: true, DeliveryProven: false, IdempotencyKey: "survey-external-push-reconcile-key",
		}
		verified, err := repository.VerifyExternalPushReconcile(tx, command)
		if err != nil || verified.State != eer.StateOutcomeUnknown || verified.EffectID != command.Lease.EffectID {
			return errors.Join(surveyapp.ErrExternalPushUnavailable, err)
		}
		if _, err = db.Exec(tx, `INSERT INTO external_effect_reconciliations(effect_id,generation,fence,evidence_digest) VALUES($1,$2,$3,$4)`, effectID, 2, 5, surveyPushDigest(evidence)); err != nil {
			return err
		}
		if _, err = db.Exec(tx, `UPDATE external_effects SET state='reconciled',updated_at=$2 WHERE id=$1`, effectID, now); err != nil {
			return err
		}
		recorded, err := repository.RecordExternalPushReconcile(tx, command)
		if err != nil || recorded.State != eer.StateReconciled || !recorded.ProviderAccepted || recorded.DeliveryProven {
			return errors.Join(surveyapp.ErrExternalPushUnavailable, err)
		}
		if verified, err = repository.VerifyExternalPushReconcile(tx, command); err != nil || !verified.ProviderAccepted || verified.DeliveryProven {
			return errors.Join(surveyapp.ErrExternalPushUnavailable, err)
		}
		if _, err = repository.RecordExternalPushReconcile(tx, command); err != nil {
			return err
		}
		command.ProviderAccepted = false
		if _, err = repository.RecordExternalPushReconcile(tx, command); !errors.Is(err, surveyapp.ErrExternalPushReconcileConflict) {
			return surveyapp.ErrExternalPushUnavailable
		}
		var receiptCount int
		if err = db.QueryRow(tx, `SELECT count(*) FROM questionnaire_external_push_delivery_receipts WHERE effect_attempt_id=$1`, attemptID).Scan(&receiptCount); err != nil || receiptCount != 1 {
			return errors.Join(surveyapp.ErrExternalPushUnavailable, err)
		}
		return errExternalPushReconcileRollback
	})
	if !errors.Is(err, errExternalPushReconcileRollback) {
		t.Fatalf("round trip=%v", err)
	}
}

func externalPushReconcileFixture(ctx context.Context, db pgx.Tx, now time.Time) (questionnaireID, submissionID, effectID, attemptID int64, err error) {
	if err = db.QueryRow(ctx, `INSERT INTO questionnaires(slug,name,title,description,answer_display_mode,assessment_enabled,assessment_config,is_disabled,created_by,version,submission_count,created_at,updated_at)
VALUES('reconcile-it','reconcile-it','reconcile-it','', 'all_in_one',FALSE,'{}'::jsonb,FALSE,1,1,0,$1,$1) RETURNING id`, now).Scan(&questionnaireID); err != nil {
		return
	}
	var definitionID, receiptID int64
	if err = db.QueryRow(ctx, `INSERT INTO questionnaire_public_definitions(questionnaire_id,definition_version,slug,state,answer_display_mode,title,description,created_at)
VALUES($1,1,'reconcile-it','draft','all_in_one','reconcile-it','',$2) RETURNING id`, questionnaireID, now).Scan(&definitionID); err != nil {
		return
	}
	digest := sha256.Sum256([]byte("external-push-reconcile-fixture"))
	if err = db.QueryRow(ctx, `INSERT INTO questionnaire_public_submission_receipts(definition_id,anonymous_digest,submission_key_digest,payload_digest,created_at)
VALUES($1,$2,$2,$2,$3) RETURNING id`, definitionID, digest[:], now).Scan(&receiptID); err != nil {
		return
	}
	if err = db.QueryRow(ctx, `INSERT INTO questionnaire_public_submissions(receipt_id,definition_id,submitted_at,created_at)
VALUES($1,$2,$3,$3) RETURNING id`, receiptID, definitionID, now).Scan(&submissionID); err != nil {
		return
	}
	hash := "sha256:" + strings.Repeat("a", 64)
	if err = db.QueryRow(ctx, `INSERT INTO external_effects(owner,kind,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash,envelope_fingerprint,state,attempt_count,generation,lease_fence,updated_at,created_at)
VALUES('survey','survey_webhook',$1,$1,$1,$1,$2,'outcome_unknown',1,2,5,$3,$3) RETURNING id`, hash, "sha256:"+strings.Repeat("b", 64), now).Scan(&effectID); err != nil {
		return
	}
	if err = db.QueryRow(ctx, `INSERT INTO external_effect_attempts(effect_id,number,generation,fence,started_at,completion,receipt_digest,completed_at)
VALUES($1,1,2,5,$2,'outcome_unknown',$3,$2) RETURNING id`, effectID, now, hash).Scan(&attemptID); err != nil {
		return
	}
	_, err = db.Exec(ctx, `INSERT INTO questionnaire_submission_external_push_bindings(questionnaire_id,public_submission_id,customer_id,external_effect_id,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash)
VALUES($1,$2,77,$3,$4,$4,$4,$4)`, questionnaireID, submissionID, effectID, hash)
	return
}

var errExternalPushReconcileRollback = errors.New("rollback external push reconcile")
