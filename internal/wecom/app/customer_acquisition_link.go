package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

var (
	ErrInvalidCustomerAcquisitionLinkCommand = errors.New("invalid customer acquisition link command")
	ErrCustomerAcquisitionLinkUnavailable    = errors.New("customer acquisition link unavailable")
	ErrCustomerAcquisitionLinkConflict       = errors.New("customer acquisition link idempotency conflict")
	ErrCustomerAcquisitionLinkReconcile      = errors.New("customer acquisition link reconciliation required")
	ErrCustomerAcquisitionLinkUnsupported    = errors.New("customer acquisition link transition unsupported by provider")
)

type CustomerAcquisitionLinkOperation string

const (
	CustomerAcquisitionLinkCreate CustomerAcquisitionLinkOperation = "create"
	CustomerAcquisitionLinkUpdate CustomerAcquisitionLinkOperation = "update"
	CustomerAcquisitionLinkDelete CustomerAcquisitionLinkOperation = "delete"
)

type CustomerAcquisitionLinkReceiptState string

const (
	CustomerAcquisitionLinkReserved       CustomerAcquisitionLinkReceiptState = "accepted"
	CustomerAcquisitionLinkAttempted      CustomerAcquisitionLinkReceiptState = "attempted"
	CustomerAcquisitionLinkExecuted       CustomerAcquisitionLinkReceiptState = "executed"
	CustomerAcquisitionLinkFinalFailed    CustomerAcquisitionLinkReceiptState = "final_failed"
	CustomerAcquisitionLinkOutcomeUnknown CustomerAcquisitionLinkReceiptState = "outcome_unknown"
	CustomerAcquisitionLinkReconciled     CustomerAcquisitionLinkReceiptState = "reconciled"
)

type CustomerAcquisitionLinkCommand struct {
	Actor          int64
	IdempotencyKey string
	LinkID         string
	Input          wecomport.CustomerAcquisitionLinkInput
}

type CustomerAcquisitionLinkReceipt struct {
	ID                         int64
	Operation                  CustomerAcquisitionLinkOperation
	Command                    CustomerAcquisitionLinkCommand
	RequestDigest              [32]byte
	State                      CustomerAcquisitionLinkReceiptState
	Link                       *wecomport.CustomerAcquisitionLink
	OutcomeDigest              [32]byte
	BusinessEndpointDispatched bool
	RealExternalCallExecuted   bool
	ReconcileKeyDigest         [32]byte
	EvidenceDigest             [32]byte
	Resolution                 CustomerAcquisitionLinkResolution
}

type CustomerAcquisitionLinkCompletion struct {
	ReceiptID                  int64
	State                      CustomerAcquisitionLinkReceiptState
	Link                       *wecomport.CustomerAcquisitionLink
	OutcomeDigest              [32]byte
	BusinessEndpointDispatched bool
	RealExternalCallExecuted   bool
	ReconcileActor             int64
	ReconcileKeyDigest         [32]byte
	EvidenceDigest             [32]byte
	Resolution                 CustomerAcquisitionLinkResolution
}

// Each store method owns its short local transaction. Provider calls happen
// between MarkAttempted and Complete and therefore never hold a DB transaction.
type CustomerAcquisitionLinkReceiptStore interface {
	ReserveCustomerAcquisitionLink(context.Context, CustomerAcquisitionLinkOperation, CustomerAcquisitionLinkCommand, [32]byte) (CustomerAcquisitionLinkReceipt, error)
	MarkCustomerAcquisitionLinkAttempted(context.Context, int64) (CustomerAcquisitionLinkReceipt, error)
	CompleteCustomerAcquisitionLink(context.Context, CustomerAcquisitionLinkCompletion) (CustomerAcquisitionLinkReceipt, error)
	GetCustomerAcquisitionLinkReceipt(context.Context, int64) (CustomerAcquisitionLinkReceipt, error)
}

type CustomerAcquisitionLinkResolution string

const (
	CustomerAcquisitionLinkProviderApplied    CustomerAcquisitionLinkResolution = "provider_applied"
	CustomerAcquisitionLinkProviderNotApplied CustomerAcquisitionLinkResolution = "provider_not_applied"
)

