package store

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
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
	q, err := queries(ctx)
	if err != nil {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushUnavailable
	}
	reconciliationEvidence, err := q.GetSurveyExternalPushReconciliationEvidence(ctx, surveydb.GetSurveyExternalPushReconciliationEvidenceParams{
		EffectID: effectID(verified.binding.EffectID), Generation: command.Lease.Generation, Fence: command.Lease.Fence,
	})
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
	receiptEvidence := pgtype.Text{}
	if command.DeliveryProven {
		receiptEvidence = pgtype.Text{String: digest, Valid: true}
	}
	if err = q.InsertSurveyExternalPushDeliveryReceipt(ctx, surveydb.InsertSurveyExternalPushDeliveryReceiptParams{
		BindingID: verified.binding.ID, EffectAttemptID: verified.attemptID,
		ProviderAccepted: command.ProviderAccepted, DeliveryProven: command.DeliveryProven, EvidenceDigest: receiptEvidence,
	}); err != nil {
		return surveyapp.ExternalPushBinding{}, surveyapp.ErrExternalPushUnavailable
	}
	return withExternalPushReceipt(verified.binding, command.ProviderAccepted, command.DeliveryProven), nil
}

func loadVerifiedExternalPushReconcile(ctx context.Context, command surveyapp.ExternalPushReconcileCommand) (verifiedExternalPushReconcile, error) {
	q, err := queries(ctx)
	if err != nil {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushUnavailable
	}
	row, err := q.LockSurveyExternalPushReconcile(ctx, surveydb.LockSurveyExternalPushReconcileParams{
		QuestionnaireID: int64(command.QuestionnaireID), PublicSubmissionID: command.SubmissionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushNotFound
	}
	if err != nil {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushUnavailable
	}
	value := verifiedExternalPushReconcile{binding: surveyapp.ExternalPushBinding{
		ID: row.ID, QuestionnaireID: surveyport.ID(row.QuestionnaireID), SubmissionID: row.PublicSubmissionID,
		CustomerID: row.CustomerID, EffectID: effectRef(row.ExternalEffectID), State: eer.State(row.State), CreatedAt: row.CreatedAt.Time,
	}}
	if value.binding.EffectID != command.Lease.EffectID || row.Owner != "survey" || row.Kind != "survey_webhook" || (row.State != "outcome_unknown" && row.State != "reconciled") {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushReconcileRequired
	}
	value.attemptID, err = q.LockSurveyExternalPushUnknownAttempt(ctx, surveydb.LockSurveyExternalPushUnknownAttemptParams{
		EffectID: row.ExternalEffectID, Generation: command.Lease.Generation, Fence: command.Lease.Fence,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushReconcileRequired
	}
	if err != nil {
		return verifiedExternalPushReconcile{}, surveyapp.ErrExternalPushUnavailable
	}
	return value, nil
}

func existingExternalPushReceipt(ctx context.Context, bindingID, attemptID int64) (bool, bool, string, bool, error) {
	q, err := queries(ctx)
	if err != nil {
		return false, false, "", false, surveyapp.ErrExternalPushUnavailable
	}
	row, err := q.LockSurveyExternalPushDeliveryReceipt(ctx, surveydb.LockSurveyExternalPushDeliveryReceiptParams{
		BindingID: bindingID, EffectAttemptID: attemptID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, "", false, nil
	}
	if err != nil {
		return false, false, "", false, surveyapp.ErrExternalPushUnavailable
	}
	return row.ProviderAccepted, row.DeliveryProven, row.EvidenceDigest, true, nil
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
