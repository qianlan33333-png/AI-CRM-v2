package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrOutboundMediaReconcileInvalid     = errors.New("invalid outbound media reconciliation")
	ErrOutboundMediaReconcileConflict    = errors.New("outbound media reconciliation conflict")
	ErrOutboundMediaReconcileUnavailable = errors.New("outbound media reconciliation unavailable")
)

type OutboundMediaReconcileCommand struct {
	ContentPackageID int64
	TargetRef        string
	Generation       int64
	Fence            int64
	LeaseExpiresAt   time.Time
	EvidenceDigest   string
	IdempotencyKey   string
	ProviderAccepted bool
	DeliveryProven   bool
}

type OutboundMediaReconcileControl struct {
	EffectID       string
	State          string
	Generation     int64
	Fence          int64
	LeaseExpiresAt time.Time
}

type OutboundMediaReconciliationReceipt struct {
	EffectID             string
	Generation           int64
	Fence                int64
	LeaseExpiresAt       time.Time
	EvidenceDigest       string
	IdempotencyKeyDigest string
	ProviderAccepted     bool
	DeliveryProven       bool
	EERReceiptDigest     string
}

type OutboundMediaReconcileResult struct {
	EffectID         string
	State            string
	ProviderAccepted bool
	DeliveryProven   bool
	Replay           bool
}

type OutboundMediaReconcileStore interface {
	LockOutboundMediaEffectForReconcile(context.Context, int64, string) (OutboundMediaReconcileControl, error)
	ReadOutboundMediaReconciliationReceipt(context.Context, string) (OutboundMediaReconciliationReceipt, bool, error)
	RecordOutboundMediaReconciliationReceipt(context.Context, OutboundMediaReconciliationReceipt) error
}

type OutboundMediaReconcileRuntime interface {
	Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error)
}

type OutboundMediaReconcileService struct {
	uow     platformport.UnitOfWork
	store   OutboundMediaReconcileStore
	runtime OutboundMediaReconcileRuntime
}

func NewOutboundMediaReconcileService(uow platformport.UnitOfWork, store OutboundMediaReconcileStore, runtime OutboundMediaReconcileRuntime) *OutboundMediaReconcileService {
	return &OutboundMediaReconcileService{uow: uow, store: store, runtime: runtime}
}

func (s *OutboundMediaReconcileService) Reconcile(ctx context.Context, command OutboundMediaReconcileCommand) (out OutboundMediaReconcileResult, err error) {
	if ctx == nil || s == nil || s.uow == nil || s.store == nil || s.runtime == nil || !validOutboundMediaReconcileCommand(command) {
		return out, ErrOutboundMediaReconcileInvalid
	}
	targetDigest := mediaEERDigest("outbound-media-target", command.TargetRef)
	err = s.uow.Within(ctx, func(tx context.Context) error {
		control, err := s.store.LockOutboundMediaEffectForReconcile(tx, command.ContentPackageID, targetDigest)
		if err != nil {
			return reconcileStoreError(err)
		}
		receipt, found, err := s.store.ReadOutboundMediaReconciliationReceipt(tx, control.EffectID)
		if err != nil {
			return reconcileStoreError(err)
		}
		if found {
			if !sameOutboundMediaReconciliation(command, control, receipt) || control.State != string(eer.StateReconciled) {
				return ErrOutboundMediaReconcileConflict
			}
			out = OutboundMediaReconcileResult{EffectID: control.EffectID, State: control.State, ProviderAccepted: receipt.ProviderAccepted, DeliveryProven: receipt.DeliveryProven, Replay: true}
			return nil
		}
		if control.State != string(eer.StateOutcomeUnknown) || !sameOutboundMediaLease(command, control) {
			return ErrOutboundMediaReconcileConflict
		}
		keyDigest := mediaEERDigest("outbound-media-manual-reconcile-key", command.IdempotencyKey)
		projection, eerReceipt, err := s.runtime.Reconcile(tx, eer.ReconcileCommand{
			Lease:            eer.Lease{EffectID: control.EffectID, Generation: command.Generation, Fence: command.Fence, ExpiresAt: command.LeaseExpiresAt},
			ReceiptKeyDigest: eer.Digest(keyDigest),
			EvidenceDigest:   eer.Digest(command.EvidenceDigest),
		})
		if err != nil || projection.ID != control.EffectID || projection.Owner != eer.OwnerOutbound || projection.Kind != eer.KindOutboundMedia || projection.State != eer.StateReconciled {
			return errors.Join(ErrOutboundMediaReconcileUnavailable, err)
		}
		stored := OutboundMediaReconciliationReceipt{EffectID: control.EffectID, Generation: command.Generation, Fence: command.Fence, LeaseExpiresAt: command.LeaseExpiresAt, EvidenceDigest: command.EvidenceDigest, IdempotencyKeyDigest: keyDigest, ProviderAccepted: command.ProviderAccepted, DeliveryProven: command.DeliveryProven, EERReceiptDigest: string(eerReceipt.CommandDigest)}
		if err = s.store.RecordOutboundMediaReconciliationReceipt(tx, stored); err != nil {
			return reconcileStoreError(err)
		}
		out = OutboundMediaReconcileResult{EffectID: projection.ID, State: string(projection.State), ProviderAccepted: command.ProviderAccepted, DeliveryProven: command.DeliveryProven}
		return nil
	})
	return out, err
}

func validOutboundMediaReconcileCommand(command OutboundMediaReconcileCommand) bool {
	return command.ContentPackageID > 0 && strings.TrimSpace(command.TargetRef) != "" && command.Generation > 0 && command.Fence > 0 && !command.LeaseExpiresAt.IsZero() && validMediaDigest(command.EvidenceDigest) && strings.ToLower(command.EvidenceDigest) == command.EvidenceDigest && validContentDeliveryIdempotencyKey(command.IdempotencyKey) && (!command.DeliveryProven || command.ProviderAccepted)
}

func sameOutboundMediaLease(command OutboundMediaReconcileCommand, control OutboundMediaReconcileControl) bool {
	return control.EffectID != "" && control.Generation == command.Generation && control.Fence == command.Fence
}

func sameOutboundMediaReconciliation(command OutboundMediaReconcileCommand, control OutboundMediaReconcileControl, receipt OutboundMediaReconciliationReceipt) bool {
	return sameOutboundMediaLease(command, control) && receipt.EffectID == control.EffectID && receipt.Generation == command.Generation && receipt.Fence == command.Fence && receipt.LeaseExpiresAt.Equal(command.LeaseExpiresAt) && receipt.EvidenceDigest == command.EvidenceDigest && receipt.IdempotencyKeyDigest == mediaEERDigest("outbound-media-manual-reconcile-key", command.IdempotencyKey) && receipt.ProviderAccepted == command.ProviderAccepted && receipt.DeliveryProven == command.DeliveryProven
}

func boolString(value bool) string { return strconv.FormatBool(value) }

func reconcileStoreError(err error) error {
	if errors.Is(err, ErrOutboundMediaReconcileConflict) {
		return ErrOutboundMediaReconcileConflict
	}
	return errors.Join(ErrOutboundMediaReconcileUnavailable, err)
}
