package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

type customerAcquisitionLinkReceipts struct {
	receipt CustomerAcquisitionLinkReceipt
}

func (store *customerAcquisitionLinkReceipts) ReserveCustomerAcquisitionLink(_ context.Context, operation CustomerAcquisitionLinkOperation, command CustomerAcquisitionLinkCommand, digest [32]byte) (CustomerAcquisitionLinkReceipt, error) {
	if store.receipt.ID == 0 {
		store.receipt = CustomerAcquisitionLinkReceipt{ID: 1, Operation: operation, Command: command, RequestDigest: digest, State: CustomerAcquisitionLinkReserved}
	}
	return store.receipt, nil
}

func (store *customerAcquisitionLinkReceipts) MarkCustomerAcquisitionLinkAttempted(_ context.Context, id int64) (CustomerAcquisitionLinkReceipt, error) {
	if store.receipt.ID != id || store.receipt.State != CustomerAcquisitionLinkReserved {
		return CustomerAcquisitionLinkReceipt{}, errors.New("invalid attempt")
	}
	store.receipt.State = CustomerAcquisitionLinkAttempted
	return store.receipt, nil
}

func (store *customerAcquisitionLinkReceipts) CompleteCustomerAcquisitionLink(_ context.Context, completion CustomerAcquisitionLinkCompletion) (CustomerAcquisitionLinkReceipt, error) {
	if store.receipt.ID != completion.ReceiptID {
		return CustomerAcquisitionLinkReceipt{}, errors.New("receipt missing")
	}
	store.receipt.State, store.receipt.Link = completion.State, completion.Link
	store.receipt.OutcomeDigest = completion.OutcomeDigest
	store.receipt.BusinessEndpointDispatched = completion.BusinessEndpointDispatched
	store.receipt.RealExternalCallExecuted = completion.RealExternalCallExecuted
	store.receipt.ReconcileKeyDigest = completion.ReconcileKeyDigest
	store.receipt.EvidenceDigest = completion.EvidenceDigest
	store.receipt.Resolution = completion.Resolution
	return store.receipt, nil
}

func (store *customerAcquisitionLinkReceipts) GetCustomerAcquisitionLinkReceipt(_ context.Context, id int64) (CustomerAcquisitionLinkReceipt, error) {
	if store.receipt.ID != id {
		return CustomerAcquisitionLinkReceipt{}, errors.New("receipt missing")
	}
	return store.receipt, nil
}

type customerAcquisitionLinkProvider struct {
	links                                           map[string]wecomport.CustomerAcquisitionLink
	createCalls, updateCalls, deleteCalls, getCalls int
	write                                           wecomport.CustomerAcquisitionLinkWriteResult
	getErr                                          error
}

func (provider *customerAcquisitionLinkProvider) ListCustomerAcquisitionLinks(context.Context, string, int) (wecomport.CustomerAcquisitionLinkPage, error) {
	items := make([]wecomport.CustomerAcquisitionLink, 0, len(provider.links))
	for _, link := range provider.links {
		items = append(items, link)
	}
	return wecomport.CustomerAcquisitionLinkPage{Links: items}, nil
}

func (provider *customerAcquisitionLinkProvider) GetCustomerAcquisitionLink(_ context.Context, linkID string) (wecomport.CustomerAcquisitionLink, error) {
	provider.getCalls++
	if provider.getErr != nil {
		return wecomport.CustomerAcquisitionLink{}, provider.getErr
	}
	link, ok := provider.links[linkID]
	if !ok {
		return wecomport.CustomerAcquisitionLink{}, errors.New("not found")
	}
	return link, nil
}

