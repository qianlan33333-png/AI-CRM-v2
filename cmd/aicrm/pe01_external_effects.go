package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

type pe01TerminalReader interface {
	GetTerminalOutcome(context.Context, string) (eer.TerminalOutcome, error)
}

type pe01ExternalEffectRuntime struct {
	runtime  *eer.Service
	terminal pe01TerminalReader
}

var _ orderport.ExternalEffectRuntime = (*pe01ExternalEffectRuntime)(nil)

func newPE01ExternalEffectRuntime(store eer.Store, terminal pe01TerminalReader) (*pe01ExternalEffectRuntime, error) {
	runtime, err := eer.NewService(store)
	if err != nil || terminal == nil {
		return nil, eer.ErrUnavailable
	}
	return &pe01ExternalEffectRuntime{runtime: runtime, terminal: terminal}, nil
}

func (runtime *pe01ExternalEffectRuntime) Execute(ctx context.Context, command orderport.ExternalEffectCommand, execute orderport.ProviderExecution) (orderport.ExternalEffectResult, error) {
	if runtime == nil || runtime.runtime == nil || runtime.terminal == nil || execute == nil {
		return orderport.ExternalEffectResult{}, eer.ErrUnavailable
	}
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOrder, Kind: eer.Kind(command.Kind), SourceRefDigest: eerDigest(command.SourceRefDigest), TargetRefDigest: eerDigest(command.TargetRefDigest), PayloadDigest: eerDigest(command.PayloadDigest), PolicyVersionHash: eerDigest(command.PolicyVersionDigest)})
	if err != nil {
		return orderport.ExternalEffectResult{}, err
	}
	acceptKey := digestText("pe01/eer/accept/v1", string(command.Kind), string(envelope.Fingerprint()))
	projection, _, err := runtime.runtime.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: acceptKey, Envelope: envelope})
	if err != nil {
		return orderport.ExternalEffectResult{}, err
	}
	job := eer.RiverJobLink{JobID: command.RiverJobID, Generation: command.RiverGeneration, Queue: command.RiverQueue, ArgsDigest: eerDigest(command.RiverArgsDigest), ScheduledAt: command.RiverScheduledAt}
	switch projection.State {
	case eer.StateAccepted:
		projection, _, err = runtime.runtime.Queue(ctx, eer.QueueCommand{EffectID: projection.ID, Job: job, ReceiptKeyDigest: digestText("pe01/eer/queue/v1", projection.ID, strconv.FormatInt(command.RiverGeneration, 10))})
	case eer.StateRetryableFailed:
		projection, _, err = runtime.runtime.Retry(ctx, eer.RetryCommand{EffectID: projection.ID, Job: job, ReceiptKeyDigest: digestText("pe01/eer/retry/v1", projection.ID, strconv.FormatInt(command.RiverGeneration, 10))})
	}
	if err != nil {
		return orderport.ExternalEffectResult{}, err
	}
	if projection.State != eer.StateQueued {
		return runtime.terminalResult(ctx, projection, nil)
	}
	lease, _, err := runtime.runtime.Claim(ctx, eer.ClaimCommand{EffectID: projection.ID, WorkerDigest: digestText("pe01/eer/worker/v1", strconv.FormatInt(command.RiverJobID, 10), strconv.FormatInt(command.RiverGeneration, 10))})
	if err != nil {
		return orderport.ExternalEffectResult{}, err
	}
	completed, receipt, runErr := runtime.runtime.RunAttempt(ctx, lease, pe01ProviderAdapter{execute: execute})
	return runtime.terminalResult(ctx, completed, errors.Join(runErr, digestError(receipt.CommandDigest)))
}

