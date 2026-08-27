package tag

import (
	"context"
	"errors"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

type catalogReaderStub struct {
	calls   int
	catalog wecomclient.CorpTagCatalog
	err     error
}

func (reader *catalogReaderStub) ListCorpTags(context.Context) (wecomclient.CorpTagCatalog, error) {
	reader.calls++
	return reader.catalog, reader.err
}

func TestCatalogProviderProducesObservedSnapshotOnlyAfterRealDirectoryRead(t *testing.T) {
	reader := &catalogReaderStub{catalog: wecomclient.CorpTagCatalog{Groups: []wecomclient.CorpTagGroup{{
		ProviderGroupID: "group-1", Name: "Lifecycle", Order: 2,
		Tags: []wecomclient.CorpTag{{ProviderTagID: "tag-1", Name: "Warm", Order: 3}},
	}}}}
	provider, err := NewCatalogProvider(reader)
	if err != nil {
		t.Fatal(err)
	}
	got, err := provider.Execute(context.Background(), catalogProviderCommand(), eer.Attempt{Number: 1})
	if err != nil || got.Completion != eer.CompletionExecuted || !got.Catalog.Observed || len(got.Catalog.Groups) != 1 || len(got.Catalog.Tags) != 1 || reader.calls != 1 {
		t.Fatalf("Execute() = %#v, %v; reader calls=%d", got, err, reader.calls)
	}
	if got.ReceiptDigest == "" || got.Catalog.Tags[0].ProviderGroupID != "group-1" {
		t.Fatalf("snapshot result=%#v", got)
	}
}

func TestCatalogProviderReceiptBindsObservedDirectoryRatherThanReadOrder(t *testing.T) {
	first := wecomclient.CorpTagCatalog{Groups: []wecomclient.CorpTagGroup{
		{ProviderGroupID: "group-2", Name: "Dormant", Order: 2, Tags: []wecomclient.CorpTag{{ProviderTagID: "tag-2", Name: "Cold", Order: 2}, {ProviderTagID: "tag-1", Name: "Warm", Order: 1}}},
		{ProviderGroupID: "group-1", Name: "Lifecycle", Order: 1},
	}}
	second := wecomclient.CorpTagCatalog{Groups: []wecomclient.CorpTagGroup{
		{ProviderGroupID: "group-1", Name: "Lifecycle", Order: 1},
		{ProviderGroupID: "group-2", Name: "Dormant", Order: 2, Tags: []wecomclient.CorpTag{{ProviderTagID: "tag-1", Name: "Warm", Order: 1}, {ProviderTagID: "tag-2", Name: "Cold", Order: 2}}},
	}}
	provider, err := NewCatalogProvider(&catalogReaderStub{catalog: first})
	if err != nil {
		t.Fatal(err)
	}
	firstResult, err := provider.Execute(context.Background(), catalogProviderCommand(), eer.Attempt{Number: 1})
	if err != nil || !firstResult.Catalog.Observed {
		t.Fatalf("first Execute() = %#v, %v", firstResult, err)
	}
	provider, err = NewCatalogProvider(&catalogReaderStub{catalog: second})
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := provider.Execute(context.Background(), catalogProviderCommand(), eer.Attempt{Number: 1})
	if err != nil || firstResult.ReceiptDigest != secondResult.ReceiptDigest {
		t.Fatalf("unordered directory receipt = %q/%q, %v", firstResult.ReceiptDigest, secondResult.ReceiptDigest, err)
	}
	second.Groups[1].Tags[0].Name = "Reclassified"
	provider, err = NewCatalogProvider(&catalogReaderStub{catalog: second})
	if err != nil {
		t.Fatal(err)
	}
	changedResult, err := provider.Execute(context.Background(), catalogProviderCommand(), eer.Attempt{Number: 1})
	if err != nil || changedResult.ReceiptDigest == firstResult.ReceiptDigest {
		t.Fatalf("changed directory receipt = %q/%q, %v", firstResult.ReceiptDigest, changedResult.ReceiptDigest, err)
	}
}

func TestCatalogProviderFailsClosedWithoutRetryableProviderResult(t *testing.T) {
	for _, test := range []struct {
		name       string
		command    ProviderCommand
		err        error
		completion eer.Completion
		calls      int
	}{
		{name: "provider rejected", err: wecomclient.ErrUpstream, completion: eer.CompletionFinalFailed, calls: 1},
		{name: "transport unobserved", err: wecomclient.ErrTransport, completion: eer.CompletionOutcomeUnknown, calls: 1},
		{name: "malformed unobserved", err: wecomclient.ErrUnexpectedResponse, completion: eer.CompletionOutcomeUnknown, calls: 1},
		{name: "other tag operation", command: ProviderCommand{CorpID: "corp-1", Operation: OperationMark, LegacyReceiptID: 38}, completion: eer.CompletionFinalFailed, calls: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &catalogReaderStub{err: test.err}
			provider, err := NewCatalogProvider(reader)
			if err != nil {
				t.Fatal(err)
			}
			command := test.command
			if command.CorpID == "" {
				command = catalogProviderCommand()
			}
			got, err := provider.Execute(context.Background(), command, eer.Attempt{Number: 1})
			if err != nil || got.Completion != test.completion || got.Catalog.Observed || got.ReceiptDigest == "" || reader.calls != test.calls {
				t.Fatalf("Execute() = %#v, %v; reader calls=%d", got, err, reader.calls)
			}
		})
	}
	if _, err := NewCatalogProvider(nil); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewCatalogProvider(nil) error = %v", err)
	}
}

func TestCatalogProviderRejectsInvalidObservedDirectoryAsUnknown(t *testing.T) {
	reader := &catalogReaderStub{catalog: wecomclient.CorpTagCatalog{Groups: []wecomclient.CorpTagGroup{{ProviderGroupID: "group-1", Name: "Lifecycle"}, {ProviderGroupID: "group-1", Name: "Duplicate"}}}}
	provider, err := NewCatalogProvider(reader)
	if err != nil {
		t.Fatal(err)
	}
	got, err := provider.Execute(context.Background(), catalogProviderCommand(), eer.Attempt{Number: 1})
	if err != nil || got.Completion != eer.CompletionOutcomeUnknown || got.Catalog.Observed || reader.calls != 1 {
		t.Fatalf("Execute() = %#v, %v; reader calls=%d", got, err, reader.calls)
	}
}

func TestCatalogProviderUnknownDirectoryIsNotRetriedByEffectRuntime(t *testing.T) {
	service, _, _ := queuedTestService(t, OperationCatalogSync)
	reader := &catalogReaderStub{err: wecomclient.ErrTransport}
	provider, err := NewCatalogProvider(reader)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Execute(context.Background(), "eer_41", digest("catalog-provider-worker", "first"), provider)
	if err != nil || first.State != eer.StateOutcomeUnknown || !first.ManualReconcileRequired || reader.calls != 1 {
		t.Fatalf("first Execute() = %#v, %v; reader calls=%d", first, err, reader.calls)
	}
	second, err := service.Execute(context.Background(), "eer_41", digest("catalog-provider-worker", "replay"), provider)
	if err != nil || second.State != eer.StateOutcomeUnknown || second.ProviderCallAttempted || reader.calls != 1 {
		t.Fatalf("replay Execute() = %#v, %v; reader calls=%d", second, err, reader.calls)
	}
}

func catalogProviderCommand() ProviderCommand {
	return ProviderCommand{CorpID: "corp-1", Operation: OperationCatalogSync, SyncTrigger: SyncTriggerManual, LegacyReceiptID: 38}
}