func TestCustomerAcquisitionLinkDeleteReconcilesOnlyExplicitNotFound(t *testing.T) {
	command := CustomerAcquisitionLinkCommand{Actor: 7, IdempotencyKey: "customer-link-delete-unknown", LinkID: "link-1"}
	digest := customerAcquisitionLinkCommandDigest(CustomerAcquisitionLinkDelete, command)
	reconcile := ReconcileCustomerAcquisitionLinkCommand{ReceiptID: 2, Actor: 9, IdempotencyKey: "customer-link-delete-reconcile", LinkID: "link-1", Resolution: CustomerAcquisitionLinkProviderNotApplied, EvidenceDigest: [32]byte{9}}

	for _, testCase := range []struct {
		name   string
		err    error
		wantOK bool
	}{
		{name: "explicit provider not found", err: wecomport.ErrCustomerAcquisitionLinkNotFound, wantOK: true},
		{name: "transport remains unknown", err: errors.New("timeout")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			receipts := &customerAcquisitionLinkReceipts{receipt: CustomerAcquisitionLinkReceipt{ID: 2, Operation: CustomerAcquisitionLinkDelete, Command: command, RequestDigest: digest, State: CustomerAcquisitionLinkOutcomeUnknown}}
			service := NewCustomerAcquisitionLinkService(receipts, &customerAcquisitionLinkProvider{getErr: testCase.err})
			result, err := service.Reconcile(context.Background(), reconcile)
			if testCase.wantOK {
				if err != nil || result.State != CustomerAcquisitionLinkReconciled || result.Link != nil || !result.RealExternalCallExecuted {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				return
			}
			if !errors.Is(err, ErrCustomerAcquisitionLinkReconcile) || receipts.receipt.State != CustomerAcquisitionLinkOutcomeUnknown {
				t.Fatalf("result=%+v state=%s err=%v", result, receipts.receipt.State, err)
			}
		})
	}
}

func TestCustomerAcquisitionLinkReconcileBindsUpdateAndDeleteToOriginalLink(t *testing.T) {
	for _, operation := range []CustomerAcquisitionLinkOperation{CustomerAcquisitionLinkUpdate, CustomerAcquisitionLinkDelete} {
		t.Run(string(operation), func(t *testing.T) {
			command := CustomerAcquisitionLinkCommand{Actor: 7, IdempotencyKey: "customer-link-original-0001", LinkID: "link-original"}
			if operation == CustomerAcquisitionLinkUpdate {
				command.Input = customerAcquisitionLinkInputFixture()
			}
			receipts := &customerAcquisitionLinkReceipts{receipt: CustomerAcquisitionLinkReceipt{
				ID: 2, Operation: operation, Command: command,
				RequestDigest: customerAcquisitionLinkCommandDigest(operation, command), State: CustomerAcquisitionLinkOutcomeUnknown,
			}}
			provider := &customerAcquisitionLinkProvider{getErr: wecomport.ErrCustomerAcquisitionLinkNotFound}
			service := NewCustomerAcquisitionLinkService(receipts, provider)
			_, err := service.Reconcile(context.Background(), ReconcileCustomerAcquisitionLinkCommand{
				ReceiptID: 2, Actor: 9, IdempotencyKey: "customer-link-reconcile-0001", LinkID: "link-other",
				Resolution: CustomerAcquisitionLinkProviderNotApplied, EvidenceDigest: [32]byte{1},
			})
			if !errors.Is(err, ErrCustomerAcquisitionLinkReconcile) || provider.getCalls != 0 || receipts.receipt.State != CustomerAcquisitionLinkOutcomeUnknown {
				t.Fatalf("err=%v get_calls=%d receipt=%+v", err, provider.getCalls, receipts.receipt)
			}
		})
	}
}

func (provider *customerAcquisitionLinkProvider) CreateCustomerAcquisitionLink(_ context.Context, input wecomport.CustomerAcquisitionLinkInput) (wecomport.CustomerAcquisitionLinkWriteResult, error) {
	provider.createCalls++
	return provider.write, nil
}

func (provider *customerAcquisitionLinkProvider) UpdateCustomerAcquisitionLink(_ context.Context, _ string, _ wecomport.CustomerAcquisitionLinkInput) (wecomport.CustomerAcquisitionLinkWriteResult, error) {
	provider.updateCalls++
	return provider.write, nil
}

func (provider *customerAcquisitionLinkProvider) DeleteCustomerAcquisitionLink(context.Context, string) (wecomport.CustomerAcquisitionLinkWriteResult, error) {
	provider.deleteCalls++
	return provider.write, nil
}