func (runtime *pe01ExternalEffectRuntime) Reconcile(ctx context.Context, effectID string, evidence [32]byte) (orderport.ExternalEffectResult, error) {
	if runtime == nil || runtime.runtime == nil || runtime.terminal == nil || effectID == "" {
		return orderport.ExternalEffectResult{}, eer.ErrUnavailable
	}
	outcome, err := runtime.terminal.GetTerminalOutcome(ctx, effectID)
	if err != nil || (outcome.State != eer.StateOutcomeUnknown && outcome.State != eer.StateReconciled) || outcome.Generation < 1 || outcome.Fence < 1 || outcome.LeaseExpiresAt.IsZero() {
		return orderport.ExternalEffectResult{}, errors.Join(eer.ErrReconcileRequired, err)
	}
	projection, _, err := runtime.runtime.Reconcile(ctx, eer.ReconcileCommand{Lease: eer.Lease{EffectID: effectID, Generation: outcome.Generation, Fence: outcome.Fence, ExpiresAt: outcome.LeaseExpiresAt}, ReceiptKeyDigest: digestText("pe01/eer/reconcile/v1", effectID), EvidenceDigest: eerDigest(evidence)})
	if err != nil {
		return orderport.ExternalEffectResult{}, err
	}
	return orderport.ExternalEffectResult{EffectID: projection.ID, State: orderport.EffectReconciled, ReceiptDigest: evidence}, nil
}

func (runtime *pe01ExternalEffectRuntime) terminalResult(ctx context.Context, projection eer.Projection, prior error) (orderport.ExternalEffectResult, error) {
	if projection.State == eer.StateAccepted || projection.State == eer.StateQueued || projection.State == eer.StateAttempted || projection.State == eer.StateRetryableFailed {
		return orderport.ExternalEffectResult{}, errors.Join(eer.ErrUnavailable, prior)
	}
	outcome, err := runtime.terminal.GetTerminalOutcome(ctx, projection.ID)
	if err != nil {
		return orderport.ExternalEffectResult{}, errors.Join(err, prior)
	}
	digest, err := parseEERDigest(outcome.ReceiptDigest)
	if err != nil {
		return orderport.ExternalEffectResult{}, errors.Join(err, prior)
	}
	state, ok := orderEffectState(outcome.State)
	if !ok {
		return orderport.ExternalEffectResult{}, errors.Join(eer.ErrInvalidTransition, prior)
	}
	return orderport.ExternalEffectResult{EffectID: outcome.EffectID, State: state, ReceiptDigest: digest}, prior
}

type pe01ProviderAdapter struct{ execute orderport.ProviderExecution }

func (adapter pe01ProviderAdapter) Execute(ctx context.Context, _ eer.EffectEnvelope, _ eer.Attempt) (eer.AdapterResult, error) {
	result, err := adapter.execute(ctx)
	completion := eer.Completion("")
	switch result.Completion {
	case orderport.ProviderExecuted:
		completion = eer.CompletionExecuted
	case orderport.ProviderOutcomeUnknown:
		completion = eer.CompletionOutcomeUnknown
	case orderport.ProviderFinalFailed:
		completion = eer.CompletionFinalFailed
	}
	return eer.AdapterResult{Completion: completion, ReceiptDigest: eerDigest(result.ReceiptDigest)}, err
}

func orderEffectState(state eer.State) (orderport.EffectState, bool) {
	switch state {
	case eer.StateExecuted:
		return orderport.EffectExecuted, true
	case eer.StateOutcomeUnknown:
		return orderport.EffectOutcomeUnknown, true
	case eer.StateReconciled:
		return orderport.EffectReconciled, true
	case eer.StateFinalFailed:
		return orderport.EffectFinalFailed, true
	default:
		return "", false
	}
}

func eerDigest(value [32]byte) eer.Digest {
	return eer.Digest("sha256:" + hex.EncodeToString(value[:]))
}
func digestText(domain string, values ...string) eer.Digest {
	sum := sha256.Sum256([]byte(domain + "\x00" + strings.Join(values, "\x00")))
	return eerDigest(sum)
}
func parseEERDigest(value eer.Digest) ([32]byte, error) {
	var result [32]byte
	text := string(value)
	if !strings.HasPrefix(text, "sha256:") {
		return result, eer.ErrUnavailable
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(text, "sha256:"))
	if err != nil || len(decoded) != len(result) {
		return result, eer.ErrUnavailable
	}
	copy(result[:], decoded)
	return result, nil
}
func digestError(value eer.Digest) error {
	_, err := parseEERDigest(value)
	return err
}
