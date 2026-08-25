package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	releaseport "github.com/qianlan33333-png/AI-CRM-v2/internal/release/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidCommand = errors.New("invalid release command")
	ErrNotReady        = errors.New("release candidate is not ready")
	ErrInvalidState    = errors.New("invalid release state transition")
	ErrConflict        = errors.New("release operation conflict")
	ErrFence           = errors.New("release worker fence rejected")
)

type Service struct { uow platformport.UnitOfWork; store releaseport.Repository; now func() time.Time }

func NewService(uow platformport.UnitOfWork, store releaseport.Repository) *Service { return &Service{uow: uow, store: store, now: time.Now} }

type RegisterCommand struct { CommitSHA, ArtifactDigest, ManifestDigest, ConfigDigest string; TargetSchemaVersion, ActorID int64; IdempotencyKey string }
type ReceiptCommand struct { CandidateID, ActorID int64; Kind releaseport.PrerequisiteKind; EvidenceSHA, IdempotencyKey string }
type CandidateCommand struct { CandidateID, ActorID int64; IdempotencyKey string }
type StepCommand struct { CandidateID, ActorID, Generation int64; Fence string; Step releaseport.CutoverStep; IdempotencyKey string }

func (s *Service) Register(ctx context.Context, c RegisterCommand) (releaseport.Candidate, error) {
	if !validRegister(c) { return releaseport.Candidate{}, ErrInvalidCommand }
	return mutate(s, ctx, "candidate.register", c.ActorID, c.IdempotencyKey, c, func(tx context.Context, now time.Time) (releaseport.Candidate, error) {
		return s.store.CreateCandidate(tx, releaseport.Candidate{CommitSHA:c.CommitSHA, ArtifactDigest:c.ArtifactDigest, ManifestDigest:c.ManifestDigest, ConfigDigest:c.ConfigDigest, TargetSchemaVersion:c.TargetSchemaVersion, State:releaseport.CandidateDraft, CreatedBy:c.ActorID, CreatedAt:now})
	})
}

func (s *Service) Get(ctx context.Context, id int64) (releaseport.Candidate, error) { return s.store.GetCandidate(ctx, id) }
func (s *Service) List(ctx context.Context, limit int32) ([]releaseport.Candidate, error) { if limit < 1 || limit > 100 { return nil, ErrInvalidCommand }; return s.store.ListCandidates(ctx, limit) }

func (s *Service) RecordPrerequisite(ctx context.Context, c ReceiptCommand) (releaseport.PrerequisiteReceipt, error) {
	if c.CandidateID < 1 || c.ActorID < 1 || !knownPrerequisite(c.Kind) || !hexDigest(c.EvidenceSHA) || !validKey(c.IdempotencyKey) { return releaseport.PrerequisiteReceipt{}, ErrInvalidCommand }
	return mutate(s, ctx, "prerequisite.record", c.ActorID, c.IdempotencyKey, c, func(tx context.Context, now time.Time) (releaseport.PrerequisiteReceipt, error) {
		candidate, err := s.store.GetCandidate(tx, c.CandidateID); if err != nil { return releaseport.PrerequisiteReceipt{}, err }; if candidate.State != releaseport.CandidateDraft { return releaseport.PrerequisiteReceipt{}, ErrInvalidState }
		return s.store.CreatePrerequisite(tx, releaseport.PrerequisiteReceipt{CandidateID:c.CandidateID, Kind:c.Kind, EvidenceSHA:c.EvidenceSHA, RecordedBy:c.ActorID, RecordedAt:now})
	})
}

func (s *Service) Readiness(ctx context.Context, id int64) (releaseport.Readiness, error) {
	if id < 1 { return releaseport.Readiness{}, ErrInvalidCommand }; if _, err := s.store.GetCandidate(ctx, id); err != nil { return releaseport.Readiness{}, err }
	receipts, err := s.store.ListPrerequisites(ctx, id); if err != nil { return releaseport.Readiness{}, err }
	have := map[releaseport.PrerequisiteKind]bool{}; for _, receipt := range receipts { have[receipt.Kind] = true }
	missing := make([]releaseport.PrerequisiteKind, 0); for _, kind := range requiredPrerequisites { if !have[kind] { missing = append(missing, kind) } }
	return releaseport.Readiness{CandidateID:id, Ready:len(missing)==0, Missing:missing, CheckedAt:s.now().UTC()}, nil
}

