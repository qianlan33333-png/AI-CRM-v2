package campaign

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

// MemoryStore is a transaction-aware test fake. It is deliberately not an
// application adapter or a persistence implementation.
type MemoryStore struct {
	mu                                 sync.Mutex
	campaigns                          map[string]memoryCampaign
	receipts                           map[string]IdempotencyRecord
	audits                             []AuditEvent
	nextReceipt, nextPlan, nextCommand int64
	failAudit                          bool
}
type memoryCampaign struct {
	campaign Campaign
	steps    []Step
}
type memorySnapshot struct {
	campaigns                          map[string]memoryCampaign
	receipts                           map[string]IdempotencyRecord
	audits                             []AuditEvent
	nextReceipt, nextPlan, nextCommand int64
}

func NewMemoryStore(seed ...Campaign) *MemoryStore {
	m := &MemoryStore{campaigns: map[string]memoryCampaign{}, receipts: map[string]IdempotencyRecord{}, nextReceipt: 1, nextPlan: 1, nextCommand: 1}
	for _, c := range seed {
		m.campaigns[c.Code] = memoryCampaign{campaign: cloneCampaign(c)}
	}
	return m
}
func (m *MemoryStore) Within(_ context.Context, fn func(context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := m.snapshot()
	if err := fn(context.Background()); err != nil {
		m.restore(before)
		return err
	}
	return nil
}
func (m *MemoryStore) List(_ context.Context, in ListInput) ([]Campaign, error) {
	out := make([]Campaign, 0, len(m.campaigns))
	for _, v := range m.campaigns {
		if in.ApprovalStatus != nil && v.campaign.ApprovalStatus != *in.ApprovalStatus {
			continue
		}
		if in.RuntimeStatus != nil && v.campaign.RuntimeStatus != *in.RuntimeStatus {
			continue
		}
		out = append(out, cloneCampaign(v.campaign))
	}
	sortCampaigns(out)
	return out, nil
}
func (m *MemoryStore) Get(_ context.Context, code string) (Campaign, []Step, error) {
	v, ok := m.campaigns[code]
	if !ok {
		return Campaign{}, nil, ErrNotFound
	}
	return cloneCampaign(v.campaign), cloneSteps(v.steps), nil
}
func (m *MemoryStore) Lock(ctx context.Context, code string) (Campaign, []Step, error) {
	return m.Get(ctx, code)
}
func (m *MemoryStore) Save(_ context.Context, c Campaign, steps []Step) error {
	if _, ok := m.campaigns[c.Code]; !ok {
		return ErrNotFound
	}
	m.campaigns[c.Code] = memoryCampaign{campaign: cloneCampaign(c), steps: cloneSteps(steps)}
	return nil
}
func (m *MemoryStore) Delete(_ context.Context, code string, version int64) error {
	v, ok := m.campaigns[code]
	if !ok {
		return ErrNotFound
	}
	if v.campaign.Version != version {
		return ErrConflict
	}
	delete(m.campaigns, code)
	return nil
}
func (m *MemoryStore) CreateLocalPlanAndCommand(_ context.Context, c Campaign, count int32, op CommandOperation, now time.Time) (Plan, Command, error) {
	p := Plan{ID: m.nextPlan, CampaignCode: c.Code, CampaignVersion: c.Version, StepCount: count, CreatedAt: now}
	m.nextPlan++
	cmd := Command{ID: m.nextCommand, Operation: op, CampaignCode: c.Code, PlanID: p.ID, RealSend: false, RuntimeExecuted: false, CreatedAt: now}
	m.nextCommand++
	return p, cmd, nil
}
func (m *MemoryStore) ReserveIdempotency(_ context.Context, r Reservation) (IdempotencyRecord, bool, error) {
	key := receiptKey(r.ActorID, r.Operation, r.KeyDigest)
	if old, ok := m.receipts[key]; ok {
		if subtle.ConstantTimeCompare(old.PayloadDigest[:], r.PayloadDigest[:]) != 1 {
			return IdempotencyRecord{}, false, ErrIdempotencyConflict
		}
		if old.State != IdempotencyCompleted || old.Result == nil {
			return IdempotencyRecord{}, false, ErrUnavailable
		}
		return cloneReceipt(old), true, nil
	}
	record := IdempotencyRecord{ID: m.nextReceipt, ActorID: r.ActorID, Operation: r.Operation, KeyDigest: r.KeyDigest, PayloadDigest: r.PayloadDigest, State: IdempotencyReserved}
	m.nextReceipt++
	m.receipts[key] = record
	return cloneReceipt(record), false, nil
}
func (m *MemoryStore) CompleteIdempotency(_ context.Context, id int64, result OperationResult) error {
	for key, record := range m.receipts {
		if record.ID == id {
			if record.State != IdempotencyReserved {
				return ErrUnavailable
			}
			record.State = IdempotencyCompleted
			record.Result = cloneResult(result)
			m.receipts[key] = record
			return nil
		}
	}
	return ErrUnavailable
}
func (m *MemoryStore) Append(_ context.Context, event AuditEvent) error {
	if m.failAudit {
		return ErrUnavailable
	}
	m.audits = append(m.audits, event)
	return nil
}
func (m *MemoryStore) SeedSteps(code string, steps []Step) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.campaigns[code]
	v.steps = cloneSteps(steps)
	m.campaigns[code] = v
}
func (m *MemoryStore) AuditEvents() []AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]AuditEvent(nil), m.audits...)
}
func (m *MemoryStore) FailAudit(value bool) { m.mu.Lock(); defer m.mu.Unlock(); m.failAudit = value }
func (m *MemoryStore) snapshot() memorySnapshot {
	out := memorySnapshot{campaigns: map[string]memoryCampaign{}, receipts: map[string]IdempotencyRecord{}, audits: append([]AuditEvent(nil), m.audits...), nextReceipt: m.nextReceipt, nextPlan: m.nextPlan, nextCommand: m.nextCommand}
	for k, v := range m.campaigns {
		out.campaigns[k] = memoryCampaign{cloneCampaign(v.campaign), cloneSteps(v.steps)}
	}
	for k, v := range m.receipts {
		out.receipts[k] = cloneReceipt(v)
	}
	return out
}
func (m *MemoryStore) restore(s memorySnapshot) {
	m.campaigns = s.campaigns
	m.receipts = s.receipts
	m.audits = s.audits
	m.nextReceipt = s.nextReceipt
	m.nextPlan = s.nextPlan
	m.nextCommand = s.nextCommand
}
func receiptKey(actor int64, op string, digest [32]byte) string {
	return strconv.FormatInt(actor, 10) + "/" + op + "/" + hex.EncodeToString(digest[:])
}
func cloneResult(in OperationResult) *OperationResult {
	out := in
	if in.Mutation != nil {
		x := cloneMutation(*in.Mutation)
		out.Mutation = &x
	}
	if in.Delete != nil {
		x := *in.Delete
		out.Delete = &x
	}
	if in.Batch != nil {
		x := cloneBatch(*in.Batch)
		out.Batch = &x
	}
	return &out
}
func cloneReceipt(in IdempotencyRecord) IdempotencyRecord {
	out := in
	if in.Result != nil {
		out.Result = cloneResult(*in.Result)
	}
	return out
}
func sortCampaigns(items []Campaign) {
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			if items[j].Code < items[i].Code {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

var _ Repository = (*MemoryStore)(nil)
var _ UnitOfWork = (*MemoryStore)(nil)
var _ AuditAppender = (*MemoryStore)(nil)
