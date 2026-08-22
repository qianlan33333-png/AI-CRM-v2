package campaign

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	uow   UnitOfWork
	repo  Repository
	audit AuditAppender
	now   func() time.Time
}

var _ Application = (*Service)(nil)

func NewService(uow UnitOfWork, repo Repository, audit AuditAppender) (*Service, error) {
	if nilish(uow) || nilish(repo) || nilish(audit) {
		return nil, ErrUnavailable
	}
	return &Service{uow: uow, repo: repo, audit: audit, now: time.Now}, nil
}

func (s *Service) List(ctx context.Context, input ListInput) (out ListResponse, err error) {
	if !validList(input) || !s.ready() {
		return out, invalidOrUnavailable(s.ready())
	}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		var items []Campaign
		items, err = s.repo.List(tx, input)
		if err == nil {
			out = ListResponse{Items: cloneCampaigns(items), Projection: projection()}
		}
		return err
	})
	return out, classify(err)
}
func (s *Service) Detail(ctx context.Context, code string) (out DetailResponse, err error) {
	if !validCode(code) || !s.ready() {
		return out, invalidOrUnavailable(s.ready())
	}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		var c Campaign
		var steps []Step
		c, steps, err = s.repo.Get(tx, code)
		if err == nil {
			if !validCampaign(c) || !validSteps(steps) {
				return ErrUnavailable
			}
			out = DetailResponse{Campaign: cloneCampaign(c), Steps: cloneSteps(steps), Projection: projection()}
		}
		return err
	})
	return out, classify(err)
}
func (s *Service) AddStep(ctx context.Context, c StepCreateCommand) (MutationResponse, error) {
	if !validVersioned(c.CampaignCode, c.ExpectedVersion, c.Actor, c.IdempotencyKey) || !validStepFields(c.DelayMinutes, c.Content) || !s.ready() {
		return MutationResponse{}, invalidOrUnavailable(s.ready())
	}
	return s.mutate(ctx, "step_add", c.Actor, c.IdempotencyKey, struct {
		Code    string
		Version int64
		Delay   int32
		Content string
	}{c.CampaignCode, c.ExpectedVersion, c.DelayMinutes, c.Content}, func(tx context.Context, now time.Time) (MutationResponse, error) {
		cur, steps, e := s.repo.Lock(tx, c.CampaignCode)
		if e != nil {
			return MutationResponse{}, e
		}
		if cur.Version != c.ExpectedVersion {
			return MutationResponse{}, ErrConflict
		}
		if cur.ApprovalStatus != ApprovalDraft || cur.RuntimeStatus != RuntimeIdle || len(steps) >= MaximumSteps {
			return MutationResponse{}, ErrStateConflict
		}
		steps = append(steps, Step{Index: int32(len(steps) + 1), DelayMinutes: c.DelayMinutes, Content: strings.TrimSpace(c.Content)})
		return s.saveAudit(tx, cur, steps, c.Actor, "step_added", c.IdempotencyKey, now)
	})
}
func (s *Service) UpdateStep(ctx context.Context, c StepUpdateCommand) (MutationResponse, error) {
	if !validVersioned(c.CampaignCode, c.ExpectedVersion, c.Actor, c.IdempotencyKey) || c.StepIndex < 1 || (c.DelayMinutes == nil && c.Content == nil) || !s.ready() {
		return MutationResponse{}, invalidOrUnavailable(s.ready())
	}
	if c.DelayMinutes != nil && !validDelay(*c.DelayMinutes) {
		return MutationResponse{}, Invalid("delay_minutes", "invalid")
	}
	if c.Content != nil && !validContent(*c.Content) {
		return MutationResponse{}, Invalid("content", "invalid")
	}
	return s.mutate(ctx, "step_update", c.Actor, c.IdempotencyKey, c, func(tx context.Context, now time.Time) (MutationResponse, error) {
		cur, steps, e := s.repo.Lock(tx, c.CampaignCode)
		if e != nil {
			return MutationResponse{}, e
		}
		if cur.Version != c.ExpectedVersion {
			return MutationResponse{}, ErrConflict
		}
		if cur.ApprovalStatus != ApprovalDraft || cur.RuntimeStatus != RuntimeIdle || int(c.StepIndex) > len(steps) {
			return MutationResponse{}, ErrStateConflict
		}
		if c.DelayMinutes != nil {
			steps[c.StepIndex-1].DelayMinutes = *c.DelayMinutes
		}
		if c.Content != nil {
			steps[c.StepIndex-1].Content = strings.TrimSpace(*c.Content)
		}
		return s.saveAudit(tx, cur, steps, c.Actor, "step_updated", c.IdempotencyKey, now)
	})
}
func (s *Service) DeleteStep(ctx context.Context, c StepDeleteCommand) (MutationResponse, error) {
	if !validVersioned(c.CampaignCode, c.ExpectedVersion, c.Actor, c.IdempotencyKey) || c.StepIndex < 1 || !s.ready() {
		return MutationResponse{}, invalidOrUnavailable(s.ready())
	}
	return s.mutate(ctx, "step_delete", c.Actor, c.IdempotencyKey, c, func(tx context.Context, now time.Time) (MutationResponse, error) {
		cur, steps, e := s.repo.Lock(tx, c.CampaignCode)
		if e != nil {
			return MutationResponse{}, e
		}
		if cur.Version != c.ExpectedVersion {
			return MutationResponse{}, ErrConflict
		}
		if cur.ApprovalStatus != ApprovalDraft || cur.RuntimeStatus != RuntimeIdle || int(c.StepIndex) > len(steps) {
			return MutationResponse{}, ErrStateConflict
		}
		steps = append(steps[:c.StepIndex-1], steps[c.StepIndex:]...)
		for i := range steps {
			steps[i].Index = int32(i + 1)
		}
		return s.saveAudit(tx, cur, steps, c.Actor, "step_deleted", c.IdempotencyKey, now)
	})
}
func (s *Service) Approve(ctx context.Context, c VersionedCommand) (MutationResponse, error) {
	return s.changeApproval(ctx, "approve", c, ApprovalDraft, ApprovalApproved, "approved")
}
func (s *Service) Reject(ctx context.Context, c VersionedCommand) (MutationResponse, error) {
	return s.changeApproval(ctx, "reject", c, ApprovalDraft, ApprovalRejected, "rejected")
}
func (s *Service) changeApproval(ctx context.Context, op string, c VersionedCommand, from, to ApprovalStatus, event string) (MutationResponse, error) {
	if !validVersioned(c.CampaignCode, c.ExpectedVersion, c.Actor, c.IdempotencyKey) || !s.ready() {
		return MutationResponse{}, invalidOrUnavailable(s.ready())
	}
	return s.mutate(ctx, op, c.Actor, c.IdempotencyKey, c, func(tx context.Context, now time.Time) (MutationResponse, error) {
		cur, steps, e := s.repo.Lock(tx, c.CampaignCode)
		if e != nil {
			return MutationResponse{}, e
		}
		if cur.Version != c.ExpectedVersion {
			return MutationResponse{}, ErrConflict
		}
		if cur.ApprovalStatus != from || cur.RuntimeStatus != RuntimeIdle {
			return MutationResponse{}, ErrStateConflict
		}
		cur.ApprovalStatus = to
		return s.saveAudit(tx, cur, steps, c.Actor, event, c.IdempotencyKey, now)
	})
}
func (s *Service) Pause(ctx context.Context, c VersionedCommand) (MutationResponse, error) {
	if !validVersioned(c.CampaignCode, c.ExpectedVersion, c.Actor, c.IdempotencyKey) || !s.ready() {
		return MutationResponse{}, invalidOrUnavailable(s.ready())
	}
	return s.mutate(ctx, "pause", c.Actor, c.IdempotencyKey, c, func(tx context.Context, now time.Time) (MutationResponse, error) {
		cur, steps, e := s.repo.Lock(tx, c.CampaignCode)
		if e != nil {
			return MutationResponse{}, e
		}
		if cur.Version != c.ExpectedVersion {
			return MutationResponse{}, ErrConflict
		}
		if cur.RuntimeStatus != RuntimePlanned {
			return MutationResponse{}, ErrStateConflict
		}
		cur.RuntimeStatus = RuntimePaused
		return s.saveAudit(tx, cur, steps, c.Actor, "paused", c.IdempotencyKey, now)
	})
}
func (s *Service) Start(ctx context.Context, c VersionedCommand) (MutationResponse, error) {
	if !validVersioned(c.CampaignCode, c.ExpectedVersion, c.Actor, c.IdempotencyKey) || !s.ready() {
		return MutationResponse{}, invalidOrUnavailable(s.ready())
	}
	return s.start(ctx, "start", c, CommandStart)
}
func (s *Service) start(ctx context.Context, op string, c VersionedCommand, operation CommandOperation) (MutationResponse, error) {
	return s.mutate(ctx, op, c.Actor, c.IdempotencyKey, c, func(tx context.Context, now time.Time) (MutationResponse, error) {
		cur, steps, e := s.repo.Lock(tx, c.CampaignCode)
		if e != nil {
			return MutationResponse{}, e
		}
		if cur.Version != c.ExpectedVersion {
			return MutationResponse{}, ErrConflict
		}
		if cur.ApprovalStatus != ApprovalApproved || cur.RuntimeStatus != RuntimeIdle || len(steps) == 0 {
			return MutationResponse{}, ErrStateConflict
		}
		cur.RuntimeStatus = RuntimePlanned
		cur.UpdatedBy = c.Actor.ID
		cur.UpdatedAt = now
		cur.Version++
		plan, cmd, e := s.repo.CreateLocalPlanAndCommand(tx, cur, int32(len(steps)), operation, now)
		if e != nil {
			return MutationResponse{}, e
		}
		if plan.CampaignCode != cur.Code || cmd.CampaignCode != cur.Code || cmd.PlanID != plan.ID || cmd.RealSend || cmd.RuntimeExecuted {
			return MutationResponse{}, ErrUnavailable
		}
		if e = s.repo.Save(tx, cur, steps); e != nil {
			return MutationResponse{}, e
		}
		if e = s.audit.Append(tx, AuditEvent{Type: "cloud_campaign.started", CampaignCode: cur.Code, ActorID: c.Actor.ID, IdempotencyKey: c.IdempotencyKey, OccurredAt: now}); e != nil {
			return MutationResponse{}, e
		}
		return MutationResponse{Campaign: cloneCampaign(cur), Plan: &plan, Command: &cmd, Projection: projection()}, nil
	})
}
func (s *Service) BatchStart(ctx context.Context, c BatchStartCommand) (out BatchStartResponse, err error) {
	if !validBatch(c) || !s.ready() {
		return out, invalidOrUnavailable(s.ready())
	}
	digest, e := digest(c.Items)
	if e != nil {
		return out, e
	}
	now := s.nowUTC()
	if now.IsZero() {
		return out, ErrUnavailable
	}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		r, re, e := s.reserve(tx, c.Actor, "batch_start", c.IdempotencyKey, digest, now)
		if e != nil {
			return e
		}
		if re {
			if r.Result == nil || r.Result.Batch == nil {
				return ErrUnavailable
			}
			out = cloneBatch(*r.Result.Batch)
			return nil
		}
		out = BatchStartResponse{Projection: projection()}
		for _, item := range c.Items {
			one, e := s.startDirect(tx, item, c.Actor, c.IdempotencyKey, now, CommandBatchStart)
			if e == nil {
				out.Started = append(out.Started, one)
			} else if errors.Is(e, ErrStateConflict) || errors.Is(e, ErrConflict) || errors.Is(e, ErrNotFound) {
				out.Skipped = append(out.Skipped, item)
			} else {
				out.Failed = append(out.Failed, item)
			}
		}
		if len(out.Failed) > 0 {
			return ErrUnavailable
		}
		return s.repo.CompleteIdempotency(tx, r.ID, OperationResult{Batch: &out})
	})
	return out, classify(err)
}
func (s *Service) startDirect(tx context.Context, item BatchStartItem, actor Actor, key string, now time.Time, op CommandOperation) (MutationResponse, error) {
	cur, steps, e := s.repo.Lock(tx, item.CampaignCode)
	if e != nil {
		return MutationResponse{}, e
	}
	if cur.Version != item.ExpectedVersion {
		return MutationResponse{}, ErrConflict
	}
	if cur.ApprovalStatus != ApprovalApproved || cur.RuntimeStatus != RuntimeIdle || len(steps) == 0 {
		return MutationResponse{}, ErrStateConflict
	}
	cur.RuntimeStatus = RuntimePlanned
	cur.UpdatedBy = actor.ID
	cur.UpdatedAt = now
	cur.Version++
	plan, cmd, e := s.repo.CreateLocalPlanAndCommand(tx, cur, int32(len(steps)), op, now)
	if e != nil {
		return MutationResponse{}, e
	}
	if cmd.RealSend || cmd.RuntimeExecuted {
		return MutationResponse{}, ErrUnavailable
	}
	if e = s.repo.Save(tx, cur, steps); e != nil {
		return MutationResponse{}, e
	}
	if e = s.audit.Append(tx, AuditEvent{Type: "cloud_campaign.batch_started", CampaignCode: cur.Code, ActorID: actor.ID, IdempotencyKey: key, OccurredAt: now}); e != nil {
		return MutationResponse{}, e
	}
	return MutationResponse{Campaign: cloneCampaign(cur), Plan: &plan, Command: &cmd, Projection: projection()}, nil
}
func (s *Service) Delete(ctx context.Context, c VersionedCommand) (out DeleteResponse, err error) {
	if !validVersioned(c.CampaignCode, c.ExpectedVersion, c.Actor, c.IdempotencyKey) || !s.ready() {
		return out, invalidOrUnavailable(s.ready())
	}
	digest, e := digest(c)
	if e != nil {
		return out, e
	}
	now := s.nowUTC()
	if now.IsZero() {
		return out, ErrUnavailable
	}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		r, re, e := s.reserve(tx, c.Actor, "delete", c.IdempotencyKey, digest, now)
		if e != nil {
			return e
		}
		if re {
			if r.Result == nil || r.Result.Delete == nil {
				return ErrUnavailable
			}
			out = *r.Result.Delete
			return nil
		}
		cur, _, e := s.repo.Lock(tx, c.CampaignCode)
		if e != nil {
			return e
		}
		if cur.Version != c.ExpectedVersion {
			return ErrConflict
		}
		if cur.ApprovalStatus != ApprovalDraft || cur.RuntimeStatus != RuntimeIdle {
			return ErrStateConflict
		}
		if e = s.repo.Delete(tx, cur.Code, cur.Version); e != nil {
			return e
		}
		if e = s.audit.Append(tx, AuditEvent{Type: "cloud_campaign.deleted", CampaignCode: cur.Code, ActorID: c.Actor.ID, IdempotencyKey: c.IdempotencyKey, OccurredAt: now}); e != nil {
			return e
		}
		out = DeleteResponse{CampaignCode: cur.Code, Deleted: true, Projection: projection()}
		return s.repo.CompleteIdempotency(tx, r.ID, OperationResult{Delete: &out})
	})
	return out, classify(err)
}

