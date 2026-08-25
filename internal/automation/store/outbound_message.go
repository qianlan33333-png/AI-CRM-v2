package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store/generated"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// OutboundMessageArgs is the only Automation-owned River payload. The EER
// envelope is persisted separately as digests, so this job contains no
// recipient, message body, credential, or Provider routing information.
type OutboundMessageArgs struct {
	EffectID string `json:"effect_id"`
}

func (OutboundMessageArgs) Kind() string { return "automation_outbound_message" }

type OutboundMessageHandoff struct {
	uow     platformport.UnitOfWork
	runtime eer.Runtime
	client  *platformjobqueue.InsertOnlyClient
}

func NewOutboundMessageHandoff(pool *pgxpool.Pool, uow platformport.UnitOfWork, runtime eer.Runtime) (*OutboundMessageHandoff, error) {
	if pool == nil || uow == nil || runtime == nil {
		return nil, ErrInvalidTagTrigger
	}
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(ErrInvalidTagTrigger, err)
	}
	return &OutboundMessageHandoff{uow: uow, runtime: runtime, client: client}, nil
}

type outboundMessageInput struct {
	ActionID      int64
	SourceEventID int64
	RuleID        int64
	RuleVersion   int64
	CustomerID    int64
	TemplateKey   string
	Now           time.Time
}

func (handoff *OutboundMessageHandoff) Queue(ctx context.Context, input outboundMessageInput) (string, error) {
	if handoff == nil || handoff.runtime == nil || handoff.client == nil || !validOutboundMessageInput(input) {
		return "", ErrInvalidTagTrigger
	}
	envelope, err := outboundMessageEnvelope(input)
	if err != nil {
		return "", err
	}
	accepted, _, err := handoff.runtime.Accept(ctx, eer.AcceptCommand{
		ReceiptKeyDigest: automationDigest("accept", strconv.FormatInt(input.ActionID, 10)),
		Envelope:         envelope,
	})
	if err != nil || accepted.ID == "" || accepted.State != eer.StateAccepted {
		return "", errors.Join(ErrInvalidTagTrigger, err)
	}
	queries, err := automationQueries(ctx)
	if err != nil {
		return "", err
	}
	linked, err := queries.AttachAutomationActionExternalEffect(ctx, automationdb.AttachAutomationActionExternalEffectParams{
		ActionID: input.ActionID, ExternalEffectID: pgtype.Text{String: accepted.ID, Valid: true},
	})
	if err != nil || linked.ID != input.ActionID || !linked.ExternalEffectID.Valid || linked.ExternalEffectID.String != accepted.ID {
		return "", errors.Join(ErrInvalidTagTrigger, err)
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return "", err
	}
	jobID, err := handoff.client.InsertTx(ctx, tx, OutboundMessageArgs{EffectID: accepted.ID}, string(platformjobqueue.QueueOutbound))
	if err != nil || jobID < 1 {
		return "", errors.Join(ErrInvalidTagTrigger, err)
	}
	queued, _, err := handoff.runtime.Queue(ctx, eer.QueueCommand{
		EffectID:         accepted.ID,
		Job:              eer.RiverJobLink{JobID: jobID, Generation: 1, Queue: string(platformjobqueue.QueueOutbound), ArgsDigest: automationDigest("river", accepted.ID), ScheduledAt: input.Now.UTC()},
		ReceiptKeyDigest: automationDigest("queue", strconv.FormatInt(input.ActionID, 10)),
	})
	if err != nil || queued.ID != accepted.ID || queued.State != eer.StateQueued {
		return "", errors.Join(ErrInvalidTagTrigger, err)
	}
	return accepted.ID, nil
}

// RunEffect contains no Provider call itself. The caller supplies the adapter;
// production composition deliberately uses the disabled adapter below.
func (handoff *OutboundMessageHandoff) RunEffect(ctx context.Context, effectID string, workerDigest eer.Digest, adapter eer.Adapter) error {
	if handoff == nil || handoff.uow == nil || handoff.runtime == nil || effectID == "" || adapter == nil {
		return ErrInvalidTagTrigger
	}
	lease, _, err := handoff.runtime.Claim(ctx, eer.ClaimCommand{EffectID: effectID, WorkerDigest: workerDigest})
	if err != nil {
		return err
	}
	projection, receipt, runErr := handoff.runtime.RunAttempt(ctx, lease, adapter)
	state, valid := automationTerminalState(projection.State)
	if !valid {
		return errors.Join(ErrInvalidTagTrigger, runErr)
	}
	writeErr := handoff.uow.Within(ctx, func(txCtx context.Context) error {
		queries, queryErr := automationQueries(txCtx)
		if queryErr != nil {
			return queryErr
		}
		_, queryErr = queries.ProjectAutomationActionTerminalEffect(txCtx, automationdb.ProjectAutomationActionTerminalEffectParams{
			State: state, ReceiptDigest: pgtype.Text{String: string(receipt.CommandDigest), Valid: true},
			Now: pgtype.Timestamptz{Time: projection.UpdatedAt.UTC(), Valid: true}, ExternalEffectID: pgtype.Text{String: effectID, Valid: true},
		})
		return queryErr
	})
	if writeErr != nil {
		return writeErr
	}
	// Ambiguous attempts are terminal locally and must never make River retry;
	// only ReconcileOutboundMessage can advance them.
	if projection.State == eer.StateOutcomeUnknown && errors.Is(runErr, eer.ErrAdapterFailure) {
		return nil
	}
	return runErr
}