type ReconcileCustomerAcquisitionLinkCommand struct {
	ReceiptID      int64
	Actor          int64
	IdempotencyKey string
	LinkID         string
	Resolution     CustomerAcquisitionLinkResolution
	EvidenceDigest [32]byte
}

type CustomerAcquisitionLinkService struct {
	receipts CustomerAcquisitionLinkReceiptStore
	provider wecomport.CustomerAcquisitionLinkProvider
}

func NewCustomerAcquisitionLinkService(receipts CustomerAcquisitionLinkReceiptStore, provider wecomport.CustomerAcquisitionLinkProvider) *CustomerAcquisitionLinkService {
	return &CustomerAcquisitionLinkService{receipts: receipts, provider: provider}
}

func (service *CustomerAcquisitionLinkService) List(ctx context.Context, cursor string, limit int) (wecomport.CustomerAcquisitionLinkPage, error) {
	if !service.ready(ctx) || !validCustomerAcquisitionLinkText(cursor, 0, 1024) || limit < 1 || limit > 100 {
		return wecomport.CustomerAcquisitionLinkPage{}, ErrInvalidCustomerAcquisitionLinkCommand
	}
	page, err := service.provider.ListCustomerAcquisitionLinks(ctx, cursor, limit)
	if err != nil || len(page.Links) > limit {
		return wecomport.CustomerAcquisitionLinkPage{}, errors.Join(ErrCustomerAcquisitionLinkUnavailable, err)
	}
	return page, nil
}

func (service *CustomerAcquisitionLinkService) Get(ctx context.Context, linkID string) (wecomport.CustomerAcquisitionLink, error) {
	if !service.ready(ctx) || !validCustomerAcquisitionLinkText(linkID, 1, 1024) {
		return wecomport.CustomerAcquisitionLink{}, ErrInvalidCustomerAcquisitionLinkCommand
	}
	link, err := service.provider.GetCustomerAcquisitionLink(ctx, linkID)
	if err != nil || link.LinkID != linkID {
		return wecomport.CustomerAcquisitionLink{}, errors.Join(ErrCustomerAcquisitionLinkUnavailable, err)
	}
	return link, nil
}

func (service *CustomerAcquisitionLinkService) Create(ctx context.Context, command CustomerAcquisitionLinkCommand) (CustomerAcquisitionLinkReceipt, error) {
	return service.mutate(ctx, CustomerAcquisitionLinkCreate, command)
}

func (service *CustomerAcquisitionLinkService) Update(ctx context.Context, command CustomerAcquisitionLinkCommand) (CustomerAcquisitionLinkReceipt, error) {
	return service.mutate(ctx, CustomerAcquisitionLinkUpdate, command)
}

func (service *CustomerAcquisitionLinkService) Delete(ctx context.Context, command CustomerAcquisitionLinkCommand) (CustomerAcquisitionLinkReceipt, error) {
	return service.mutate(ctx, CustomerAcquisitionLinkDelete, command)
}

// The official provider has no enable/disable transition. Returning unsupported
// is safer than silently mapping legacy actions to create/delete.
func (*CustomerAcquisitionLinkService) SetEnabled(context.Context, string, bool) error {
	return ErrCustomerAcquisitionLinkUnsupported
}

