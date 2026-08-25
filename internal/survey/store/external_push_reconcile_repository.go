package store

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
)

type verifiedExternalPushReconcile struct {
	binding   surveyapp.ExternalPushBinding
	attemptID int64
}

func (r *ExternalPushRepository) VerifyExternalPushReconcile(ctx context.Context, command surveyapp.ExternalPushReconcileCommand) (surveyapp.ExternalPushBinding, error) {
	if r == nil {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushUnavailable
	}
	verified, err := loadVerifiedExternalPushReconcile(ctx, command)
	if err != nil {
		return surveyapp.ExternalPushBinding{}, err
	}
	if verified.binding.State == "reconciled" {
		providerAccepted, deliveryProven, evidenceDigest, found, err := existingExternalPushReceipt(ctx, verified.binding.ID, verified.attemptID)
		if err != nil {
			return surveyapp.ExternalPushBinding{}, err
		}
		if !found || deliveryProven && evidenceDigest != string(surveyPushDigest(command.EvidenceDigest)) || providerAccepted != command.ProviderAccepted || deliveryProven != command.DeliveryProven {
			return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushReconcileConflict
		}
		return withExternalPushReceipt(verified.binding, providerAccepted, deliveryProven), nil
	}
	if _, _, _, found, err := existingExternalPushReceipt(ctx, verified.binding.ID, verified.attemptID); err != nil {
		return surveyapp.ExternalPushBinding{}, err
	} else if found {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushReconcileConflict
	}
	return verified.binding, nil
}

func (r *ExternalPushRepository) RecordExternalPushReconcile(ctx context.Context, command surveyapp.ExternalPushReconcileCommand) (surveyapp.ExternalPushBinding, error) {
	if r == nil {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushUnavailable
	}
	verified, err := loadVerifiedExternalPushReconcile(ctx, command)
	if err != nil {
		return surveyapp.ExternalPushBinding{}, err
	}
	if verified.binding.State != "reconciled" {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushReconcileRequired
	}
	digest := string(surveyPushDigest(command.EvidenceDigest))
	db, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushUnavailable
	}
	var reconciliationEvidence string
	err = db.QueryRow(ctx, `SELECT evidence_digest FROM external_effect_reconciliations
WHERE effect_id=$1 AND generation=$2 AND fence=$3 FOR SHARE`, effectID(verified.binding.EffectID), command.Lease.Generation, command.Lease.Fence).Scan(&reconciliationEvidence)
	if errors.Is(err, pgx.ErrNoRows) {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushReconcileRequired
	}
	if err != nil {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushUnavailable
	}
	if reconciliationEvidence != digest {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushReconcileConflict
	}
	providerAccepted, deliveryProven, evidenceDigest, found, err := existingExternalPushReceipt(ctx, verified.binding.ID, verified.attemptID)
	if err != nil {
		return surveyapp.ExternalPushBinding{}, err
	}
	if found {
		if deliveryProven && evidenceDigest != digest || providerAccepted != command.ProviderAccepted || deliveryProven != command.DeliveryProven {
			return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushReconcileConflict
		}
		return withExternalPushReceipt(verified.binding, providerAccepted, deliveryProven), nil
	}
	var receiptEvidence any
	if command.DeliveryProven {
		receiptEvidence = digest
	}
	if _, err = db.Exec(ctx, `INSERT INTO questionnaire_external_push_delivery_receipts(binding_id,effect_attempt_id,provider_accepted,delivery_proven,evidence_digest)
VALUES($1,$2,$3,$4,$5)`, verified.binding.ID, verified.attemptID, command.ProviderAccepted, command.DeliveryProven, receiptEvidence); err != nil {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushUnavailable
	}
	return withExternalPushReceipt(verified.binding, command.ProviderAccepted, command.DeliveryProven), nil
}

func loadVerifiedExternalPushReconcile(ctx context.Context, command surveyapp.ExternalPushReconcileCommand) (verifiedExternalPushReconcile, error) {
	db, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushUnavailable
	}
	var value verifiedExternalPushReconcile
	var effectID int64
	var state, owner, kind string
	var createdAt time.Time
	err = db.QueryRow(ctx, `SELECT b.id,b.questionnaire_id,b.public_submission_id,b.customer_id,b.external_effect_id,b.created_at,e.state,e.owner,e.kind
FROM questionnaire_submission_external_push_bindings b
JOIN external_effects e ON e.id=b.external_effect_id
WHERE b.questionnaire_id=$1 AND b.public_submission_id=$2
FOR UPDATE OF b,e`, int64(command.QuestionnaireID), command.SubmissionID).Scan(
		&value.binding.ID, &value.binding.QuestionnaireID, &value.binding.SubmissionID, &value.binding.CustomerID, &effectID, &createdAt, &state, &owner, &kind,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushNotFound
	}
	if err != nil {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushUnavailable
	}
	value.binding.EffectID = effectRef(effectID)
	value.binding.State = eer.State(state)
	value.binding.CreatedAt = createdAt
	if value.binding.EffectID != command.Lease.EffectID || owner != "survey" || kind != "survey_webhook" || (state != "outcome_unknown" && state != "reconciled") {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushReconcileRequired
	}
	err = db.QueryRow(ctx, `SELECT id FROM external_effect_attempts
WHERE effect_id=$1 AND generation=$2 AND fence=$3 AND completion='outcome_unknown'
FOR UPDATE`, effectID, command.Lease.Generation, command.Lease.Fence).Scan(&value.attemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushReconcileRequired
	}
	if err != nil {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushUnavailable
	}
	return value, nil
}

func existingExternalPushReceipt(ctx context.Context, bindingID, attemptID int64) (bool, bool, string, bool, error) {
	db, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return false, false, "", false, surveyapp.ErrExternalPushUnavailable
	}
	var providerAccepted, deliveryProven bool
	var evidenceDigest string
	err = db.QueryRow(ctx, `SELECT provider_accepted,delivery_proven,COALESCE(evidence_digest,'')
FROM questionnaire_external_push_delivery_receipts
WHERE binding_id=$1 AND effect_attempt_id=$2 FOR UPDATE`, bindingID, attemptID).Scan(&providerAccepted, &deliveryProven, &evidenceDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, "", false, nil
	}
	if err != nil {
		return false, false, "", false, surveyapp.ErrExternalPushUnavailable
	}
	return providerAccepted, deliveryProven, evidenceDigest, true, nil
}

func withExternalPushReceipt(value surveyapp.ExternalPushBinding, providerAccepted, deliveryProven bool) surveyapp.ExternalPushBinding {
	value.ProviderAccepted = providerAccepted
	value.DeliveryProven = deliveryProven
	return value
}

func effectRef(id int64) string { return "eer_" + strconv.FormatInt(id, 10) }

func effectID(value string) int64 {
	id, _ := pushEffectID(value)
	return id
}

func surveyPushDigest(value [32]byte) string { return "sha256:" + hex.EncodeToString(value[:]) }