func (handoff *OutboundMessageHandoff) ReconcileOutboundMessage(ctx context.Context, command automationport.ReconcileOutboundMessageCommand) (automationport.RuntimeExecution, error) {
	if handoff == nil || handoff.uow == nil || handoff.runtime == nil || !validAutomationReconcile(command) {
		return automationport.RuntimeExecution{}, ErrInvalidTagTrigger
	}
	var result automationport.RuntimeExecution
	err := handoff.uow.Within(ctx, func(txCtx context.Context) error {
		queries, err := automationQueries(txCtx)
		if err != nil {
			return err
		}
		action, err := queries.GetAutomationActionForReconcile(txCtx, command.ActionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRuleNotFound
		}
		if err != nil || !action.ExternalEffectID.Valid || (action.State != "outcome_unknown" && action.State != "completed") {
			return errors.Join(ErrInvalidTagTrigger, err)
		}
		projection, receipt, err := handoff.runtime.Reconcile(txCtx, eer.ReconcileCommand{
			Lease:            eer.Lease{EffectID: action.ExternalEffectID.String, Generation: command.Generation, Fence: command.Fence, ExpiresAt: command.LeaseExpiresAt},
			ReceiptKeyDigest: automationDigest("reconcile", strconv.FormatInt(command.Actor, 10), command.IdempotencyKey), EvidenceDigest: command.EvidenceDigest,
		})
		if err != nil || projection.State != eer.StateReconciled {
			return errors.Join(ErrInvalidTagTrigger, err)
		}
		if action.State == "outcome_unknown" {
			_, err = queries.ProjectAutomationActionTerminalEffect(txCtx, automationdb.ProjectAutomationActionTerminalEffectParams{
				State: "completed", ReceiptDigest: pgtype.Text{String: string(receipt.CommandDigest), Valid: true},
				Now: pgtype.Timestamptz{Time: projection.UpdatedAt.UTC(), Valid: true}, ExternalEffectID: action.ExternalEffectID,
			})
			if err != nil {
				return err
			}
		}
		result = automationport.RuntimeExecution{ActionID: action.ID, EnrollmentID: action.EnrollmentID, ActionType: action.ActionType, State: "completed", ExternalEffectID: &action.ExternalEffectID.String, CreatedAt: action.CreatedAt.Time.UTC()}
		completed := projection.UpdatedAt.UTC()
		if action.CompletedAt.Valid {
			completed = action.CompletedAt.Time.UTC()
		}
		result.CompletedAt = &completed
		return nil
	})
	return result, err
}

func validOutboundMessageInput(input outboundMessageInput) bool {
	return input.ActionID > 0 && input.SourceEventID > 0 && input.RuleID > 0 && input.RuleVersion > 0 && input.CustomerID > 0 &&
		input.TemplateKey == "text.notice.v1" && !input.Now.IsZero()
}

func validAutomationReconcile(command automationport.ReconcileOutboundMessageCommand) bool {
	return command.ActionID > 0 && command.Actor > 0 && len(command.IdempotencyKey) >= 16 && len(command.IdempotencyKey) <= 128 &&
		strings.TrimSpace(command.IdempotencyKey) == command.IdempotencyKey && command.Generation > 0 && command.Fence > 0 &&
		!command.LeaseExpiresAt.IsZero() && validAutomationDigest(command.EvidenceDigest)
}

func outboundMessageEnvelope(input outboundMessageInput) (eer.EffectEnvelope, error) {
	return eer.NewEnvelope(eer.EnvelopeInput{
		Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage,
		SourceRefDigest:   automationDigest("source", strconv.FormatInt(input.SourceEventID, 10), strconv.FormatInt(input.RuleID, 10), strconv.FormatInt(input.RuleVersion, 10)),
		TargetRefDigest:   automationDigest("target", strconv.FormatInt(input.CustomerID, 10)),
		PayloadDigest:     automationDigest("template", input.TemplateKey),
		PolicyVersionHash: automationDigest("policy", "a01-outbound-message-v2"),
	})
}

func automationTerminalState(state eer.State) (string, bool) {
	switch state {
	case eer.StateExecuted, eer.StateReconciled:
		return "completed", true
	case eer.StateFinalFailed:
		return "final_failed", true
	case eer.StateOutcomeUnknown:
		return "outcome_unknown", true
	default:
		return "", false
	}
}

func automationDigest(label string, parts ...string) eer.Digest {
	sum := sha256.Sum256([]byte("automation.outbound_message.v2\x00" + label + "\x00" + strings.Join(parts, "\x00")))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func validAutomationDigest(value eer.Digest) bool {
	return len(value) == len("sha256:")+64 && strings.HasPrefix(string(value), "sha256:")
}

// DisabledOutboundMessageAdapter is the production default. It is an explicit
// local final-failure receipt, not a Provider receipt or delivery assertion.
type DisabledOutboundMessageAdapter struct{}

func (DisabledOutboundMessageAdapter) Execute(_ context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	return eer.AdapterResult{Completion: eer.CompletionFinalFailed, ReceiptDigest: automationDigest("disabled", string(envelope.Fingerprint()), fmt.Sprintf("%d", attempt.Number))}, nil
}