func (service *CustomerAcquisitionLinkService) mutate(ctx context.Context, operation CustomerAcquisitionLinkOperation, command CustomerAcquisitionLinkCommand) (CustomerAcquisitionLinkReceipt, error) {
	if !service.ready(ctx) || !validCustomerAcquisitionLinkCommand(operation, command) {
		return CustomerAcquisitionLinkReceipt{}, ErrInvalidCustomerAcquisitionLinkCommand
	}
	digest := customerAcquisitionLinkCommandDigest(operation, command)
	receipt, err := service.receipts.ReserveCustomerAcquisitionLink(ctx, operation, command, digest)
	if err != nil {
		return CustomerAcquisitionLinkReceipt{}, errors.Join(ErrCustomerAcquisitionLinkUnavailable, err)
	}
	if receipt.Operation != operation || receipt.RequestDigest != digest {
		return CustomerAcquisitionLinkReceipt{}, ErrCustomerAcquisitionLinkConflict
	}
	switch receipt.State {
	case CustomerAcquisitionLinkExecuted, CustomerAcquisitionLinkFinalFailed, CustomerAcquisitionLinkOutcomeUnknown, CustomerAcquisitionLinkReconciled:
		return receipt, nil
	case CustomerAcquisitionLinkAttempted:
		return service.complete(ctx, CustomerAcquisitionLinkCompletion{
			ReceiptID: receipt.ID, State: CustomerAcquisitionLinkOutcomeUnknown, BusinessEndpointDispatched: true,
			OutcomeDigest: sha256.Sum256([]byte("wecom.customer_acquisition.link.outcome.interrupted.v1\x00" + strconv.FormatInt(receipt.ID, 10))),
		})
	case CustomerAcquisitionLinkReserved:
	default:
		return CustomerAcquisitionLinkReceipt{}, ErrCustomerAcquisitionLinkUnavailable
	}
	receipt, err = service.receipts.MarkCustomerAcquisitionLinkAttempted(ctx, receipt.ID)
	if err != nil || receipt.State != CustomerAcquisitionLinkAttempted || receipt.RequestDigest != digest {
		return CustomerAcquisitionLinkReceipt{}, errors.Join(ErrCustomerAcquisitionLinkUnavailable, err)
	}
	result, providerErr := service.callProvider(ctx, operation, command)
	return service.complete(ctx, customerAcquisitionLinkCompletion(operation, receipt.ID, result, providerErr))
}

func (service *CustomerAcquisitionLinkService) Reconcile(ctx context.Context, command ReconcileCustomerAcquisitionLinkCommand) (CustomerAcquisitionLinkReceipt, error) {
	if !service.ready(ctx) || command.ReceiptID < 1 || command.Actor < 1 || !validCustomerAcquisitionLinkText(command.IdempotencyKey, 16, 128) ||
		!validCustomerAcquisitionLinkText(command.LinkID, 1, 1024) || command.EvidenceDigest == ([32]byte{}) ||
		(command.Resolution != CustomerAcquisitionLinkProviderApplied && command.Resolution != CustomerAcquisitionLinkProviderNotApplied) {
		return CustomerAcquisitionLinkReceipt{}, ErrInvalidCustomerAcquisitionLinkCommand
	}
	receipt, err := service.receipts.GetCustomerAcquisitionLinkReceipt(ctx, command.ReceiptID)
	if err != nil {
		return CustomerAcquisitionLinkReceipt{}, errors.Join(ErrCustomerAcquisitionLinkUnavailable, err)
	}
	if (receipt.Operation == CustomerAcquisitionLinkUpdate || receipt.Operation == CustomerAcquisitionLinkDelete) && command.LinkID != receipt.Command.LinkID {
		return CustomerAcquisitionLinkReceipt{}, ErrCustomerAcquisitionLinkReconcile
	}
	if receipt.State == CustomerAcquisitionLinkReconciled {
		if receipt.ReconcileKeyDigest != sha256.Sum256([]byte(command.IdempotencyKey)) || receipt.EvidenceDigest != command.EvidenceDigest || receipt.Resolution != command.Resolution {
			return CustomerAcquisitionLinkReceipt{}, ErrCustomerAcquisitionLinkConflict
		}
		return receipt, nil
	}
	if receipt.State != CustomerAcquisitionLinkOutcomeUnknown {
		return CustomerAcquisitionLinkReceipt{}, ErrCustomerAcquisitionLinkReconcile
	}
	observed, err := service.provider.GetCustomerAcquisitionLink(ctx, command.LinkID)
	if receipt.Operation == CustomerAcquisitionLinkDelete && command.Resolution == CustomerAcquisitionLinkProviderNotApplied && errors.Is(err, wecomport.ErrCustomerAcquisitionLinkNotFound) {
		return service.complete(ctx, CustomerAcquisitionLinkCompletion{
			ReceiptID: receipt.ID, State: CustomerAcquisitionLinkReconciled,
			OutcomeDigest:              sha256.Sum256([]byte("wecom.customer_acquisition.link.outcome.not_found.v1\x00" + command.LinkID)),
			BusinessEndpointDispatched: true, RealExternalCallExecuted: true, ReconcileActor: command.Actor,
			ReconcileKeyDigest: sha256.Sum256([]byte(command.IdempotencyKey)), EvidenceDigest: command.EvidenceDigest,
			Resolution: command.Resolution,
		})
	}
	if err != nil || !customerAcquisitionLinkReadbackProves(receipt, observed, command.Resolution) {
		return CustomerAcquisitionLinkReceipt{}, errors.Join(ErrCustomerAcquisitionLinkReconcile, err)
	}
	return service.complete(ctx, CustomerAcquisitionLinkCompletion{
		ReceiptID: receipt.ID, State: CustomerAcquisitionLinkReconciled, Link: &observed,
		OutcomeDigest:              sha256.Sum256([]byte("wecom.customer_acquisition.link.outcome.readback.v1\x00" + observed.LinkID)),
		BusinessEndpointDispatched: true, RealExternalCallExecuted: true, ReconcileActor: command.Actor,
		ReconcileKeyDigest: sha256.Sum256([]byte(command.IdempotencyKey)), EvidenceDigest: command.EvidenceDigest,
		Resolution: command.Resolution,
	})
}

