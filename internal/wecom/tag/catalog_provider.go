package tag

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

// CatalogReader is the read-only WeCom boundary required for a catalog-sync
// effect. It intentionally exposes no tag mutation method.
type CatalogReader interface {
	ListCorpTags(context.Context) (wecomclient.CorpTagCatalog, error)
}

// CatalogProvider turns one observed WeCom tag-directory response into the
// immutable snapshot that TagEffectRepository persists atomically with the
// EER attempt result. Construction is inert; central composition must still
// explicitly decide when a credential is authorized for this directory read.
type CatalogProvider struct {
	reader CatalogReader
}

func NewCatalogProvider(reader CatalogReader) (*CatalogProvider, error) {
	if nilDependency(reader) {
		return nil, ErrInvalidConfiguration
	}
	return &CatalogProvider{reader: reader}, nil
}

func (provider *CatalogProvider) Execute(ctx context.Context, command ProviderCommand, attempt eer.Attempt) (ProviderResult, error) {
	if provider == nil || nilDependency(provider.reader) || ctx == nil || !validCorpID(command.CorpID) {
		return catalogFailure(command, attempt, eer.CompletionFinalFailed, "invalid-configuration"), nil
	}
	if command.Operation != OperationCatalogSync {
		return catalogFailure(command, attempt, eer.CompletionFinalFailed, "catalog-sync-only"), nil
	}
	catalog, err := provider.reader.ListCorpTags(ctx)
	if err != nil {
		if errors.Is(err, wecomclient.ErrUpstream) || errors.Is(err, wecomclient.ErrInvalidConfig) {
			return catalogFailure(command, attempt, eer.CompletionFinalFailed, "provider-rejected"), nil
		}
		// A read that did not yield a valid complete directory cannot establish
		// an observed snapshot. Persist unknown and require reconciliation;
		// never make River retry it automatically.
		return catalogFailure(command, attempt, eer.CompletionOutcomeUnknown, "catalog-unobserved"), nil
	}
	snapshot := catalogSnapshot(catalog)
	if !validCatalog(snapshot) {
		return catalogFailure(command, attempt, eer.CompletionOutcomeUnknown, "catalog-invalid"), nil
	}
	return ProviderResult{
		Completion:    eer.CompletionExecuted,
		ReceiptDigest: catalogObservedDigest(command, attempt, snapshot),
		Catalog:       snapshot,
	}, nil
}

func catalogSnapshot(catalog wecomclient.CorpTagCatalog) CatalogSnapshot {
	snapshot := CatalogSnapshot{Observed: true, Groups: make([]CatalogGroup, 0, len(catalog.Groups))}
	for _, group := range catalog.Groups {
		snapshot.Groups = append(snapshot.Groups, CatalogGroup{ProviderGroupID: group.ProviderGroupID, Name: group.Name, Order: group.Order})
		for _, providerTag := range group.Tags {
			snapshot.Tags = append(snapshot.Tags, CatalogTag{
				ProviderTagID: providerTag.ProviderTagID, ProviderGroupID: group.ProviderGroupID, Name: providerTag.Name, Order: providerTag.Order,
			})
		}
	}
	return snapshot
}

func catalogFailure(command ProviderCommand, attempt eer.Attempt, completion eer.Completion, reason string) ProviderResult {
	return ProviderResult{Completion: completion, ReceiptDigest: digest(
		"catalog-provider", string(completion), reason, command.CorpID, strconv.FormatInt(command.LegacyReceiptID, 10), strconv.Itoa(int(attempt.Number)),
	)}
}

func catalogObservedDigest(command ProviderCommand, attempt eer.Attempt, snapshot CatalogSnapshot) eer.Digest {
	groups := make([]string, 0, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		groups = append(groups, strings.Join([]string{group.ProviderGroupID, group.Name, strconv.FormatInt(int64(group.Order), 10)}, "\x00"))
	}
	tags := make([]string, 0, len(snapshot.Tags))
	for _, providerTag := range snapshot.Tags {
		tags = append(tags, strings.Join([]string{providerTag.ProviderTagID, providerTag.ProviderGroupID, providerTag.Name, strconv.FormatInt(int64(providerTag.Order), 10)}, "\x00"))
	}
	sort.Strings(groups)
	sort.Strings(tags)
	return digest("catalog-observed", command.CorpID, strconv.FormatInt(command.LegacyReceiptID, 10), strconv.Itoa(int(attempt.Number)), strings.Join(groups, "\x01"), strings.Join(tags, "\x01"))
}

var _ Provider = (*CatalogProvider)(nil)