func (s *Service) Prepare(ctx context.Context, c CandidateCommand) (releaseport.Candidate, error) {
	if !validCandidateCommand(c) { return releaseport.Candidate{}, ErrInvalidCommand }
	return mutate(s, ctx, "candidate.prepare", c.ActorID, c.IdempotencyKey, c, func(tx context.Context, now time.Time) (releaseport.Candidate, error) {
		candidate, err := s.store.GetCandidate(tx,c.CandidateID); if err != nil { return releaseport.Candidate{},err }; if candidate.State != releaseport.CandidateDraft { return releaseport.Candidate{},ErrInvalidState }
		ready, err := s.Readiness(tx,c.CandidateID); if err != nil { return releaseport.Candidate{},err }; if !ready.Ready { return releaseport.Candidate{},ErrNotReady }
		return s.store.UpdateState(tx,c.CandidateID,releaseport.CandidatePrepared,now)
	})
}

func (s *Service) StartCutover(ctx context.Context, c CandidateCommand) (releaseport.WorkerLease, error) {
	if !validCandidateCommand(c) { return releaseport.WorkerLease{},ErrInvalidCommand }
	return mutate(s,ctx,"cutover.start",c.ActorID,c.IdempotencyKey,c,func(tx context.Context,now time.Time)(releaseport.WorkerLease,error){
		candidate,err:=s.store.GetCandidate(tx,c.CandidateID); if err!=nil{return releaseport.WorkerLease{},err}; if candidate.State!=releaseport.CandidatePrepared{return releaseport.WorkerLease{},ErrInvalidState}
		lease,err:=s.store.StartWorker(tx,releaseport.WorkerLease{CandidateID:c.CandidateID,StartedBy:c.ActorID,StartedAt:now,Active:true}); if err!=nil{return releaseport.WorkerLease{},err}
		if _,err=s.store.UpdateState(tx,c.CandidateID,releaseport.CandidateCutoverActive,now);err!=nil{return releaseport.WorkerLease{},err}; return lease,nil
	})
}

func (s *Service) CompleteStep(ctx context.Context, c StepCommand) (releaseport.CutoverJournalEntry,error) {
	if c.CandidateID<1||c.ActorID<1||c.Generation<1||!knownStep(c.Step)||!validFence(c.Fence)||!validKey(c.IdempotencyKey){return releaseport.CutoverJournalEntry{},ErrInvalidCommand}
	return mutate(s,ctx,"cutover.step.complete",c.ActorID,c.IdempotencyKey,c,func(tx context.Context,now time.Time)(releaseport.CutoverJournalEntry,error){
		candidate,err:=s.store.GetCandidate(tx,c.CandidateID);if err!=nil{return releaseport.CutoverJournalEntry{},err};if candidate.State!=releaseport.CandidateCutoverActive{return releaseport.CutoverJournalEntry{},ErrInvalidState}
		lease,err:=s.store.GetWorker(tx,c.CandidateID);if err!=nil{return releaseport.CutoverJournalEntry{},err};if !lease.Active||lease.Generation!=c.Generation||lease.Fence!=c.Fence{return releaseport.CutoverJournalEntry{},ErrFence}
		entries,err:=s.store.ListCutoverSteps(tx,c.CandidateID);if err!=nil{return releaseport.CutoverJournalEntry{},err};if len(entries)>=len(releaseport.FixedCutoverSteps)||releaseport.FixedCutoverSteps[len(entries)]!=c.Step{return releaseport.CutoverJournalEntry{},ErrInvalidState}
		return s.store.AppendCutoverStep(tx,releaseport.CutoverJournalEntry{CandidateID:c.CandidateID,Step:c.Step,Fence:c.Fence,CompletedBy:c.ActorID,CompletedAt:now})
	})
}

