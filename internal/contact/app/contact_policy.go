package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	ContactPolicyReasonManualOptOut = "manual_opt_out"
	ContactPolicyReasonCompliance   = "compliance_hold"
	ContactPolicyReasonOperatorHold = "operator_hold"

	contactPolicySetOperation   = "customer_contact_policy.set"
	contactPolicyClearOperation = "customer_contact_policy.clear"
)

var (
	ErrInvalidContactPolicy     = errors.New("invalid customer contact policy")
	ErrContactPolicyNotFound    = errors.New("customer contact policy customer not found")
	ErrContactPolicyConflict    = errors.New("customer contact policy conflict")
	ErrContactPolicyUnavailable = errors.New("customer contact policy unavailable")
)

type ContactPolicy struct {
	CustomerID                contactport.CustomerID `json:"customer_id"`
	Version                   int64                  `json:"version"`
	PolicyPresent             bool                   `json:"policy_present"`
	Eligible                  bool                   `json:"eligible"`
	SuppressionActive         bool                   `json:"suppression_active"`
	ReasonCode                *string                `json:"reason_code"`
	SuppressedUntil           *time.Time             `json:"suppressed_until"`
	LocalOnly                 bool                   `json:"local_only"`
	ProviderExecutionEligible bool                   `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool                   `json:"real_external_call_executed"`
	DeliveryProven            bool                   `json:"delivery_proven"`
}

type SetContactPolicyCommand struct {
	CustomerID      contactport.CustomerID
	ExpectedVersion int64
	ReasonCode      string
	SuppressedUntil *time.Time
	ActorID         int64
	IdempotencyKey  string
}

type ClearContactPolicyCommand struct {
	CustomerID      contactport.CustomerID
	ExpectedVersion int64
	ActorID         int64
	IdempotencyKey  string
}

type StoredContactPolicy struct {
	CustomerID      contactport.CustomerID
	ReasonCode      string
	SuppressedUntil *time.Time
	Version         int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ContactPolicyReceiptReservation struct {
	Operation     string
	ActorScope    string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type ContactPolicyReceipt struct {
	ID             int64
	Operation      string
	ActorScope     string
	KeyDigest      [32]byte
	PayloadDigest  [32]byte
	State          string
	ResultSnapshot json.RawMessage
}

// ContactPolicyStore is Contact-internal and every method requires the
// transaction context created by UnitOfWork.
type ContactPolicyStore interface {
	ReadActiveCustomerPolicy(context.Context, contactport.CustomerID, time.Time) (ContactPolicy, error)
	ReserveContactPolicyReceipt(context.Context, ContactPolicyReceiptReservation) (ContactPolicyReceipt, bool, error)
	CompleteContactPolicyReceipt(context.Context, int64, json.RawMessage, time.Time) (ContactPolicyReceipt, error)
	LockContactPolicyCustomer(context.Context, contactport.CustomerID) error
	ReadStoredContactPolicy(context.Context, contactport.CustomerID) (StoredContactPolicy, bool, error)
	InsertContactPolicy(context.Context, StoredContactPolicy) (StoredContactPolicy, error)
	UpdateContactPolicy(context.Context, StoredContactPolicy, int64) (StoredContactPolicy, error)
	DeleteContactPolicy(context.Context, contactport.CustomerID, int64) (bool, error)
}

type ContactPolicyService struct {
	uow    platformport.UnitOfWork
	store  ContactPolicyStore
	events eventport.Appender
	now    func() time.Time
}

func NewContactPolicyService(uow platformport.UnitOfWork, store ContactPolicyStore, events eventport.Appender) *ContactPolicyService {
	return &ContactPolicyService{uow: uow, store: store, events: events, now: time.Now}
}

func (service *ContactPolicyService) Get(ctx context.Context, customerID contactport.CustomerID) (ContactPolicy, error) {
	if !contactPolicyServiceReady(service) || ctx == nil || ctx.Err() != nil {
		return ContactPolicy{}, ErrContactPolicyUnavailable
	}
	if customerID <= 0 {
		return ContactPolicy{}, ErrContactPolicyNotFound
	}
	now := service.now().UTC()
	if now.IsZero() {
		return ContactPolicy{}, ErrContactPolicyUnavailable
	}
	var result ContactPolicy
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var readErr error
		result, readErr = service.store.ReadActiveCustomerPolicy(txCtx, customerID, now)
		return readErr
	})
	if err != nil {
		return ContactPolicy{}, classifyContactPolicyError(err)
	}
	if !validContactPolicy(result, customerID) {
		return ContactPolicy{}, ErrContactPolicyUnavailable
	}
	return cloneContactPolicy(result), nil
}

func (service *ContactPolicyService) Set(ctx context.Context, command SetContactPolicyCommand) (ContactPolicy, error) {
	if !contactPolicyServiceReady(service) || ctx == nil || ctx.Err() != nil {
		return ContactPolicy{}, ErrContactPolicyUnavailable
	}
	now := service.now().UTC()
	if !validSetContactPolicy(command, now) {
		return ContactPolicy{}, ErrInvalidContactPolicy
	}
	payload, err := json.Marshal(struct {
		CustomerID      contactport.CustomerID `json:"customer_id"`
		ExpectedVersion int64                  `json:"expected_version"`
		ReasonCode      string                 `json:"reason_code"`
		SuppressedUntil *time.Time             `json:"suppressed_until"`
	}{command.CustomerID, command.ExpectedVersion, command.ReasonCode, utcTimePointer(command.SuppressedUntil)})
	if err != nil {
		return ContactPolicy{}, ErrInvalidContactPolicy
	}
	return service.mutate(ctx, contactPolicySetOperation, command.CustomerID, command.ActorID, command.IdempotencyKey, sha256.Sum256(payload), func(txCtx context.Context) (ContactPolicy, error) {
		current, present, readErr := service.store.ReadStoredContactPolicy(txCtx, command.CustomerID)
		if readErr != nil {
			return ContactPolicy{}, readErr
		}
		if !present {
			if command.ExpectedVersion != 0 {
				return ContactPolicy{}, ErrContactPolicyConflict
			}
			current, readErr = service.store.InsertContactPolicy(txCtx, StoredContactPolicy{
				CustomerID: command.CustomerID, ReasonCode: command.ReasonCode,
				SuppressedUntil: utcTimePointer(command.SuppressedUntil), CreatedAt: now, UpdatedAt: now,
			})
		} else {
			if current.Version != command.ExpectedVersion {
				return ContactPolicy{}, ErrContactPolicyConflict
			}
			current.ReasonCode = command.ReasonCode
			current.SuppressedUntil = utcTimePointer(command.SuppressedUntil)
			current.UpdatedAt = now
			current, readErr = service.store.UpdateContactPolicy(txCtx, current, command.ExpectedVersion)
		}
		if readErr != nil {
			return ContactPolicy{}, readErr
		}
		return contactPolicyProjection(current, now), nil
	})
}

func (service *ContactPolicyService) Clear(ctx context.Context, command ClearContactPolicyCommand) (ContactPolicy, error) {
	if !contactPolicyServiceReady(service) || ctx == nil || ctx.Err() != nil {
		return ContactPolicy{}, ErrContactPolicyUnavailable
	}
	now := service.now().UTC()
	if command.CustomerID <= 0 || command.ExpectedVersion <= 0 || command.ActorID <= 0 || !validContactPolicyIdempotencyKey(command.IdempotencyKey) || now.IsZero() {
		return ContactPolicy{}, ErrInvalidContactPolicy
	}
	payload, err := json.Marshal(struct {
		CustomerID      contactport.CustomerID `json:"customer_id"`
		ExpectedVersion int64                  `json:"expected_version"`
	}{command.CustomerID, command.ExpectedVersion})
	if err != nil {
		return ContactPolicy{}, ErrInvalidContactPolicy
	}
	return service.mutate(ctx, contactPolicyClearOperation, command.CustomerID, command.ActorID, command.IdempotencyKey, sha256.Sum256(payload), func(txCtx context.Context) (ContactPolicy, error) {
		current, present, readErr := service.store.ReadStoredContactPolicy(txCtx, command.CustomerID)
		if readErr != nil {
			return ContactPolicy{}, readErr
		}
		if !present || current.Version != command.ExpectedVersion {
			return ContactPolicy{}, ErrContactPolicyConflict
		}
		deleted, deleteErr := service.store.DeleteContactPolicy(txCtx, command.CustomerID, command.ExpectedVersion)
		if deleteErr != nil {
			return ContactPolicy{}, deleteErr
		}
		if !deleted {
			return ContactPolicy{}, ErrContactPolicyConflict
		}
		return emptyContactPolicy(command.CustomerID), nil
	})
}

func (service *ContactPolicyService) mutate(
	ctx context.Context,
	operation string,
	customerID contactport.CustomerID,
	actorID int64,
	idempotencyKey string,
	payloadDigest [32]byte,
	write func(context.Context) (ContactPolicy, error),
) (result ContactPolicy, err error) {
	now := service.now().UTC()
	reservation := ContactPolicyReceiptReservation{
		Operation: operation, ActorScope: contactPolicyActorScope(actorID),
		KeyDigest: sha256.Sum256([]byte(idempotencyKey)), PayloadDigest: payloadDigest, CreatedAt: now,
	}
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveContactPolicyReceipt(txCtx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !contactPolicyReceiptMatches(receipt, reservation) {
			return ErrContactPolicyUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrContactPolicyConflict
		}
		if !owned {
			if receipt.State != "completed" || !decodeContactPolicy(receipt.ResultSnapshot, &result) || !validContactPolicy(result, customerID) {
				return ErrContactPolicyUnavailable
			}
			return nil
		}
		if err := service.store.LockContactPolicyCustomer(txCtx, customerID); err != nil {
			return err
		}
		result, err = write(txCtx)
		if err != nil {
			return err
		}
		if !validContactPolicy(result, customerID) {
			return ErrContactPolicyUnavailable
		}
		readback, readErr := service.store.ReadActiveCustomerPolicy(txCtx, customerID, now)
		if readErr != nil || !sameContactPolicy(result, readback) {
			return errors.Join(ErrContactPolicyUnavailable, readErr)
		}
		result = readback
		eventPayload, marshalErr := json.Marshal(struct {
			CustomerID contactport.CustomerID `json:"customer_id"`
			Operation  string                 `json:"operation"`
			Version    int64                  `json:"version"`
		}{customerID, operation, result.Version})
		if marshalErr != nil {
			return marshalErr
		}
		if _, appendErr := service.events.Append(txCtx, eventport.Event{
			Type: eventport.EvCustomerContactPolicyChanged, CustomerID: eventport.CustomerID(customerID),
			Payload: eventPayload, OccurredAt: now,
			IdempotencyKey: "customer-contact-policy:" + operation + ":" + strconv.FormatInt(receipt.ID, 10),
		}); appendErr != nil {
			return appendErr
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		completed, completeErr := service.store.CompleteContactPolicyReceipt(txCtx, receipt.ID, snapshot, now)
		if completeErr != nil || !contactPolicyReceiptMatches(completed, reservation) || completed.State != "completed" || !jsonEquivalentContactPolicy(completed.ResultSnapshot, snapshot) {
			return errors.Join(ErrContactPolicyUnavailable, completeErr)
		}
		return nil
	})
	if err != nil {
		return ContactPolicy{}, classifyContactPolicyError(err)
	}
	return cloneContactPolicy(result), nil
}

func validSetContactPolicy(command SetContactPolicyCommand, now time.Time) bool {
	if command.CustomerID <= 0 || command.ExpectedVersion < 0 || command.ActorID <= 0 || !validContactPolicyReason(command.ReasonCode) || !validContactPolicyIdempotencyKey(command.IdempotencyKey) || now.IsZero() {
		return false
	}
	return command.SuppressedUntil == nil || command.SuppressedUntil.UTC().After(now)
}

func validContactPolicyReason(reason string) bool {
	return reason == ContactPolicyReasonManualOptOut || reason == ContactPolicyReasonCompliance || reason == ContactPolicyReasonOperatorHold
}

func validContactPolicyIdempotencyKey(key string) bool {
	return utf8.ValidString(key) && strings.TrimSpace(key) == key && utf8.RuneCountInString(key) >= 16 && utf8.RuneCountInString(key) <= 128
}

func contactPolicyServiceReady(service *ContactPolicyService) bool {
	return service != nil && service.uow != nil && service.store != nil && service.events != nil && service.now != nil
}

func contactPolicyActorScope(actorID int64) string {
	return "customer_contact_policy:actor:" + strconv.FormatInt(actorID, 10)
}

func contactPolicyReceiptMatches(receipt ContactPolicyReceipt, reservation ContactPolicyReceiptReservation) bool {
	return receipt.ID > 0 && receipt.Operation == reservation.Operation && receipt.ActorScope == reservation.ActorScope &&
		subtle.ConstantTimeCompare(receipt.KeyDigest[:], reservation.KeyDigest[:]) == 1 &&
		(receipt.State == "reserved" || receipt.State == "completed")
}

func contactPolicyProjection(stored StoredContactPolicy, now time.Time) ContactPolicy {
	active := stored.SuppressedUntil == nil || stored.SuppressedUntil.After(now)
	reason := stored.ReasonCode
	return ContactPolicy{
		CustomerID: stored.CustomerID, Version: stored.Version, PolicyPresent: true,
		Eligible: !active, SuppressionActive: active, ReasonCode: &reason,
		SuppressedUntil: utcTimePointer(stored.SuppressedUntil), LocalOnly: true,
		ProviderExecutionEligible: false, RealExternalCallExecuted: false, DeliveryProven: false,
	}
}

func emptyContactPolicy(customerID contactport.CustomerID) ContactPolicy {
	return ContactPolicy{CustomerID: customerID, Eligible: true, LocalOnly: true}
}

func validContactPolicy(value ContactPolicy, customerID contactport.CustomerID) bool {
	if value.CustomerID != customerID || customerID <= 0 || !value.LocalOnly || value.ProviderExecutionEligible || value.RealExternalCallExecuted || value.DeliveryProven || value.Eligible == value.SuppressionActive {
		return false
	}
	if !value.PolicyPresent {
		return value.Version == 0 && value.Eligible && value.ReasonCode == nil && value.SuppressedUntil == nil
	}
	return value.Version > 0 && value.ReasonCode != nil && validContactPolicyReason(*value.ReasonCode)
}

func sameContactPolicy(left, right ContactPolicy) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func cloneContactPolicy(value ContactPolicy) ContactPolicy {
	if value.ReasonCode != nil {
		reason := *value.ReasonCode
		value.ReasonCode = &reason
	}
	value.SuppressedUntil = utcTimePointer(value.SuppressedUntil)
	return value
}

func utcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func decodeContactPolicy(raw json.RawMessage, destination *ContactPolicy) bool {
	if destination == nil || len(raw) == 0 {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF) && json.Valid(raw)
}

func jsonEquivalentContactPolicy(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func classifyContactPolicyError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidContactPolicy), errors.Is(err, ErrContactPolicyNotFound), errors.Is(err, ErrContactPolicyConflict):
		return err
	default:
		return errors.Join(ErrContactPolicyUnavailable, err)
	}
}
