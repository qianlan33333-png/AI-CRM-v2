package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcdb "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type HXCHistoryStore struct{}
type HXCHistoryReader struct{ db hxcdb.DBTX }

var _ hxc.HXCHistoryStore = (*HXCHistoryStore)(nil)
var _ hxc.HXCHistoryReader = (*HXCHistoryReader)(nil)

func NewHXCHistoryStore() *HXCHistoryStore                { return &HXCHistoryStore{} }
func NewHXCHistoryReader(db hxcdb.DBTX) *HXCHistoryReader { return &HXCHistoryReader{db: db} }
func (s *HXCHistoryStore) q(ctx context.Context) (*hxcdb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, hxc.ErrHXCHistoryUnavailable
	}
	tx, e := platformstore.TxFromContext(ctx)
	if e != nil {
		return nil, hxc.ErrHXCHistoryUnavailable
	}
	return hxcdb.New(tx), nil
}
func (r *HXCHistoryReader) q(ctx context.Context) (*hxcdb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, hxc.ErrHXCHistoryUnavailable
	}
	if tx, e := platformstore.TxFromContext(ctx); e == nil {
		return hxcdb.New(tx), nil
	}
	if nilDB(r.db) {
		return nil, hxc.ErrHXCHistoryUnavailable
	}
	return hxcdb.New(r.db), nil
}
func nilDB(db hxcdb.DBTX) bool {
	if db == nil {
		return true
	}
	v := reflect.ValueOf(db)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func (s *HXCHistoryStore) CreateHistoricalHXCMeta(c context.Context, v hxc.HistoricalHXCMeta) (hxc.HistoricalHXCMeta, error) {
	if v.ID != 0 || badMeta(v) {
		return hxc.HistoricalHXCMeta{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCMeta{}, e
	}
	x, e := q.CreateHistoricalHXCMeta(c, hxcdb.CreateHistoricalHXCMetaParams{SourceID: v.SourceID, SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], StartedAt: ts(v.StartedAt), FinishedAt: pts(v.FinishedAt), Status: v.Status, RowCount: v.RowCount, MemberHit: v.MemberHit, UserHit: v.UserHit, OnlyMember: v.OnlyMember, TriggerSource: v.TriggerSource})
	if e != nil {
		return hxc.HistoricalHXCMeta{}, dbErr(e)
	}
	return meta(x)
}
func (s *HXCHistoryStore) GetHistoricalHXCMeta(c context.Context, id int64) (hxc.HistoricalHXCMeta, error) {
	q, e := s.q(c)
	if id < 1 {
		return hxc.HistoricalHXCMeta{}, hxc.ErrHXCHistoryInvalid
	}
	if e != nil {
		return hxc.HistoricalHXCMeta{}, e
	}
	x, e := q.GetHistoricalHXCMeta(c, id)
	if e != nil {
		return hxc.HistoricalHXCMeta{}, dbErr(e)
	}
	return meta(x)
}
func (r *HXCHistoryReader) GetHistoricalHXCMeta(c context.Context, id int64) (hxc.HistoricalHXCMeta, error) {
	if id < 1 {
		return hxc.HistoricalHXCMeta{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return hxc.HistoricalHXCMeta{}, e
	}
	x, e := q.GetHistoricalHXCMeta(c, id)
	if e != nil {
		return hxc.HistoricalHXCMeta{}, dbErr(e)
	}
	return meta(x)
}
func (r *HXCHistoryReader) ListHistoricalHXCMeta(c context.Context, x hxc.HXCHistoryQuery) ([]hxc.HistoricalHXCMeta, int64, error) {
	if badQ(x, false, false) {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return nil, 0, e
	}
	n, e := q.CountHistoricalHXCMeta(c)
	if e != nil {
		return nil, 0, dbErr(e)
	}
	rs, e := q.ListHistoricalHXCMeta(c, hxcdb.ListHistoricalHXCMetaParams{RowLimit: x.Limit, RowOffset: x.Offset})
	if e != nil {
		return nil, 0, dbErr(e)
	}
	out := make([]hxc.HistoricalHXCMeta, 0, len(rs))
	for _, z := range rs {
		v, e := meta(z)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *HXCHistoryStore) CreateHistoricalHXCSnapshot(c context.Context, v hxc.HistoricalHXCSnapshot) (hxc.HistoricalHXCSnapshot, error) {
	if v.ID != 0 || badSnapshot(v) {
		return hxc.HistoricalHXCSnapshot{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCSnapshot{}, e
	}
	x, e := q.CreateHistoricalHXCSnapshot(c, snapshotArg(v))
	if e != nil {
		return hxc.HistoricalHXCSnapshot{}, dbErr(e)
	}
	return snapshot(x)
}
func (s *HXCHistoryStore) GetHistoricalHXCSnapshot(c context.Context, id int64) (hxc.HistoricalHXCSnapshot, error) {
	if id < 1 {
		return hxc.HistoricalHXCSnapshot{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCSnapshot{}, e
	}
	x, e := q.GetHistoricalHXCSnapshot(c, id)
	if e != nil {
		return hxc.HistoricalHXCSnapshot{}, dbErr(e)
	}
	return snapshot(x)
}
func (r *HXCHistoryReader) GetHistoricalHXCSnapshot(c context.Context, id int64) (hxc.HistoricalHXCSnapshot, error) {
	if id < 1 {
		return hxc.HistoricalHXCSnapshot{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return hxc.HistoricalHXCSnapshot{}, e
	}
	x, e := q.GetHistoricalHXCSnapshot(c, id)
	if e != nil {
		return hxc.HistoricalHXCSnapshot{}, dbErr(e)
	}
	return snapshot(x)
}
func (r *HXCHistoryReader) ListHistoricalHXCSnapshot(c context.Context, x hxc.HXCHistoryQuery) ([]hxc.HistoricalHXCSnapshot, int64, error) {
	if badQ(x, true, false) {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return nil, 0, e
	}
	customer := i8(x.CustomerID)
	n, e := q.CountHistoricalHXCSnapshot(c, customer)
	if e != nil {
		return nil, 0, dbErr(e)
	}
	rs, e := q.ListHistoricalHXCSnapshot(c, hxcdb.ListHistoricalHXCSnapshotParams{CustomerID: customer, RowLimit: x.Limit, RowOffset: x.Offset})
	if e != nil {
		return nil, 0, dbErr(e)
	}
	out := make([]hxc.HistoricalHXCSnapshot, 0, len(rs))
	for _, z := range rs {
		v, e := snapshot(z)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *HXCHistoryStore) CreateHistoricalHXCActivation(c context.Context, v hxc.HistoricalHXCActivation) (hxc.HistoricalHXCActivation, error) {
	if v.ID != 0 || badActivation(v) {
		return hxc.HistoricalHXCActivation{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCActivation{}, e
	}
	x, e := q.CreateHistoricalHXCActivation(c, hxcdb.CreateHistoricalHXCActivationParams{SourceID: v.SourceID, SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], SourceTable: v.SourceTable, OriginalState: v.OriginalState, IsActive: v.IsActive, LegacyImportBatchRef: text(v.LegacyImportBatchRef), CreatedAt: ts(v.CreatedAt), UpdatedAt: ts(v.UpdatedAt)})
	if e != nil {
		return hxc.HistoricalHXCActivation{}, dbErr(e)
	}
	return activation(x)
}
func (s *HXCHistoryStore) GetHistoricalHXCActivation(c context.Context, id int64) (hxc.HistoricalHXCActivation, error) {
	if id < 1 {
		return hxc.HistoricalHXCActivation{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCActivation{}, e
	}
	x, e := q.GetHistoricalHXCActivation(c, id)
	if e != nil {
		return hxc.HistoricalHXCActivation{}, dbErr(e)
	}
	return activation(x)
}
func (r *HXCHistoryReader) GetHistoricalHXCActivation(c context.Context, id int64) (hxc.HistoricalHXCActivation, error) {
	if id < 1 {
		return hxc.HistoricalHXCActivation{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return hxc.HistoricalHXCActivation{}, e
	}
	x, e := q.GetHistoricalHXCActivation(c, id)
	if e != nil {
		return hxc.HistoricalHXCActivation{}, dbErr(e)
	}
	return activation(x)
}
func (r *HXCHistoryReader) ListHistoricalHXCActivation(c context.Context, x hxc.HXCHistoryQuery) ([]hxc.HistoricalHXCActivation, int64, error) {
	if badQ(x, false, true) {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return nil, 0, e
	}
	n, e := q.CountHistoricalHXCActivation(c, x.SourceTable)
	if e != nil {
		return nil, 0, dbErr(e)
	}
	rs, e := q.ListHistoricalHXCActivation(c, hxcdb.ListHistoricalHXCActivationParams{SourceTable: x.SourceTable, RowLimit: x.Limit, RowOffset: x.Offset})
	if e != nil {
		return nil, 0, dbErr(e)
	}
	out := make([]hxc.HistoricalHXCActivation, 0, len(rs))
	for _, z := range rs {
		v, e := activation(z)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *HXCHistoryStore) CreateHistoricalHXCLead(c context.Context, v hxc.HistoricalHXCLead) (hxc.HistoricalHXCLead, error) {
	if v.ID != 0 || badLead(v) {
		return hxc.HistoricalHXCLead{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCLead{}, e
	}
	x, e := q.CreateHistoricalHXCLead(c, hxcdb.CreateHistoricalHXCLeadParams{SourceID: v.SourceID, SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], OriginalType: v.OriginalType, IsActive: v.IsActive, LegacyImportBatchRef: text(v.LegacyImportBatchRef), CreatedAt: ts(v.CreatedAt), UpdatedAt: ts(v.UpdatedAt)})
	if e != nil {
		return hxc.HistoricalHXCLead{}, dbErr(e)
	}
	return lead(x)
}
func (s *HXCHistoryStore) GetHistoricalHXCLead(c context.Context, id int64) (hxc.HistoricalHXCLead, error) {
	if id < 1 {
		return hxc.HistoricalHXCLead{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCLead{}, e
	}
	x, e := q.GetHistoricalHXCLead(c, id)
	if e != nil {
		return hxc.HistoricalHXCLead{}, dbErr(e)
	}
	return lead(x)
}
func (r *HXCHistoryReader) GetHistoricalHXCLead(c context.Context, id int64) (hxc.HistoricalHXCLead, error) {
	if id < 1 {
		return hxc.HistoricalHXCLead{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return hxc.HistoricalHXCLead{}, e
	}
	x, e := q.GetHistoricalHXCLead(c, id)
	if e != nil {
		return hxc.HistoricalHXCLead{}, dbErr(e)
	}
	return lead(x)
}
func (r *HXCHistoryReader) ListHistoricalHXCLead(c context.Context, x hxc.HXCHistoryQuery) ([]hxc.HistoricalHXCLead, int64, error) {
	if badQ(x, false, false) {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return nil, 0, e
	}
	n, e := q.CountHistoricalHXCLead(c)
	if e != nil {
		return nil, 0, dbErr(e)
	}
	rs, e := q.ListHistoricalHXCLead(c, hxcdb.ListHistoricalHXCLeadParams{RowLimit: x.Limit, RowOffset: x.Offset})
	if e != nil {
		return nil, 0, dbErr(e)
	}
	out := make([]hxc.HistoricalHXCLead, 0, len(rs))
	for _, z := range rs {
		v, e := lead(z)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func (s *HXCHistoryStore) CreateHistoricalHXCBatch(c context.Context, v hxc.HistoricalHXCBatch) (hxc.HistoricalHXCBatch, error) {
	if v.ID != 0 || badBatch(v) {
		return hxc.HistoricalHXCBatch{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCBatch{}, e
	}
	x, e := q.CreateHistoricalHXCBatch(c, hxcdb.CreateHistoricalHXCBatchParams{SourceID: v.SourceID, SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], ImportType: v.ImportType, TotalRows: v.TotalRows, SuccessRows: v.SuccessRows, FailedRows: v.FailedRows, CreatedAt: ts(v.CreatedAt)})
	if e != nil {
		return hxc.HistoricalHXCBatch{}, dbErr(e)
	}
	return batch(x)
}
func (s *HXCHistoryStore) GetHistoricalHXCBatch(c context.Context, id int64) (hxc.HistoricalHXCBatch, error) {
	if id < 1 {
		return hxc.HistoricalHXCBatch{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := s.q(c)
	if e != nil {
		return hxc.HistoricalHXCBatch{}, e
	}
	x, e := q.GetHistoricalHXCBatch(c, id)
	if e != nil {
		return hxc.HistoricalHXCBatch{}, dbErr(e)
	}
	return batch(x)
}
func (r *HXCHistoryReader) GetHistoricalHXCBatch(c context.Context, id int64) (hxc.HistoricalHXCBatch, error) {
	if id < 1 {
		return hxc.HistoricalHXCBatch{}, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return hxc.HistoricalHXCBatch{}, e
	}
	x, e := q.GetHistoricalHXCBatch(c, id)
	if e != nil {
		return hxc.HistoricalHXCBatch{}, dbErr(e)
	}
	return batch(x)
}
func (r *HXCHistoryReader) ListHistoricalHXCBatch(c context.Context, x hxc.HXCHistoryQuery) ([]hxc.HistoricalHXCBatch, int64, error) {
	if badQ(x, false, false) {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	q, e := r.q(c)
	if e != nil {
		return nil, 0, e
	}
	n, e := q.CountHistoricalHXCBatch(c)
	if e != nil {
		return nil, 0, dbErr(e)
	}
	rs, e := q.ListHistoricalHXCBatch(c, hxcdb.ListHistoricalHXCBatchParams{RowLimit: x.Limit, RowOffset: x.Offset})
	if e != nil {
		return nil, 0, dbErr(e)
	}
	out := make([]hxc.HistoricalHXCBatch, 0, len(rs))
	for _, z := range rs {
		v, e := batch(z)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, n, nil
}

func ident(id, source int64, key, payload []byte) (hxc.HistoricalHXCIdentity, bool) {
	if id < 1 || len(key) != 32 || len(payload) != 32 {
		return hxc.HistoricalHXCIdentity{}, false
	}
	var v hxc.HistoricalHXCIdentity
	v.ID, v.SourceID = id, source
	copy(v.SourceKeyDigest[:], key)
	copy(v.SourcePayloadDigest[:], payload)
	return v, true
}
func tsv(v pgtype.Timestamptz) (time.Time, bool) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return v.Time.UTC().Truncate(time.Microsecond), true
}
func ptsv(v pgtype.Timestamptz) (*time.Time, bool) {
	if !v.Valid {
		return nil, true
	}
	x, ok := tsv(v)
	return &x, ok
}
func datev(v pgtype.Date) (*string, bool) {
	if !v.Valid {
		return nil, true
	}
	if v.InfinityModifier != pgtype.Finite {
		return nil, false
	}
	x := v.Time.Format("2006-01-02")
	return &x, true
}
func meta(x hxcdb.HxcV1DashboardRefreshHistory) (hxc.HistoricalHXCMeta, error) {
	id, ok := ident(x.ID, x.SourceID, x.SourceKeyDigest, x.SourcePayloadDigest)
	a, ok2 := tsv(x.StartedAt)
	b, ok3 := ptsv(x.FinishedAt)
	v := hxc.HistoricalHXCMeta{HistoricalHXCIdentity: id, StartedAt: a, FinishedAt: b, Status: x.Status, RowCount: x.RowCount, MemberHit: x.MemberHit, UserHit: x.UserHit, OnlyMember: x.OnlyMember, TriggerSource: x.TriggerSource}
	if !ok || !ok2 || !ok3 || badMeta(v) {
		return hxc.HistoricalHXCMeta{}, hxc.ErrHXCHistoryUnavailable
	}
	return v, nil
}
func snapshot(x hxcdb.HxcV1DashboardObservation) (hxc.HistoricalHXCSnapshot, error) {
	id, ok := ident(x.ID, x.SourceID, x.SourceKeyDigest, x.SourcePayloadDigest)
	a, o1 := tsv(x.ObservedAt)
	b, o2 := ptsv(x.HxcRegisteredAt)
	c, o3 := ptsv(x.HxcLastLoginAt)
	d, o4 := ptsv(x.MembershipEndAt)
	e, o5 := ptsv(x.LastMessageAt)
	f, o6 := ptsv(x.SubscriptionExpires)
	g, o7 := datev(x.CrmCreatedAt)
	h, o8 := datev(x.LastQuestionnaireAt)
	j, o9 := datev(x.SubscriptionPeriodStart)
	v := hxc.HistoricalHXCSnapshot{HistoricalHXCIdentity: id, CustomerID: i8v(x.CustomerID), Observation: x.Observation, ObservedAt: a, InLeadPool: x.InLeadPool, InPeople: x.InPeople, InQuestionnaire: x.InQuestionnaire, ClassTermNo: i8v(x.ClassTermNo), ClassTermLabel: x.ClassTermLabel, CRMHXCState: x.CrmHxcState, CRMCreatedAt: g, LastQuestionnaireAt: h, HXCMemberHit: x.HxcMemberHit, HXCUserHit: x.HxcUserHit, FunnelState: x.FunnelState, HXCMemberStatus: x.HxcMemberStatus, HXCRegisteredAt: b, HXCLastLoginAt: c, MembershipType: x.MembershipType, MembershipStatus: x.MembershipStatus, MembershipEndAt: d, MembershipDaysLeft: i8v(x.MembershipDaysLeft), ConsultationUsed: i8v(x.ConsultationUsed), ConsultationLimit: i8v(x.ConsultationLimit), ConversationChat: x.ConversationChat, ConversationConsult: x.ConversationConsult, ConversationLesson: x.ConversationLesson, MessagesUser: x.MessagesUser, MessagesAI: x.MessagesAi, ConsultCompleted: x.ConsultCompleted, LastMessageAt: e, SubscriptionTier: x.SubscriptionTier, SubscriptionExpires: f, SubscriptionQuota: i8v(x.SubscriptionQuota), SubscriptionUsed: i8v(x.SubscriptionUsed), SubscriptionPeriodStart: j}
	if !ok || !o1 || !o2 || !o3 || !o4 || !o5 || !o6 || !o7 || !o8 || !o9 || badSnapshot(v) {
		return hxc.HistoricalHXCSnapshot{}, hxc.ErrHXCHistoryUnavailable
	}
	return v, nil
}
func activation(x hxcdb.HxcV1ActivationObservation) (hxc.HistoricalHXCActivation, error) {
	id, ok := ident(x.ID, x.SourceID, x.SourceKeyDigest, x.SourcePayloadDigest)
	a, o1 := tsv(x.CreatedAt)
	b, o2 := tsv(x.UpdatedAt)
	v := hxc.HistoricalHXCActivation{HistoricalHXCIdentity: id, SourceTable: x.SourceTable, OriginalState: x.OriginalState, IsActive: x.IsActive, LegacyImportBatchRef: textv(x.LegacyImportBatchRef), CreatedAt: a, UpdatedAt: b}
	if !ok || !o1 || !o2 || badActivation(v) {
		return hxc.HistoricalHXCActivation{}, hxc.ErrHXCHistoryUnavailable
	}
	return v, nil
}
func lead(x hxcdb.HxcV1ExperienceLeadHistory) (hxc.HistoricalHXCLead, error) {
	id, ok := ident(x.ID, x.SourceID, x.SourceKeyDigest, x.SourcePayloadDigest)
	a, o1 := tsv(x.CreatedAt)
	b, o2 := tsv(x.UpdatedAt)
	v := hxc.HistoricalHXCLead{HistoricalHXCIdentity: id, OriginalType: x.OriginalType, IsActive: x.IsActive, LegacyImportBatchRef: textv(x.LegacyImportBatchRef), CreatedAt: a, UpdatedAt: b}
	if !ok || !o1 || !o2 || badLead(v) {
		return hxc.HistoricalHXCLead{}, hxc.ErrHXCHistoryUnavailable
	}
	return v, nil
}
func batch(x hxcdb.HxcV1ImportBatchHistory) (hxc.HistoricalHXCBatch, error) {
	id, ok := ident(x.ID, x.SourceID, x.SourceKeyDigest, x.SourcePayloadDigest)
	a, o := tsv(x.CreatedAt)
	v := hxc.HistoricalHXCBatch{HistoricalHXCIdentity: id, ImportType: x.ImportType, TotalRows: x.TotalRows, SuccessRows: x.SuccessRows, FailedRows: x.FailedRows, CreatedAt: a}
	if !ok || !o || badBatch(v) {
		return hxc.HistoricalHXCBatch{}, hxc.ErrHXCHistoryUnavailable
	}
	return v, nil
}
func snapshotArg(v hxc.HistoricalHXCSnapshot) hxcdb.CreateHistoricalHXCSnapshotParams {
	return hxcdb.CreateHistoricalHXCSnapshotParams{SourceID: v.SourceID, SourceKeyDigest: v.SourceKeyDigest[:], SourcePayloadDigest: v.SourcePayloadDigest[:], CustomerID: i8(v.CustomerID), Observation: v.Observation, ObservedAt: ts(v.ObservedAt), InLeadPool: v.InLeadPool, InPeople: v.InPeople, InQuestionnaire: v.InQuestionnaire, ClassTermNo: i8(v.ClassTermNo), ClassTermLabel: v.ClassTermLabel, CrmHxcState: v.CRMHXCState, CrmCreatedAt: text(v.CRMCreatedAt), LastQuestionnaireAt: text(v.LastQuestionnaireAt), HxcMemberHit: v.HXCMemberHit, HxcUserHit: v.HXCUserHit, FunnelState: v.FunnelState, HxcMemberStatus: v.HXCMemberStatus, HxcRegisteredAt: pts(v.HXCRegisteredAt), HxcLastLoginAt: pts(v.HXCLastLoginAt), MembershipType: v.MembershipType, MembershipStatus: v.MembershipStatus, MembershipEndAt: pts(v.MembershipEndAt), MembershipDaysLeft: i8(v.MembershipDaysLeft), ConsultationUsed: i8(v.ConsultationUsed), ConsultationLimit: i8(v.ConsultationLimit), ConversationChat: v.ConversationChat, ConversationConsult: v.ConversationConsult, ConversationLesson: v.ConversationLesson, MessagesUser: v.MessagesUser, MessagesAi: v.MessagesAI, ConsultCompleted: v.ConsultCompleted, LastMessageAt: pts(v.LastMessageAt), SubscriptionTier: v.SubscriptionTier, SubscriptionExpires: pts(v.SubscriptionExpires), SubscriptionQuota: i8(v.SubscriptionQuota), SubscriptionUsed: i8(v.SubscriptionUsed), SubscriptionPeriodStart: text(v.SubscriptionPeriodStart)}
}
func ts(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
func pts(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*v)
}
func i8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func i8v(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
func text(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}
func textv(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	x := v.String
	return &x
}
func badMeta(v hxc.HistoricalHXCMeta) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, e := hxcapp.HistoricalHXCMetaDigest(v)
	return e != nil
}
func badSnapshot(v hxc.HistoricalHXCSnapshot) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, e := hxcapp.HistoricalHXCSnapshotDigest(v)
	return e != nil
}
func badActivation(v hxc.HistoricalHXCActivation) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, e := hxcapp.HistoricalHXCActivationDigest(v)
	return e != nil
}
func badLead(v hxc.HistoricalHXCLead) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, e := hxcapp.HistoricalHXCLeadDigest(v)
	return e != nil
}
func badBatch(v hxc.HistoricalHXCBatch) bool {
	if v.ID == 0 {
		v.ID = 1
	}
	_, e := hxcapp.HistoricalHXCBatchDigest(v)
	return e != nil
}
func badQ(q hxc.HXCHistoryQuery, customer, source bool) bool {
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || (!customer && q.CustomerID != nil) || (!source && q.SourceTable != "") {
		return true
	}
	if q.CustomerID != nil && *q.CustomerID < 1 {
		return true
	}
	return source && q.SourceTable != "" && q.SourceTable != "public/user_ops_activation_status_source" && q.SourceTable != "public/user_ops_huangxiaocan_activation_source"
}
func dbErr(e error) error {
	var p *pgconn.PgError
	if errors.As(e, &p) && p.Code == "23505" {
		return hxc.ErrHXCHistoryConflict
	}
	return hxc.ErrHXCHistoryUnavailable
}
