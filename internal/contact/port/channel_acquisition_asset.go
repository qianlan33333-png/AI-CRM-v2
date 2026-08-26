package port

import (
	"context"
	"errors"
	"time"
)

var ErrAcquisitionAssetCorrelationUnavailable = errors.New("acquisition asset correlation unavailable")

// AcquisitionAssetKind is the Contact-owned carrier being published. Provider
// adapters translate these domain facts to their own protocol; raw provider
// payloads do not cross this boundary.
type AcquisitionAssetKind string

const (
	AcquisitionAssetQRCode AcquisitionAssetKind = "contact_way_qrcode"
	AcquisitionAssetLink   AcquisitionAssetKind = "customer_acquisition_link"
)

type AcquisitionAssetSnapshot struct {
	ChannelID            int64
	ChannelRevision      int64
	ChannelCode          string
	ChannelName          string
	ChannelStatus        string
	SceneValue           string
	AssigneeWeComUserIDs []string
}

type AcquisitionAssetPublishRequest struct {
	EffectID string
	CorpID   string
	// CorrelationKey is an opaque public callback handle, not a credential.
	CorrelationKey string
	AssetVersion   int64
	Supersedes     int64
	Kind           AcquisitionAssetKind
	Snapshot       AcquisitionAssetSnapshot
	SnapshotDigest [32]byte
}

type AcquisitionAssetProviderOutcome string

const (
	AcquisitionAssetProviderExecuted       AcquisitionAssetProviderOutcome = "executed"
	AcquisitionAssetProviderFinalFailed    AcquisitionAssetProviderOutcome = "final_failed"
	AcquisitionAssetProviderOutcomeUnknown AcquisitionAssetProviderOutcome = "outcome_unknown"
)

// AcquisitionAssetProviderResult contains the bounded public result required
// for the administrator's QR download/link-copy flow. Raw payloads, tokens and
// credentials remain inside the adapter.
type AcquisitionAssetProviderResult struct {
	Outcome                    AcquisitionAssetProviderOutcome
	ReceiptDigest              [32]byte
	AssetReferenceDigest       [32]byte
	ProviderAssetID            string
	AssetURL                   string
	BusinessEndpointDispatched bool
	RealExternalCallExecuted   bool
}

type AcquisitionAssetCorrelationCardinality string

const (
	AcquisitionAssetCorrelationZero     AcquisitionAssetCorrelationCardinality = "zero"
	AcquisitionAssetCorrelationOne      AcquisitionAssetCorrelationCardinality = "one"
	AcquisitionAssetCorrelationMultiple AcquisitionAssetCorrelationCardinality = "multiple"
)

type AcquisitionAssetCorrelationMatch struct {
	EffectID     string
	ChannelID    int64
	Kind         AcquisitionAssetKind
	AssetVersion int64
}

type AcquisitionAssetCorrelationResolution struct {
	Cardinality AcquisitionAssetCorrelationCardinality
	Match       AcquisitionAssetCorrelationMatch
}

type AcquisitionAssetCorrelationResolver interface {
	ResolveAcquisitionAssetCorrelation(ctx context.Context, corpID, callbackState string, occurredAt time.Time) (AcquisitionAssetCorrelationResolution, error)
}

type AcquisitionAssetProvider interface {
	PublishAcquisitionAsset(context.Context, AcquisitionAssetPublishRequest) (AcquisitionAssetProviderResult, error)
}