func TestCustomerAcquisitionLinkCreateIsDurableAndIdempotent(t *testing.T) {
	link := customerAcquisitionLinkFixture()
	provider := &customerAcquisitionLinkProvider{write: wecomport.CustomerAcquisitionLinkWriteResult{Outcome: wecomport.CustomerAcquisitionLinkExecuted, Link: &link, OutcomeDigest: [32]byte{1}, BusinessEndpointDispatched: true, RealExternalCallExecuted: true}}
	receipts := &customerAcquisitionLinkReceipts{}
	service := NewCustomerAcquisitionLinkService(receipts, provider)
	command := CustomerAcquisitionLinkCommand{Actor: 7, IdempotencyKey: "customer-link-create-0001", Input: customerAcquisitionLinkInputFixture()}

	first, err := service.Create(context.Background(), command)
	if err != nil || first.State != CustomerAcquisitionLinkExecuted || first.Link == nil || first.Link.LinkID != link.LinkID || !first.BusinessEndpointDispatched || !first.RealExternalCallExecuted {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := service.Create(context.Background(), command)
	if err != nil || second.ID != first.ID || provider.createCalls != 1 {
		t.Fatalf("second=%+v calls=%d err=%v", second, provider.createCalls, err)
	}
}

func TestCustomerAcquisitionLinkAttemptedReplayBecomesUnknownWithoutProviderCall(t *testing.T) {
	command := CustomerAcquisitionLinkCommand{Actor: 7, IdempotencyKey: "customer-link-update-0001", LinkID: "link-1", Input: customerAcquisitionLinkInputFixture()}
	digest := customerAcquisitionLinkCommandDigest(CustomerAcquisitionLinkUpdate, command)
	receipts := &customerAcquisitionLinkReceipts{receipt: CustomerAcquisitionLinkReceipt{ID: 1, Operation: CustomerAcquisitionLinkUpdate, Command: command, RequestDigest: digest, State: CustomerAcquisitionLinkAttempted}}
	provider := &customerAcquisitionLinkProvider{}
	service := NewCustomerAcquisitionLinkService(receipts, provider)

	result, err := service.Update(context.Background(), command)
	if err != nil || result.State != CustomerAcquisitionLinkOutcomeUnknown || !result.BusinessEndpointDispatched || result.RealExternalCallExecuted || result.OutcomeDigest == ([32]byte{}) || provider.updateCalls != 0 {
		t.Fatalf("result=%+v calls=%d err=%v", result, provider.updateCalls, err)
	}
}

func TestCustomerAcquisitionLinkReadsAndRemainingWritesUseProvider(t *testing.T) {
	link := customerAcquisitionLinkFixture()
	for _, testCase := range []struct {
		name      string
		operation CustomerAcquisitionLinkOperation
		command   CustomerAcquisitionLinkCommand
		wantCalls func(*customerAcquisitionLinkProvider) int
	}{
		{name: "update", operation: CustomerAcquisitionLinkUpdate, command: CustomerAcquisitionLinkCommand{Actor: 7, IdempotencyKey: "customer-link-update-0002", LinkID: "link-1", Input: customerAcquisitionLinkInputFixture()}, wantCalls: func(provider *customerAcquisitionLinkProvider) int { return provider.updateCalls }},
		{name: "delete", operation: CustomerAcquisitionLinkDelete, command: CustomerAcquisitionLinkCommand{Actor: 7, IdempotencyKey: "customer-link-delete-0001", LinkID: "link-1"}, wantCalls: func(provider *customerAcquisitionLinkProvider) int { return provider.deleteCalls }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			write := wecomport.CustomerAcquisitionLinkWriteResult{Outcome: wecomport.CustomerAcquisitionLinkExecuted, OutcomeDigest: [32]byte{1}, BusinessEndpointDispatched: true, RealExternalCallExecuted: true}
			if testCase.operation != CustomerAcquisitionLinkDelete {
				write.Link = &link
			}
			provider := &customerAcquisitionLinkProvider{links: map[string]wecomport.CustomerAcquisitionLink{"link-1": link}, write: write}
			service := NewCustomerAcquisitionLinkService(&customerAcquisitionLinkReceipts{}, provider)
			var result CustomerAcquisitionLinkReceipt
			var err error
			if testCase.operation == CustomerAcquisitionLinkUpdate {
				result, err = service.Update(context.Background(), testCase.command)
			} else {
				result, err = service.Delete(context.Background(), testCase.command)
			}
			if err != nil || result.State != CustomerAcquisitionLinkExecuted || testCase.wantCalls(provider) != 1 {
				t.Fatalf("result=%+v calls=%d err=%v", result, testCase.wantCalls(provider), err)
			}
			if got, getErr := service.Get(context.Background(), "link-1"); getErr != nil || got.LinkID != "link-1" {
				t.Fatalf("get=%+v err=%v", got, getErr)
			}
			if page, listErr := service.List(context.Background(), "", 10); listErr != nil || len(page.Links) != 1 {
				t.Fatalf("page=%+v err=%v", page, listErr)
			}
		})
	}
}