func (s *Service) Activate(ctx context.Context,c CandidateCommand)(releaseport.Candidate,error){
	if !validCandidateCommand(c){return releaseport.Candidate{},ErrInvalidCommand};return mutate(s,ctx,"candidate.activate",c.ActorID,c.IdempotencyKey,c,func(tx context.Context,now time.Time)(releaseport.Candidate,error){
		candidate,err:=s.store.GetCandidate(tx,c.CandidateID);if err!=nil{return releaseport.Candidate{},err};if candidate.State!=releaseport.CandidateCutoverActive{return releaseport.Candidate{},ErrInvalidState}; entries,err:=s.store.ListCutoverSteps(tx,c.CandidateID);if err!=nil{return releaseport.Candidate{},err};if len(entries)!=len(releaseport.FixedCutoverSteps){return releaseport.Candidate{},ErrNotReady};return s.store.UpdateState(tx,c.CandidateID,releaseport.CandidateActivated,now)
	})
}
func (s *Service) RequestRollback(ctx context.Context,c CandidateCommand)(releaseport.Candidate,error){return s.transition(ctx,"rollback.request",c,releaseport.CandidateActivated,releaseport.CandidateRollbackPending)}
func (s *Service) CompleteRollback(ctx context.Context,c CandidateCommand)(releaseport.Candidate,error){return s.transition(ctx,"rollback.complete",c,releaseport.CandidateRollbackPending,releaseport.CandidateRolledBack)}
func (s *Service) transition(ctx context.Context,action string,c CandidateCommand,from,to releaseport.CandidateState)(releaseport.Candidate,error){if !validCandidateCommand(c){return releaseport.Candidate{},ErrInvalidCommand};return mutate(s,ctx,action,c.ActorID,c.IdempotencyKey,c,func(tx context.Context,now time.Time)(releaseport.Candidate,error){current,err:=s.store.GetCandidate(tx,c.CandidateID);if err!=nil{return releaseport.Candidate{},err};if current.State!=from{return releaseport.Candidate{},ErrInvalidState};return s.store.UpdateState(tx,c.CandidateID,to,now)})}

var requiredPrerequisites=[]releaseport.PrerequisiteKind{releaseport.PrerequisiteNightly,releaseport.PrerequisiteBackupRestoreDrill,releaseport.PrerequisiteMigration,releaseport.PrerequisiteContactClosure,releaseport.PrerequisiteCampaignClosure,releaseport.PrerequisiteOutboundClosure,releaseport.PrerequisiteCommerceClosure}
func knownPrerequisite(k releaseport.PrerequisiteKind)bool{for _,x:=range requiredPrerequisites{if x==k{return true}};return false}
func knownStep(s releaseport.CutoverStep)bool{for _,x:=range releaseport.FixedCutoverSteps{if x==s{return true}};return false}
func validRegister(c RegisterCommand)bool{return c.ActorID>0&&c.TargetSchemaVersion>0&&sha40(c.CommitSHA)&&hexDigest(c.ArtifactDigest)&&hexDigest(c.ManifestDigest)&&hexDigest(c.ConfigDigest)&&validKey(c.IdempotencyKey)}
func validCandidateCommand(c CandidateCommand)bool{return c.CandidateID>0&&c.ActorID>0&&validKey(c.IdempotencyKey)}
func sha40(v string)bool{return len(v)==40&&v==strings.ToLower(v)&&isHex(v)}
func hexDigest(v string)bool{return len(v)==64&&v==strings.ToLower(v)&&isHex(v)}
func isHex(v string)bool{_,err:=hex.DecodeString(v);return err==nil}
func validFence(v string)bool{return len(v)==64&&hexDigest(v)}
func validKey(v string)bool{return len(v)>=16&&len(v)<=128&&strings.TrimSpace(v)==v}
func mutate[T any](s *Service,ctx context.Context,action string,actor int64,key string,payload any,fn func(context.Context,time.Time)(T,error))(T,error){var zero T;if s==nil||s.uow==nil||s.store==nil{return zero,ErrInvalidCommand};raw,err:=json.Marshal(payload);if err!=nil{return zero,ErrInvalidCommand};keyDigest:=digest(key);payloadDigest:=digest(string(raw));var result T;err=s.uow.Within(ctx,func(tx context.Context)error{receipt,created,receiptErr:=s.store.ReserveOperationReceipt(tx,releaseport.OperationReceipt{Action:action,ActorID:actor,KeyDigest:keyDigest,PayloadDigest:payloadDigest,State:"in_progress"});if receiptErr!=nil{return receiptErr};if !created{if receipt.PayloadDigest!=payloadDigest{return ErrConflict};if receipt.State!="completed"{return ErrConflict};return json.Unmarshal(receipt.Result,&result)};result,receiptErr=fn(tx,s.now().UTC());if receiptErr!=nil{return receiptErr};out,marshalErr:=json.Marshal(result);if marshalErr!=nil{return marshalErr};_,receiptErr=s.store.CompleteOperationReceipt(tx,receipt.ID,out);return receiptErr});if err!=nil{return zero,err};return result,nil}
func digest(v string)string{sum:=sha256.Sum256([]byte(v));return fmt.Sprintf("%x",sum[:])}
func SortMissing(values []releaseport.PrerequisiteKind){sort.Slice(values,func(i,j int)bool{return values[i]<values[j]})}
