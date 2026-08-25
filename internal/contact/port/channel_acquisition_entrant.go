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

// ChannelAcquisitionEntrantCommand crosses into Contact without an external
// customer identifier. Contact re-locks Match and validates WeComUserID
// against the immutable binding snapshot before it writes a customer event.
type ChannelAcquisitionEntrantCommand struct {
	InboxID       int64
	SourceKey     string
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
// It reserves by InboxID, revalidates the exact historical asset/version, and
// writes the customer event plus receipt atomically for attributed callbacks.
type ChannelAcquisitionEntrantRecorder interface {
	RecordChannelAcquisitionEntrant(context.Context, ChannelAcquisitionEntrantCommand) (ChannelAcquisitionEntrantReceipt, error)
}