func (service *CustomerAcquisitionLinkService) callProvider(ctx context.Context, operation CustomerAcquisitionLinkOperation, command CustomerAcquisitionLinkCommand) (wecomport.CustomerAcquisitionLinkWriteResult, error) {
	switch operation {
	case CustomerAcquisitionLinkCreate:
		return service.provider.CreateCustomerAcquisitionLink(ctx, command.Input)
	case CustomerAcquisitionLinkUpdate:
		return service.provider.UpdateCustomerAcquisitionLink(ctx, command.LinkID, command.Input)
	case CustomerAcquisitionLinkDelete:
		return service.provider.DeleteCustomerAcquisitionLink(ctx, command.LinkID)
	default:
		return wecomport.CustomerAcquisitionLinkWriteResult{}, ErrCustomerAcquisitionLinkUnsupported
	}
}

func (service *CustomerAcquisitionLinkService) complete(ctx context.Context, completion CustomerAcquisitionLinkCompletion) (CustomerAcquisitionLinkReceipt, error) {
	receipt, err := service.receipts.CompleteCustomerAcquisitionLink(ctx, completion)
	if err != nil || receipt.ID != completion.ReceiptID || receipt.State != completion.State {
		return CustomerAcquisitionLinkReceipt{}, errors.Join(ErrCustomerAcquisitionLinkUnavailable, err)
	}
	return receipt, nil
}

func customerAcquisitionLinkCompletion(operation CustomerAcquisitionLinkOperation, receiptID int64, result wecomport.CustomerAcquisitionLinkWriteResult, providerErr error) CustomerAcquisitionLinkCompletion {
	completion := CustomerAcquisitionLinkCompletion{ReceiptID: receiptID, Link: result.Link, OutcomeDigest: result.OutcomeDigest, BusinessEndpointDispatched: result.BusinessEndpointDispatched, RealExternalCallExecuted: result.RealExternalCallExecuted}
	if providerErr != nil {
		completion.State, completion.BusinessEndpointDispatched = CustomerAcquisitionLinkOutcomeUnknown, true
		if errors.Is(providerErr, wecomport.ErrCustomerAcquisitionLinkNotDispatched) {
			completion.State, completion.BusinessEndpointDispatched = CustomerAcquisitionLinkFinalFailed, false
		}
		completion.OutcomeDigest = sha256.Sum256([]byte("wecom.customer_acquisition.link.outcome.local_failure.v1\x00" + strconv.FormatInt(receiptID, 10)))
		return completion
	}
	switch result.Outcome {
	case wecomport.CustomerAcquisitionLinkExecuted:
		completion.State = CustomerAcquisitionLinkExecuted
		if result.OutcomeDigest == ([32]byte{}) || !result.BusinessEndpointDispatched || !result.RealExternalCallExecuted || operation != CustomerAcquisitionLinkDelete && result.Link == nil || operation == CustomerAcquisitionLinkDelete && result.Link != nil {
			completion.State = CustomerAcquisitionLinkOutcomeUnknown
		}
	case wecomport.CustomerAcquisitionLinkFinalFailed:
		completion.State = CustomerAcquisitionLinkFinalFailed
	case wecomport.CustomerAcquisitionLinkOutcomeUnknown:
		completion.State = CustomerAcquisitionLinkOutcomeUnknown
	default:
		completion.State = CustomerAcquisitionLinkOutcomeUnknown
	}
	return completion
}

