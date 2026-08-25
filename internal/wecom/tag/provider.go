package tag

import (
	"context"
	"strconv"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

// CatalogSnapshot is present only after an observed catalog-sync response.
// An empty but observed snapshot is valid and distinct from no response.
type CatalogSnapshot struct {
	Observed bool
	Groups   []CatalogGroup
	Tags     []CatalogTag
}

type CatalogGroup struct {
	ProviderGroupID string
	Name            string
	Order           int32
}

type CatalogTag struct {
	ProviderTagID   string
	ProviderGroupID string
	Name            string
	Order           int32
}

type ProviderCommand struct {
	CorpID          string
	Operation       Operation
	SyncTrigger     SyncTrigger
	ExternalUserID  string
	ProviderTagIDs  []string
	LegacyReceiptID int64
}

type ProviderResult struct {
	Completion    eer.Completion
	ReceiptDigest eer.Digest
	Catalog       CatalogSnapshot
}

// Provider is the domain-private adapter boundary. Production composition in
// this package exposes only DisabledProvider; no HTTP/token/network client is
// implemented here.
type Provider interface {
	Execute(context.Context, ProviderCommand, eer.Attempt) (ProviderResult, error)
}

type effectAdapter struct {
	record   Effect
	provider Provider
	catalog  CatalogSnapshot
}

func (adapter *effectAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	if adapter == nil || nilDependency(adapter.provider) || envelope.Fingerprint() != adapter.record.EnvelopeFingerprint {
		return eer.AdapterResult{}, ErrEffectConflict
	}
	result, err := adapter.provider.Execute(ctx, ProviderCommand{
		CorpID: adapter.record.CorpID, Operation: adapter.record.Operation, SyncTrigger: adapter.record.SyncTrigger,
		ExternalUserID: adapter.record.ExternalUserID, ProviderTagIDs: append([]string(nil), adapter.record.ProviderTagIDs...),
		LegacyReceiptID: adapter.record.LegacyReceiptID,
	}, attempt)
	if err != nil {
		return eer.AdapterResult{}, err
	}
	if !validProviderResult(adapter.record.Operation, result) {
		return eer.AdapterResult{}, ErrEffectUnavailable
	}
	adapter.catalog = cloneCatalog(result.Catalog)
	return eer.AdapterResult{Completion: result.Completion, ReceiptDigest: result.ReceiptDigest}, nil
}

func validProviderResult(operation Operation, result ProviderResult) bool {
	if !validDigest(result.ReceiptDigest) {
		return false
	}
	switch result.Completion {
	case "executed":
		if operation == OperationCatalogSync {
			return validCatalog(result.Catalog)
		}
		return !result.Catalog.Observed && len(result.Catalog.Groups) == 0 && len(result.Catalog.Tags) == 0
	case "final_failed", "outcome_unknown":
		return !result.Catalog.Observed && len(result.Catalog.Groups) == 0 && len(result.Catalog.Tags) == 0
	default:
		// retryable_failed is deliberately excluded. The current public EER
		// port has no owner-safe retry command, and unknown is never retried.
		return false
	}
}

func validCatalog(snapshot CatalogSnapshot) bool {
	if !snapshot.Observed || len(snapshot.Groups) > 1000 || len(snapshot.Tags) > 10000 {
		return false
	}
	groups := make(map[string]struct{}, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		if !validText(group.ProviderGroupID, 1, maximumTagID) || !validText(group.Name, 1, 256) {
			return false
		}
		if _, duplicate := groups[group.ProviderGroupID]; duplicate {
			return false
		}
		groups[group.ProviderGroupID] = struct{}{}
	}
	tags := make(map[string]struct{}, len(snapshot.Tags))
	for _, tag := range snapshot.Tags {
		if !validText(tag.ProviderTagID, 1, maximumTagID) || !validText(tag.ProviderGroupID, 1, maximumTagID) || !validText(tag.Name, 1, 256) {
			return false
		}
		if _, exists := groups[tag.ProviderGroupID]; !exists {
			return false
		}
		if _, duplicate := tags[tag.ProviderTagID]; duplicate {
			return false
		}
		tags[tag.ProviderTagID] = struct{}{}
	}
	return true
}

func cloneCatalog(snapshot CatalogSnapshot) CatalogSnapshot {
	return CatalogSnapshot{Observed: snapshot.Observed, Groups: append([]CatalogGroup(nil), snapshot.Groups...), Tags: append([]CatalogTag(nil), snapshot.Tags...)}
}

// DisabledProvider is the only production-ready adapter in this package. It
// performs zero network I/O and returns a local typed final-failure receipt;
// the receipt is not a Provider response and cannot prove delivery.
type DisabledProvider struct{}

func (DisabledProvider) Execute(_ context.Context, command ProviderCommand, attempt eer.Attempt) (ProviderResult, error) {
	return ProviderResult{
		Completion:    "final_failed",
		ReceiptDigest: digest("provider-disabled", string(command.Operation), strconv.FormatInt(command.LegacyReceiptID, 10), strconv.Itoa(int(attempt.Number))),
	}, nil
}

var _ Provider = DisabledProvider{}
var _ eer.Adapter = (*effectAdapter)(nil)
