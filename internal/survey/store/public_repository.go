package store

// This adapter is intentionally isolated from the legacy submission tables:
// public answers are immutable anonymous snapshots only.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type PublicRepository struct{}

var _ surveyapp.PublicStore = (*PublicRepository)(nil)

func NewPublicRepository() *PublicRepository { return &PublicRepository{} }

func publicTx(ctx context.Context) (pgx.Tx, error) { return platformstore.TxFromContext(ctx) }

func (r *PublicRepository) GetPublishedBySlug(ctx context.Context, slug string) (surveyapp.PublicDefinitionRecord, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil || slug == "" {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	var id, questionnaireID, version int64
	var state string
	// A submit holds this shared lock through receipt/submission completion. A
	// concurrent disable therefore linearizes either before us (not found) or
	// after our accepted local receipt; it cannot disable between read and write.
	err = tx.QueryRow(ctx, `SELECT id,questionnaire_id,definition_version,state FROM questionnaire_public_definitions WHERE slug=$1 AND state='public' FOR SHARE`, slug).Scan(&id, &questionnaireID, &version, &state)
	if err != nil {
		return surveyapp.PublicDefinitionRecord{}, publicDBErr(err)
	}
	return r.definition(ctx, id, questionnaireID, version, state, slug)
}
func (r *PublicRepository) GetPublicDefinition(ctx context.Context, questionnaireID surveyport.ID, version int64) (surveyapp.PublicDefinitionRecord, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil || questionnaireID < 1 || version < 1 {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	var id int64
	var state, slug string
	err = tx.QueryRow(ctx, `SELECT id,state,slug FROM questionnaire_public_definitions WHERE questionnaire_id=$1 AND definition_version=$2`, int64(questionnaireID), version).Scan(&id, &state, &slug)
	if err != nil {
		return surveyapp.PublicDefinitionRecord{}, publicDBErr(err)
	}
	return r.definition(ctx, id, int64(questionnaireID), version, state, slug)
}
func (r *PublicRepository) definition(ctx context.Context, id, qid, version int64, state, slug string) (surveyapp.PublicDefinitionRecord, error) {
	tx, err := publicTx(ctx)
	if err != nil {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	var title, description, mode string
	if err = tx.QueryRow(ctx, `SELECT title,description,answer_display_mode FROM questionnaire_public_definitions WHERE id=$1`, id).Scan(&title, &description, &mode); err != nil {
		return surveyapp.PublicDefinitionRecord{}, publicDBErr(err)
	}
	rows, err := tx.Query(ctx, `SELECT q.id,q.type,q.title,q.required,q.sort_order,q.minimum_selections,q.maximum_selections,o.id,o.option_text,o.sort_order FROM questionnaire_public_definition_questions q LEFT JOIN questionnaire_public_definition_options o ON o.question_id=q.id WHERE q.definition_id=$1 ORDER BY q.sort_order,o.sort_order`, id)
	if err != nil {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	defer rows.Close()
	view := surveyport.PublicQuestionnaire{ID: surveyport.ID(qid), Slug: slug, Title: title, Description: description, AnswerDisplayMode: surveyport.AnswerDisplayMode(mode), Version: version, Questions: []surveyport.PublicQuestion{}}
	index := map[int64]int{}
	for rows.Next() {
		var qid int64
		var typ, qt string
		var req bool
		var sort, min, max int
		var oid *int64
		var ot *string
		var os *int
		if err = rows.Scan(&qid, &typ, &qt, &req, &sort, &min, &max, &oid, &ot, &os); err != nil {
			return surveyapp.PublicDefinitionRecord{}, unavailable(err)
		}
		i, ok := index[qid]
		if !ok {
			i = len(view.Questions)
			index[qid] = i
			view.Questions = append(view.Questions, surveyport.PublicQuestion{ID: qid, Type: surveyport.QuestionType(typ), Title: qt, Required: req, SortOrder: sort, Minimum: min, Maximum: max, Options: []surveyport.PublicOption{}})
		}
		if oid != nil && ot != nil && os != nil {
			view.Questions[i].Options = append(view.Questions[i].Options, surveyport.PublicOption{ID: *oid, OptionText: *ot, SortOrder: *os})
		}
	}
	if err = rows.Err(); err != nil {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	return surveyapp.PublicDefinitionRecord{ID: id, State: state, View: view}, nil
}
func (r *PublicRepository) GetQuestionnaire(ctx context.Context, id surveyport.ID) (surveyport.Questionnaire, error) {
	return NewQuestionnaireRepository().Get(ctx, id)
}
func (r *PublicRepository) CreatePublicDefinition(ctx context.Context, source surveyport.Questionnaire, now time.Time) (surveyapp.PublicDefinitionRecord, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	view, err := surveyapp.PublicDefinition(source)
	if err != nil {
		return surveyapp.PublicDefinitionRecord{}, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(source.ID)); err != nil {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	// Lifecycle is explicit: a caller must disable the existing public version
	// before publishing another immutable snapshot. Publish never mutates it.
	var alreadyPublic bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM questionnaire_public_definitions WHERE questionnaire_id=$1 AND state='public')`, int64(source.ID)).Scan(&alreadyPublic); err != nil {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	if alreadyPublic {
		return surveyapp.PublicDefinitionRecord{}, surveyapp.ErrConflict
	}
	var id, version int64
	err = tx.QueryRow(ctx, `INSERT INTO questionnaire_public_definitions(questionnaire_id,definition_version,slug,state,answer_display_mode,title,description,created_at,published_at) SELECT $1,COALESCE(MAX(definition_version),0)+1,$2,'public',$3,$4,$5,$6,$6 FROM questionnaire_public_definitions WHERE questionnaire_id=$1 RETURNING id,definition_version`, int64(source.ID), view.Slug, string(view.AnswerDisplayMode), view.Title, view.Description, now).Scan(&id, &version)
	if err != nil {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	for _, question := range view.Questions {
		var snapshotQ int64
		err = tx.QueryRow(ctx, `INSERT INTO questionnaire_public_definition_questions(definition_id,source_question_id,type,title,required,sort_order,minimum_selections,maximum_selections) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, id, question.ID, string(question.Type), question.Title, question.Required, question.SortOrder, question.Minimum, question.Maximum).Scan(&snapshotQ)
		if err != nil {
			return surveyapp.PublicDefinitionRecord{}, unavailable(err)
		}
		for _, option := range question.Options {
			if _, err = tx.Exec(ctx, `INSERT INTO questionnaire_public_definition_options(question_id,source_option_id,option_text,sort_order) VALUES($1,$2,$3,$4)`, snapshotQ, option.ID, option.OptionText, option.SortOrder); err != nil {
				return surveyapp.PublicDefinitionRecord{}, unavailable(err)
			}
		}
	}
	return r.definition(ctx, id, int64(source.ID), version, "public", source.Slug)
}
func (r *PublicRepository) DisablePublicDefinition(ctx context.Context, qid surveyport.ID, version int64, now time.Time) (surveyapp.PublicDefinitionRecord, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return surveyapp.PublicDefinitionRecord{}, unavailable(err)
	}
	var id int64
	err = tx.QueryRow(ctx, `UPDATE questionnaire_public_definitions SET state='disabled',disabled_at=$3 WHERE questionnaire_id=$1 AND definition_version=$2 AND state='public' RETURNING id`, int64(qid), version, now).Scan(&id)
	if err != nil {
		return surveyapp.PublicDefinitionRecord{}, publicDBErr(err)
	}
	return r.definition(ctx, id, int64(qid), version, "disabled", "")
}

func (r *PublicRepository) ReservePublicReceipt(ctx context.Context, d surveyapp.PublicDefinitionRecord, anon, key, payload [32]byte, now time.Time) (surveyapp.PublicReceipt, bool, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return surveyapp.PublicReceipt{}, false, unavailable(err)
	}
	var x surveyapp.PublicReceipt
	var a, k, p, token []byte
	var snap []byte
	var complete *time.Time
	err = tx.QueryRow(ctx, `INSERT INTO questionnaire_public_submission_receipts(definition_id,anonymous_digest,submission_key_digest,payload_digest,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(definition_id,anonymous_digest,submission_key_digest) DO NOTHING RETURNING id,definition_id,anonymous_digest,submission_key_digest,payload_digest,result_token_digest,state,result_snapshot,completed_at`, d.ID, anon[:], key[:], payload[:], now).Scan(&x.ID, &x.DefinitionID, &a, &k, &p, &token, &x.State, &snap, &complete)
	owned := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id,definition_id,anonymous_digest,submission_key_digest,payload_digest,result_token_digest,state,result_snapshot,completed_at FROM questionnaire_public_submission_receipts WHERE definition_id=$1 AND anonymous_digest=$2 AND submission_key_digest=$3`, d.ID, anon[:], key[:]).Scan(&x.ID, &x.DefinitionID, &a, &k, &p, &token, &x.State, &snap, &complete)
	}
	if err != nil {
		return x, false, unavailable(err)
	}
	copy(x.SubmissionKeyDigest[:], k)
	copy(x.PayloadDigest[:], p)
	copy(x.ResultTokenDigest[:], token)
	x.ResultSnapshot = snap
	return x, owned, nil
}
func (r *PublicRepository) ConsumePublicRate(ctx context.Context, definition int64, source, cookie [32]byte, window time.Time) error {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return unavailable(err)
	}
	for _, digest := range [][32]byte{source, cookie} {
		var count int
		err = tx.QueryRow(ctx, `INSERT INTO questionnaire_public_submission_rate_windows(definition_id,anonymous_digest,window_started_at,attempt_count) VALUES($1,$2,$3,1) ON CONFLICT(definition_id,anonymous_digest,window_started_at) DO UPDATE SET attempt_count=questionnaire_public_submission_rate_windows.attempt_count+1 WHERE questionnaire_public_submission_rate_windows.attempt_count<5 RETURNING attempt_count`, definition, digest[:], window).Scan(&count)
		if errors.Is(err, pgx.ErrNoRows) {
			return surveyapp.ErrPublicRateLimited
		}
		if err != nil || count < 1 || count > 5 {
			return unavailable(err)
		}
	}
	return nil
}
func (r *PublicRepository) CreatePublicSubmission(ctx context.Context, receiptID, definition int64, now time.Time, answers []surveyport.PublicSubmissionAnswer) (int64, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil || receiptID < 1 {
		return 0, unavailable(err)
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO questionnaire_public_submissions(receipt_id,definition_id,submitted_at,created_at) VALUES($1,$2,$3,$3) RETURNING id`, receiptID, definition, now).Scan(&id)
	if err != nil {
		return 0, unavailable(err)
	}
	for _, a := range answers {
		for _, o := range a.OptionIDs {
			tag, err := tx.Exec(ctx, `INSERT INTO questionnaire_public_submission_answers(submission_id,definition_question_id,definition_option_id) VALUES($1,$2,$3)`, id, a.QuestionID, o)
			if err != nil || tag.RowsAffected() != 1 {
				return 0, unavailable(err)
			}
		}
	}
	return id, nil
}
func (r *PublicRepository) CompletePublicReceipt(ctx context.Context, id int64, token [32]byte, snapshot json.RawMessage, now time.Time) (surveyapp.PublicReceipt, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return surveyapp.PublicReceipt{}, unavailable(err)
	}
	var x surveyapp.PublicReceipt
	var k, p, t []byte
	err = tx.QueryRow(ctx, `UPDATE questionnaire_public_submission_receipts SET result_token_digest=$2,result_snapshot=$3,state='completed',completed_at=$4 WHERE id=$1 AND state='in_progress' RETURNING id,definition_id,submission_key_digest,payload_digest,result_token_digest,state,result_snapshot`, id, token[:], snapshot, now).Scan(&x.ID, &x.DefinitionID, &k, &p, &t, &x.State, &x.ResultSnapshot)
	if err != nil {
		return x, unavailable(err)
	}
	copy(x.SubmissionKeyDigest[:], k)
	copy(x.PayloadDigest[:], p)
	copy(x.ResultTokenDigest[:], t)
	return x, nil
}
func (r *PublicRepository) LookupPublicResult(ctx context.Context, digest [32]byte) (surveyport.PublicSubmissionResult, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return surveyport.PublicSubmissionResult{}, unavailable(err)
	}
	var x surveyport.PublicSubmissionResult
	var stamp pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT s.id,d.definition_version,s.submitted_at FROM questionnaire_public_submission_receipts r JOIN questionnaire_public_submissions s ON s.receipt_id=r.id JOIN questionnaire_public_definitions d ON d.id=r.definition_id WHERE r.result_token_digest=$1 AND r.state='completed'`, digest[:]).Scan(&x.SubmissionID, &x.DefinitionVersion, &stamp)
	if err != nil {
		return x, publicDBErr(err)
	}
	x.SubmittedAt = stamp.Time
	x.LocalOnly = true
	x.ExternalExecuted = false
	return x, nil
}
func publicDBErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return surveyapp.ErrNotFound
	}
	return unavailable(err)
}
func (r *PublicRepository) PublicAnalytics(ctx context.Context, d surveyapp.PublicDefinitionRecord) (surveyport.PublicAnalytics, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return surveyport.PublicAnalytics{}, unavailable(err)
	}
	out := surveyport.PublicAnalytics{QuestionnaireID: d.View.ID, DefinitionVersion: d.View.Version, Questions: []surveyport.PublicAnalyticsQuestion{}}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM questionnaire_public_submissions WHERE definition_id=$1`, d.ID).Scan(&out.SubmissionCount)
	if err != nil {
		return out, unavailable(err)
	}
	for _, q := range d.View.Questions {
		aq := surveyport.PublicAnalyticsQuestion{QuestionID: q.ID, Type: q.Type, SortOrder: q.SortOrder, Options: []surveyport.PublicAnalyticsOption{}}
		if err = tx.QueryRow(ctx, `SELECT count(DISTINCT submission_id) FROM questionnaire_public_submission_answers WHERE definition_question_id=$1`, q.ID).Scan(&aq.AnsweredCount); err != nil {
			return out, unavailable(err)
		}
		for _, o := range q.Options {
			ao := surveyport.PublicAnalyticsOption{OptionID: o.ID, SortOrder: o.SortOrder}
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM questionnaire_public_submission_answers WHERE definition_question_id=$1 AND definition_option_id=$2`, q.ID, o.ID).Scan(&ao.SelectionCount); err != nil {
				return out, unavailable(err)
			}
			aq.Options = append(aq.Options, ao)
		}
		out.Questions = append(out.Questions, aq)
	}
	return out, nil
}
func (r *PublicRepository) ReservePublicManagement(ctx context.Context, op string, input surveyapp.PublicManagementReceipt, now time.Time) (surveyapp.PublicManagementReceipt, bool, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return surveyapp.PublicManagementReceipt{}, false, unavailable(err)
	}
	var out surveyapp.PublicManagementReceipt
	var key, payload []byte
	var complete *time.Time
	err = tx.QueryRow(ctx, `INSERT INTO questionnaire_public_management_receipts(operation,actor_scope,key_digest,payload_digest,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(operation,actor_scope,key_digest) DO NOTHING RETURNING id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot,completed_at`, op, input.ActorScope, input.KeyDigest[:], input.PayloadDigest[:], now).Scan(&out.ID, &out.Operation, &out.ActorScope, &key, &payload, &out.State, &out.ResultSnapshot, &complete)
	owned := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot,completed_at FROM questionnaire_public_management_receipts WHERE operation=$1 AND actor_scope=$2 AND key_digest=$3`, op, input.ActorScope, input.KeyDigest[:]).Scan(&out.ID, &out.Operation, &out.ActorScope, &key, &payload, &out.State, &out.ResultSnapshot, &complete)
	}
	if err != nil {
		return out, false, unavailable(err)
	}
	copy(out.KeyDigest[:], key)
	copy(out.PayloadDigest[:], payload)
	return out, owned, nil
}
func (r *PublicRepository) CompletePublicManagement(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (surveyapp.PublicManagementReceipt, error) {
	tx, err := publicTx(ctx)
	if r == nil || err != nil {
		return surveyapp.PublicManagementReceipt{}, unavailable(err)
	}
	var out surveyapp.PublicManagementReceipt
	var key, payload []byte
	err = tx.QueryRow(ctx, `UPDATE questionnaire_public_management_receipts SET result_snapshot=$2,state='completed',completed_at=$3 WHERE id=$1 AND state='in_progress' RETURNING id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot`, id, snapshot, now).Scan(&out.ID, &out.Operation, &out.ActorScope, &key, &payload, &out.State, &out.ResultSnapshot)
	if err != nil {
		return out, unavailable(err)
	}
	copy(out.KeyDigest[:], key)
	copy(out.PayloadDigest[:], payload)
	return out, nil
}

var _ = sha256.Size