func validCustomerAcquisitionLinkCommand(operation CustomerAcquisitionLinkOperation, command CustomerAcquisitionLinkCommand) bool {
	if command.Actor < 1 || !validCustomerAcquisitionLinkText(command.IdempotencyKey, 16, 128) {
		return false
	}
	switch operation {
	case CustomerAcquisitionLinkCreate:
		return command.LinkID == "" && validCustomerAcquisitionLinkInput(command.Input)
	case CustomerAcquisitionLinkUpdate:
		return validCustomerAcquisitionLinkText(command.LinkID, 1, 1024) && validCustomerAcquisitionLinkInput(command.Input)
	case CustomerAcquisitionLinkDelete:
		return validCustomerAcquisitionLinkText(command.LinkID, 1, 1024) && command.Input.LinkName == "" && len(command.Input.UserIDs)+len(command.Input.DepartmentIDs) == 0 && !command.Input.SkipVerify
	default:
		return false
	}
}

func validCustomerAcquisitionLinkInput(input wecomport.CustomerAcquisitionLinkInput) bool {
	return validCustomerAcquisitionLinkName(input.LinkName) && validCustomerAcquisitionLinkStrings(input.UserIDs, 500) && validCustomerAcquisitionLinkInt64s(input.DepartmentIDs, 500) && len(input.UserIDs)+len(input.DepartmentIDs) > 0
}

func validCustomerAcquisitionLinkName(value string) bool {
	return validCustomerAcquisitionLinkText(value, 1, 120) && utf8.RuneCountInString(value) <= 30
}

func customerAcquisitionLinkReadbackProves(receipt CustomerAcquisitionLinkReceipt, observed wecomport.CustomerAcquisitionLink, resolution CustomerAcquisitionLinkResolution) bool {
	if receipt.Operation == CustomerAcquisitionLinkDelete {
		return resolution == CustomerAcquisitionLinkProviderNotApplied && observed.LinkID == receipt.Command.LinkID
	}
	want := receipt.Command.Input
	if receipt.Operation == CustomerAcquisitionLinkUpdate && observed.LinkID != receipt.Command.LinkID {
		return false
	}
	return resolution == CustomerAcquisitionLinkProviderApplied && observed.LinkID != "" && observed.LinkName == want.LinkName && observed.SkipVerify == want.SkipVerify && sameStringSet(observed.UserIDs, want.UserIDs) && sameInt64Set(observed.DepartmentIDs, want.DepartmentIDs)
}

func customerAcquisitionLinkCommandDigest(operation CustomerAcquisitionLinkOperation, command CustomerAcquisitionLinkCommand) [32]byte {
	input := command.Input
	input.UserIDs, input.DepartmentIDs = append([]string(nil), input.UserIDs...), append([]int64(nil), input.DepartmentIDs...)
	sort.Strings(input.UserIDs)
	sort.Slice(input.DepartmentIDs, func(left, right int) bool { return input.DepartmentIDs[left] < input.DepartmentIDs[right] })
	payload, _ := json.Marshal(struct {
		Operation CustomerAcquisitionLinkOperation
		Actor     int64
		LinkID    string
		Input     wecomport.CustomerAcquisitionLinkInput
	}{operation, command.Actor, command.LinkID, input})
	return sha256.Sum256(payload)
}

func validCustomerAcquisitionLinkText(value string, minimum, maximum int) bool {
	return len(value) >= minimum && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validCustomerAcquisitionLinkStrings(values []string, maximum int) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validCustomerAcquisitionLinkText(value, 1, 1024) {
			return false
		}
		seen[value] = struct{}{}
	}
	return len(values) <= maximum && len(seen) == len(values)
}

func validCustomerAcquisitionLinkInt64s(values []int64, maximum int) bool {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value < 1 {
			return false
		}
		seen[value] = struct{}{}
	}
	return len(values) <= maximum && len(seen) == len(values)
}

func sameStringSet(left, right []string) bool {
	a, b := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

func sameInt64Set(left, right []int64) bool {
	a, b := append([]int64(nil), left...), append([]int64(nil), right...)
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	return reflect.DeepEqual(a, b)
}

func (service *CustomerAcquisitionLinkService) ready(ctx context.Context) bool {
	return service != nil && ctx != nil && service.receipts != nil && service.provider != nil
}