func (s *Service) saveAudit(tx context.Context, cur Campaign, steps []Step, actor Actor, event, key string, now time.Time) (MutationResponse, error) {
	cur.Version++
	cur.UpdatedBy = actor.ID
	cur.UpdatedAt = now
	if e := s.repo.Save(tx, cur, steps); e != nil {
		return MutationResponse{}, e
	}
	if e := s.audit.Append(tx, AuditEvent{Type: "cloud_campaign." + event, CampaignCode: cur.Code, ActorID: actor.ID, IdempotencyKey: key, OccurredAt: now}); e != nil {
		return MutationResponse{}, e
	}
	return MutationResponse{Campaign: cloneCampaign(cur), Projection: projection()}, nil
}
func (s *Service) mutate(ctx context.Context, op string, actor Actor, key string, payload any, fn func(context.Context, time.Time) (MutationResponse, error)) (out MutationResponse, err error) {
	d, e := digest(payload)
	if e != nil {
		return out, e
	}
	now := s.nowUTC()
	if now.IsZero() {
		return out, ErrUnavailable
	}
	err = s.uow.Within(ctx, func(tx context.Context) error {
		r, re, e := s.reserve(tx, actor, op, key, d, now)
		if e != nil {
			return e
		}
		if re {
			if r.Result == nil || r.Result.Mutation == nil {
				return ErrUnavailable
			}
			out = cloneMutation(*r.Result.Mutation)
			return nil
		}
		out, e = fn(tx, now)
		if e != nil {
			return e
		}
		return s.repo.CompleteIdempotency(tx, r.ID, OperationResult{Mutation: &out})
	})
	return out, classify(err)
}
func (s *Service) reserve(ctx context.Context, actor Actor, op, key string, payload [32]byte, now time.Time) (IdempotencyRecord, bool, error) {
	if !validKey(key) {
		return IdempotencyRecord{}, false, Invalid("idempotency_key", "invalid")
	}
	return s.repo.ReserveIdempotency(ctx, Reservation{ActorID: actor.ID, Operation: op, KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: payload, CreatedAt: now})
}
func (s *Service) ready() bool { return !nilish(s.uow) && !nilish(s.repo) && !nilish(s.audit) }
func (s *Service) nowUTC() time.Time {
	n := s.now().UTC()
	if n.IsZero() {
		return time.Time{}
	}
	return n
}
func validVersioned(code string, v int64, a Actor, key string) bool {
	return validCode(code) && v > 0 && a.ID > 0 && validKey(key)
}
func validCode(code string) bool {
	return code != "" && len(code) <= MaximumCampaignCodeBytes && code == strings.TrimSpace(code) && !strings.ContainsAny(code, "/\\\x00\r\n")
}
func validKey(key string) bool {
	return len(key) >= 16 && len(key) <= 128 && key == strings.TrimSpace(key)
}
func validList(in ListInput) bool {
	return (in.ApprovalStatus == nil || in.ApprovalStatus.Valid()) && (in.RuntimeStatus == nil || in.RuntimeStatus.Valid())
}
func validDelay(v int32) bool { return v >= 0 && v <= 525600 }
func validContent(v string) bool {
	return strings.TrimSpace(v) != "" && utf8.RuneCountInString(v) <= MaximumStepContentRunes
}
func validStepFields(d int32, c string) bool { return validDelay(d) && validContent(c) }
func validSteps(steps []Step) bool {
	if len(steps) > MaximumSteps {
		return false
	}
	for i, s := range steps {
		if s.Index != int32(i+1) || !validStepFields(s.DelayMinutes, s.Content) {
			return false
		}
	}
	return true
}
func validCampaign(c Campaign) bool {
	return validCode(c.Code) && strings.TrimSpace(c.Name) != "" && utf8.RuneCountInString(c.Name) <= MaximumCampaignNameRunes && c.ApprovalStatus.Valid() && c.RuntimeStatus.Valid() && c.Version > 0 && c.CreatedBy > 0 && c.UpdatedBy > 0 && !c.CreatedAt.IsZero() && !c.UpdatedAt.IsZero()
}
func validBatch(c BatchStartCommand) bool {
	if c.Actor.ID <= 0 || !validKey(c.IdempotencyKey) || len(c.Items) == 0 || len(c.Items) > MaximumBatch {
		return false
	}
	seen := map[string]bool{}
	for _, i := range c.Items {
		if !validCode(i.CampaignCode) || i.ExpectedVersion < 1 || seen[i.CampaignCode] {
			return false
		}
		seen[i.CampaignCode] = true
	}
	return true
}
func invalidOrUnavailable(ready bool) error {
	if !ready {
		return ErrUnavailable
	}
	return ErrInvalidArgument
}
func classify(e error) error {
	if e == nil {
		return nil
	}
	for _, v := range []error{ErrInvalidArgument, ErrNotFound, ErrConflict, ErrStateConflict, ErrIdempotencyConflict, ErrUnavailable} {
		if errors.Is(e, v) {
			return v
		}
	}
	return ErrUnavailable
}
func digest(value any) ([32]byte, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return [32]byte{}, ErrUnavailable
	}
	return sha256.Sum256(raw), nil
}
func nilish(v any) bool {
	if v == nil {
		return true
	}
	x := reflect.ValueOf(v)
	return (x.Kind() == reflect.Ptr || x.Kind() == reflect.Interface || x.Kind() == reflect.Map || x.Kind() == reflect.Slice || x.Kind() == reflect.Func) && x.IsNil()
}
func cloneCampaign(c Campaign) Campaign       { return c }
func cloneCampaigns(in []Campaign) []Campaign { out := append([]Campaign(nil), in...); return out }
func cloneSteps(in []Step) []Step             { return append([]Step(nil), in...) }
func cloneMutation(in MutationResponse) MutationResponse {
	out := in
	out.Campaign = cloneCampaign(in.Campaign)
	if in.Plan != nil {
		x := *in.Plan
		out.Plan = &x
	}
	if in.Command != nil {
		x := *in.Command
		out.Command = &x
	}
	return out
}
func cloneBatch(in BatchStartResponse) BatchStartResponse {
	out := in
	out.Started = make([]MutationResponse, len(in.Started))
	for i := range in.Started {
		out.Started[i] = cloneMutation(in.Started[i])
	}
	out.Skipped = append([]BatchStartItem(nil), in.Skipped...)
	out.Failed = append([]BatchStartItem(nil), in.Failed...)
	return out
}