func TestCustomerAcquisitionLinkReconcileRequiresMatchingProviderReadback(t *testing.T) {
	command := CustomerAcquisitionLinkCommand{Actor: 7, IdempotencyKey: "customer-link-create-0002", Input: customerAcquisitionLinkInputFixture()}
	digest := customerAcquisitionLinkCommandDigest(CustomerAcquisitionLinkCreate, command)
	receipts := &customerAcquisitionLinkReceipts{receipt: CustomerAcquisitionLinkReceipt{ID: 2, Operation: CustomerAcquisitionLinkCreate, Command: command, RequestDigest: digest, State: CustomerAcquisitionLinkOutcomeUnknown}}
	link := customerAcquisitionLinkFixture()
	provider := &customerAcquisitionLinkProvider{links: map[string]wecomport.CustomerAcquisitionLink{"link-1": link}}
	service := NewCustomerAcquisitionLinkService(receipts, provider)
	reconcile := ReconcileCustomerAcquisitionLinkCommand{ReceiptID: 2, Actor: 9, IdempotencyKey: "customer-link-reconcile-0001", LinkID: "link-1", Resolution: CustomerAcquisitionLinkProviderApplied, EvidenceDigest: [32]byte{9}}

	result, err := service.Reconcile(context.Background(), reconcile)
	if err != nil || result.State != CustomerAcquisitionLinkReconciled || result.Link == nil || result.Link.LinkID != "link-1" || provider.getCalls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, provider.getCalls, err)
	}
	if replayed, replayErr := service.Reconcile(context.Background(), reconcile); replayErr != nil || replayed.State != CustomerAcquisitionLinkReconciled || provider.getCalls != 1 {
		t.Fatalf("replayed=%+v calls=%d err=%v", replayed, provider.getCalls, replayErr)
	}
	conflict := reconcile
	conflict.IdempotencyKey = "customer-link-reconcile-other"
	if _, conflictErr := service.Reconcile(context.Background(), conflict); !errors.Is(conflictErr, ErrCustomerAcquisitionLinkConflict) {
		t.Fatalf("conflict err=%v", conflictErr)
	}

	receipts.receipt.State = CustomerAcquisitionLinkOutcomeUnknown
	provider.links["link-1"] = wecomport.CustomerAcquisitionLink{LinkID: "link-1", LinkName: "different", URL: link.URL, UserIDs: []string{"staff-a"}}
	if _, err = service.Reconcile(context.Background(), reconcile); !errors.Is(err, ErrCustomerAcquisitionLinkReconcile) {
		t.Fatalf("mismatched readback err=%v", err)
	}
}

func TestCustomerAcquisitionLinkUnsupportedTransitionNeverCallsProvider(t *testing.T) {
	provider := &customerAcquisitionLinkProvider{}
	service := NewCustomerAcquisitionLinkService(&customerAcquisitionLinkReceipts{}, provider)
	if err := service.SetEnabled(context.Background(), "link-1", true); !errors.Is(err, ErrCustomerAcquisitionLinkUnsupported) || provider.createCalls+provider.updateCalls+provider.deleteCalls != 0 {
		t.Fatalf("err=%v provider=%+v", err, provider)
	}
}

func TestCustomerAcquisitionLinkRejectsNamesOverThirtyRunes(t *testing.T) {
	input := customerAcquisitionLinkInputFixture()
	input.LinkName = strings.Repeat("测", 31)
	if validCustomerAcquisitionLinkInput(input) {
		t.Fatal("31-rune customer acquisition link name must be rejected")
	}
}

func customerAcquisitionLinkInputFixture() wecomport.CustomerAcquisitionLinkInput {
	return wecomport.CustomerAcquisitionLinkInput{LinkName: "获客链接", UserIDs: []string{"staff-a"}, DepartmentIDs: []int64{12}, SkipVerify: true}
}

func customerAcquisitionLinkFixture() wecomport.CustomerAcquisitionLink {
	input := customerAcquisitionLinkInputFixture()
	return wecomport.CustomerAcquisitionLink{LinkID: "link-1", LinkName: input.LinkName, URL: "https://work.weixin.qq.com/ca/link-1", UserIDs: input.UserIDs, DepartmentIDs: input.DepartmentIDs, SkipVerify: input.SkipVerify}
}
