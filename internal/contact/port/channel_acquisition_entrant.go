package port

import (
	"context"
	"time"
)

// ChannelAcquisitionEntrantStatus is the durable disposition for one inbound
// WeCom acquisition callback. These facts are receipts, never a claim that a
// Provider call or a new Customer was created.
type ChannelAcquisitionEntrantStatus string

const (
	ChannelAcquisitionEntrantCorrelated      ChannelAcquisitionEntrantStatus = "correlated"
	ChannelAcquisitionEntrantAttributed      ChannelAcquisitionEntrantStatus = "attributed"
	ChannelAcquisitionEntrantPendingIdentity ChannelAcquisitionEntrantStatus = "pending_identity"
	ChannelAcquisitionEntrantUnmatchedAsset  ChannelAcquisitionEntrantStatus = "unmatched_asset"
	ChannelAcquisitionEntrantAmbiguousAsset  ChannelAcquisitionEntrantStatus = "ambiguous_asset"
	ChannelAcquisitionEntrantConflict        ChannelAcquisitionEntrantStatus = "conflict"
	ChannelAcquisitionEntrantIgnored         ChannelAcquisitionEntrantStatus = "ignored"
	ChannelAcquisitionEntrantReconciled      ChannelAcquisitionEntrantStatus = "reconciled"
)

func (status ChannelAcquisitionEntrantStatus) Valid() bool {
	switch status {
	case ChannelAcquisitionEntrantCorrelated,
		ChannelAcquisitionEntrantAttributed,
		ChannelAcquisitionEntrantPendingIdentity,
		ChannelAcquisitionEntrantUnmatchedAsset,
		ChannelAcquisitionEntrantAmbiguousAsset,
		ChannelAcquisitionEntrantConflict,
		ChannelAcquisitionEntrantIgnored,
		ChannelAcquisitionEntrantReconciled:
		return true
	default:
		return false
	}
}

// CanTransitionTo is the receipt state machine. In particular, a pending
// identity may be reconciled to an attributed Contact entrant exactly once;
// attributed and reconciled receipts never create another customer event.
func (status ChannelAcquisitionEntrantStatus) CanTransitionTo(next ChannelAcquisitionEntrantStatus) bool {
	if !status.Valid() || !next.Valid() || status == next {
		return status == next && status.Valid()
	}
	switch status {
	case ChannelAcquisitionEntrantCorrelated:
		return next == ChannelAcquisitionEntrantAttributed || next == ChannelAcquisitionEntrantPendingIdentity || next == ChannelAcquisitionEntrantConflict || next == ChannelAcquisitionEntrantReconciled
	case ChannelAcquisitionEntrantPendingIdentity:
		return next == ChannelAcquisitionEntrantAttributed || next == ChannelAcquisitionEntrantConflict || next == ChannelAcquisitionEntrantReconciled
	case ChannelAcquisitionEntrantUnmatchedAsset, ChannelAcquisitionEntrantAmbiguousAsset, ChannelAcquisitionEntrantConflict:
		return next == ChannelAcquisitionEntrantReconciled
	default:
		return false
	}
}

// ChannelAcquisitionEntrantCommand crosses into Contact without an external
// customer identifier. Contact re-locks Match and validates WeComUserID
// against the immutable binding snapshot before it writes a customer event.
type ChannelAcquisitionEntrantCommand struct {
	InboxID   int64
	SourceKey string
	// InputDigest is a domain-separated digest of the complete callback
	// identity: source key, corp, change type, state, callback user, external
	// user digest, and occurrence time. Reusing an InboxID with a different
	// digest is a conflict, never a replay.
	InputDigest   string
	CorpID        string
	CallbackState string
	WeComUserID   string
	OccurredAt    time.Time
	Status        ChannelAcquisitionEntrantStatus
	Match         AcquisitionAssetCorrelationMatch
	CustomerID    CustomerID
}

type ChannelAcquisitionEntrantReceipt struct {
	ID              int64
	InboxID         int64
	InputDigest     string
	Status          ChannelAcquisitionEntrantStatus
	EffectID        string
	ChannelID       int64
	Kind            AcquisitionAssetKind
	AssetVersion    int64
	CustomerID      CustomerID
	CustomerEventID EventID
	OccurredAt      time.Time
}

// ChannelAcquisitionEntrantRecorder is Contact-owned and transaction-bound.
// It reserves by InboxID+InputDigest, re-locks the exact historical binding,
// validates its frozen assignee list, and writes the receipt/customer event in
// one transaction. A pending_identity -> attributed CAS creates exactly one
// event; attributed/reconciled replays return the existing receipt/event.
type ChannelAcquisitionEntrantRecorder interface {
	RecordChannelAcquisitionEntrant(context.Context, ChannelAcquisitionEntrantCommand) (ChannelAcquisitionEntrantReceipt, error)
}
