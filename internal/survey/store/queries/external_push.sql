-- name: BindSurveyExternalPush :one
INSERT INTO questionnaire_submission_external_push_bindings(questionnaire_id,public_submission_id,customer_id,external_effect_id,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash)
VALUES(sqlc.arg(questionnaire_id),sqlc.arg(public_submission_id),sqlc.arg(customer_id),sqlc.arg(external_effect_id),sqlc.arg(source_ref_digest),sqlc.arg(target_ref_digest),sqlc.arg(payload_digest),sqlc.arg(policy_version_hash))
ON CONFLICT (public_submission_id) DO UPDATE SET public_submission_id=EXCLUDED.public_submission_id
RETURNING id,questionnaire_id,public_submission_id,customer_id,external_effect_id,created_at;

-- name: GetSurveyExternalPush :one
SELECT b.id,b.questionnaire_id,b.public_submission_id,b.customer_id,b.external_effect_id,b.created_at,e.state,
COALESCE((SELECT bool_or(r.provider_accepted) FROM questionnaire_external_push_delivery_receipts r WHERE r.binding_id=b.id),false) provider_accepted,
COALESCE((SELECT bool_or(r.delivery_proven) FROM questionnaire_external_push_delivery_receipts r WHERE r.binding_id=b.id),false) delivery_proven
FROM questionnaire_submission_external_push_bindings b JOIN external_effects e ON e.id=b.external_effect_id
WHERE b.questionnaire_id=sqlc.arg(questionnaire_id) AND b.public_submission_id=sqlc.arg(public_submission_id);
