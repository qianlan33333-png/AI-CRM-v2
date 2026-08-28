package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	survey "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
	surveydb "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/store/generated"
)

type SurveyUnresolvedHistoryStore struct{}
type SurveyUnresolvedHistoryReader struct{ db surveydb.DBTX }

var _ survey.SurveyUnresolvedHistoryStore = (*SurveyUnresolvedHistoryStore)(nil)
var _ survey.SurveyUnresolvedHistoryReader = (*SurveyUnresolvedHistoryReader)(nil)

func NewSurveyUnresolvedHistoryStore() *SurveyUnresolvedHistoryStore {
	return &SurveyUnresolvedHistoryStore{}
}
func NewSurveyUnresolvedHistoryReader(db surveydb.DBTX) *SurveyUnresolvedHistoryReader {
	return &SurveyUnresolvedHistoryReader{db: db}
}

func (s *SurveyUnresolvedHistoryStore) q(ctx context.Context) (*surveydb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	return surveydb.New(tx), nil
}
func (r *SurveyUnresolvedHistoryReader) q(ctx context.Context) (*surveydb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return surveydb.New(tx), nil
	}
	if nilSurveyDB(r.db) {
		return nil, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	return surveydb.New(r.db), nil
}
func nilSurveyDB(value surveydb.DBTX) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func (s *SurveyUnresolvedHistoryStore) CreateHistoricalUnresolvedSurveySubmission(ctx context.Context, v survey.HistoricalUnresolvedSurveySubmission) (survey.HistoricalUnresolvedSurveySubmission, error) {
	if v.ID != 0 || badSubmission(v) {
		return survey.HistoricalUnresolvedSurveySubmission{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return survey.HistoricalUnresolvedSurveySubmission{}, err
	}
	row, err := q.CreateHistoricalUnresolvedSurveySubmission(ctx, surveydb.CreateHistoricalUnresolvedSurveySubmissionParams{
		SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SourceID: v.SourceID, QuestionnaireSourceID: v.QuestionnaireSourceID,
		QuestionnaireID: optionalInt(v.QuestionnaireID), CustomerID: optionalInt(v.CustomerID), MatchedBy: v.MatchedBy, SourceChannel: v.SourceChannel, TotalScore: v.TotalScore, FinalTags: v.FinalTags,
		SubmittedAt: unresolvedTimestamp(v.SubmittedAt), CreatedAt: unresolvedTimestamp(v.CreatedAt), UnionIDDigest: v.UnionIDDigest[:], FollowUserUserIDDigest: v.FollowUserUserIDDigest[:], CampaignIDDigest: v.CampaignIDDigest[:], StaffIDDigest: v.StaffIDDigest[:], RedirectUrlDigest: v.RedirectURLDigest[:], AssessmentResultDigest: v.AssessmentResultDigest[:],
	})
	if err != nil {
		return survey.HistoricalUnresolvedSurveySubmission{}, unresolvedDBError(err)
	}
	return submissionValue(row)
}
func (s *SurveyUnresolvedHistoryStore) GetHistoricalUnresolvedSurveySubmission(ctx context.Context, id int64) (survey.HistoricalUnresolvedSurveySubmission, error) {
	q, err := s.q(ctx)
	if id < 1 {
		return survey.HistoricalUnresolvedSurveySubmission{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	if err != nil {
		return survey.HistoricalUnresolvedSurveySubmission{}, err
	}
	row, err := q.GetHistoricalUnresolvedSurveySubmission(ctx, id)
	if err != nil {
		return survey.HistoricalUnresolvedSurveySubmission{}, unresolvedDBError(err)
	}
	return submissionValue(row)
}
func (r *SurveyUnresolvedHistoryReader) GetHistoricalUnresolvedSurveySubmission(ctx context.Context, id int64) (survey.HistoricalUnresolvedSurveySubmission, error) {
	q, err := r.q(ctx)
	if id < 1 {
		return survey.HistoricalUnresolvedSurveySubmission{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	if err != nil {
		return survey.HistoricalUnresolvedSurveySubmission{}, err
	}
	row, err := q.GetHistoricalUnresolvedSurveySubmission(ctx, id)
	if err != nil {
		return survey.HistoricalUnresolvedSurveySubmission{}, unresolvedDBError(err)
	}
	return submissionValue(row)
}

func (r *SurveyUnresolvedHistoryReader) ListHistoricalUnresolvedSurveySubmissions(ctx context.Context, x survey.SurveyUnresolvedHistoryQuery) ([]survey.HistoricalUnresolvedSurveySubmission, int64, error) {
	if badQuery(x) {
		return nil, 0, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalUnresolvedSurveySubmissions(ctx, optionalInt(x.QuestionnaireID))
	if err != nil {
		return nil, 0, unresolvedDBError(err)
	}
	rows, err := q.ListHistoricalUnresolvedSurveySubmissions(ctx, surveydb.ListHistoricalUnresolvedSurveySubmissionsParams{QuestionnaireID: optionalInt(x.QuestionnaireID), RowLimit: x.Limit, RowOffset: x.Offset})
	if err != nil {
		return nil, 0, unresolvedDBError(err)
	}
	out := make([]survey.HistoricalUnresolvedSurveySubmission, 0, len(rows))
	for _, row := range rows {
		v, err := submissionValue(row)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *SurveyUnresolvedHistoryStore) CreateHistoricalUnresolvedSurveyAnswer(ctx context.Context, v survey.HistoricalUnresolvedSurveyAnswer) (survey.HistoricalUnresolvedSurveyAnswer, error) {
	if v.ID != 0 || badAnswer(v) {
		return survey.HistoricalUnresolvedSurveyAnswer{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	q, err := s.q(ctx)
	if err != nil {
		return survey.HistoricalUnresolvedSurveyAnswer{}, err
	}
	row, err := q.CreateHistoricalUnresolvedSurveyAnswer(ctx, surveydb.CreateHistoricalUnresolvedSurveyAnswerParams{SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceFieldDigest: v.SourceFieldDigest[:], SourceID: v.SourceID, SubmissionID: v.SubmissionID, SubmissionSourceID: v.SubmissionSourceID, QuestionSourceID: v.QuestionSourceID, QuestionType: v.QuestionType, QuestionTitleSnapshot: v.QuestionTitleSnapshot, SelectedOptionIds: v.SelectedOptionIDs, SelectedOptionTexts: v.SelectedOptionTexts, SelectedOptionScores: v.SelectedOptionScores, SelectedOptionTags: v.SelectedOptionTags, TextValue: v.TextValue, ScoreContribution: v.ScoreContribution, CreatedAt: unresolvedTimestamp(v.CreatedAt)})
	if err != nil {
		return survey.HistoricalUnresolvedSurveyAnswer{}, unresolvedDBError(err)
	}
	return answerValue(row)
}
func (s *SurveyUnresolvedHistoryStore) GetHistoricalUnresolvedSurveyAnswer(ctx context.Context, id int64) (survey.HistoricalUnresolvedSurveyAnswer, error) {
	q, err := s.q(ctx)
	if id < 1 {
		return survey.HistoricalUnresolvedSurveyAnswer{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	if err != nil {
		return survey.HistoricalUnresolvedSurveyAnswer{}, err
	}
	row, err := q.GetHistoricalUnresolvedSurveyAnswer(ctx, id)
	if err != nil {
		return survey.HistoricalUnresolvedSurveyAnswer{}, unresolvedDBError(err)
	}
	return answerValue(row)
}
func (r *SurveyUnresolvedHistoryReader) GetHistoricalUnresolvedSurveyAnswer(ctx context.Context, id int64) (survey.HistoricalUnresolvedSurveyAnswer, error) {
	if id < 1 {
		return survey.HistoricalUnresolvedSurveyAnswer{}, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return survey.HistoricalUnresolvedSurveyAnswer{}, err
	}
	row, err := q.GetHistoricalUnresolvedSurveyAnswer(ctx, id)
	if err != nil {
		return survey.HistoricalUnresolvedSurveyAnswer{}, unresolvedDBError(err)
	}
	return answerValue(row)
}

func (r *SurveyUnresolvedHistoryReader) ListHistoricalUnresolvedSurveyAnswers(ctx context.Context, submissionID int64, x survey.SurveyUnresolvedHistoryQuery) ([]survey.HistoricalUnresolvedSurveyAnswer, int64, error) {
	if submissionID < 1 || badQuery(x) {
		return nil, 0, survey.ErrSurveyUnresolvedHistoryInvalid
	}
	q, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	n, err := q.CountHistoricalUnresolvedSurveyAnswers(ctx, submissionID)
	if err != nil {
		return nil, 0, unresolvedDBError(err)
	}
	rows, err := q.ListHistoricalUnresolvedSurveyAnswers(ctx, surveydb.ListHistoricalUnresolvedSurveyAnswersParams{SubmissionID: submissionID, RowLimit: x.Limit, RowOffset: x.Offset})
	if err != nil {
		return nil, 0, unresolvedDBError(err)
	}
	out := make([]survey.HistoricalUnresolvedSurveyAnswer, 0, len(rows))
	for _, row := range rows {
		v, err := answerValue(row)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, n, nil
}

func unresolvedTimestamp(v time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: v, Valid: true}
}
func timeValue(v pgtype.Timestamptz) (time.Time, bool) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return v.Time.UTC().Truncate(time.Microsecond), true
}
func optionalInt(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func intValue(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}
func digest3(a, b, c []byte) ([32]byte, [32]byte, [32]byte, bool) {
	var x, y, z [32]byte
	if len(a) != 32 || len(b) != 32 || len(c) != 32 {
		return x, y, z, false
	}
	copy(x[:], a)
	copy(y[:], b)
	copy(z[:], c)
	return x, y, z, x != ([32]byte{}) && y != ([32]byte{}) && z != ([32]byte{})
}
func digest6(values ...[]byte) ([32]byte, [32]byte, [32]byte, [32]byte, [32]byte, [32]byte, bool) {
	var a, b, c, d, e, f [32]byte
	if len(values) != 6 {
		return a, b, c, d, e, f, false
	}
	for _, value := range values {
		if len(value) != 32 {
			return a, b, c, d, e, f, false
		}
	}
	copy(a[:], values[0])
	copy(b[:], values[1])
	copy(c[:], values[2])
	copy(d[:], values[3])
	copy(e[:], values[4])
	copy(f[:], values[5])
	return a, b, c, d, e, f, a != ([32]byte{}) && b != ([32]byte{}) && c != ([32]byte{}) && d != ([32]byte{}) && e != ([32]byte{}) && f != ([32]byte{})
}
func submissionValue(r surveydb.SurveyV1UnresolvedSubmission) (survey.HistoricalUnresolvedSurveySubmission, error) {
	key, payload, field, ok := digest3(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest)
	union, follow, campaign, staff, redirect, assessment, privateOK := digest6(r.UnionIDDigest, r.FollowUserUserIDDigest, r.CampaignIDDigest, r.StaffIDDigest, r.RedirectUrlDigest, r.AssessmentResultDigest)
	submitted, submittedOK := timeValue(r.SubmittedAt)
	created, createdOK := timeValue(r.CreatedAt)
	v := survey.HistoricalUnresolvedSurveySubmission{ID: r.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: r.SourceID, QuestionnaireSourceID: r.QuestionnaireSourceID, QuestionnaireID: intValue(r.QuestionnaireID), CustomerID: intValue(r.CustomerID), MatchedBy: r.MatchedBy, SourceChannel: r.SourceChannel, TotalScore: r.TotalScore, FinalTags: r.FinalTags, SubmittedAt: submitted, CreatedAt: created, UnionIDDigest: union, FollowUserUserIDDigest: follow, CampaignIDDigest: campaign, StaffIDDigest: staff, RedirectURLDigest: redirect, AssessmentResultDigest: assessment}
	if !ok || !privateOK || !submittedOK || !createdOK || badSubmission(v) {
		return survey.HistoricalUnresolvedSurveySubmission{}, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	return v, nil
}
func answerValue(r surveydb.SurveyV1UnresolvedAnswer) (survey.HistoricalUnresolvedSurveyAnswer, error) {
	key, payload, field, ok := digest3(r.SourceKeyDigest, r.SourcePayloadDigest, r.SourceFieldDigest)
	created, createdOK := timeValue(r.CreatedAt)
	v := survey.HistoricalUnresolvedSurveyAnswer{ID: r.ID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, SourceID: r.SourceID, SubmissionID: r.SubmissionID, SubmissionSourceID: r.SubmissionSourceID, QuestionSourceID: r.QuestionSourceID, QuestionType: r.QuestionType, QuestionTitleSnapshot: r.QuestionTitleSnapshot, SelectedOptionIDs: r.SelectedOptionIds, SelectedOptionTexts: r.SelectedOptionTexts, SelectedOptionScores: r.SelectedOptionScores, SelectedOptionTags: r.SelectedOptionTags, TextValue: r.TextValue, ScoreContribution: r.ScoreContribution, CreatedAt: created}
	if !ok || !createdOK || badAnswer(v) {
		return survey.HistoricalUnresolvedSurveyAnswer{}, survey.ErrSurveyUnresolvedHistoryUnavailable
	}
	return v, nil
}
func badSubmission(v survey.HistoricalUnresolvedSurveySubmission) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, err := surveyapp.HistoricalUnresolvedSurveySubmissionDigest(v)
	return err != nil
}
func badAnswer(v survey.HistoricalUnresolvedSurveyAnswer) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, err := surveyapp.HistoricalUnresolvedSurveyAnswerDigest(v)
	return err != nil
}
func badQuery(x survey.SurveyUnresolvedHistoryQuery) bool {
	return x.Limit < 1 || x.Limit > 100 || x.Offset < 0 || (x.QuestionnaireID != nil && *x.QuestionnaireID < 1)
}
func unresolvedDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return survey.ErrSurveyUnresolvedHistoryConflict
	}
	return survey.ErrSurveyUnresolvedHistoryUnavailable
}
