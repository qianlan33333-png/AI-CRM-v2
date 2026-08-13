package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrIdentityBindFailed              = errors.New("identity bind failed")
	ErrIdentityBindIdempotencyConflict = errors.New("identity bind idempotency conflict")
)

const bindReceiptKeyVersion = int16(1)

// BindReceipt is the completed durable result of one Bind idempotency key.
// The raw key and identity value are never persisted in the receipt.
type BindReceipt struct {
	ID          int64
	Found       bool
	PayloadHMAC []byte
	Result      identityport.BindResult
}

type BindRecord struct {
	Status     identityport.BindStatus
	IdentityID int64
}

// BindStore keeps the receipt reservation, identity edge and completion in the
// same transaction supplied by BindService's UnitOfWork.
type BindStore interface {
	ReserveBindReceipt(context.Context, []byte, []byte) (BindReceipt, error)
	BindNormalized(context.Context, NormalizedIdentity, int64) (BindRecord, error)
	CompleteBindReceipt(context.Context, BindReceipt, identityport.BindResult) error
}

// BindService implements the deliberately narrow I-3 decision boundary. It
// may bind a floating edge, replay a same-customer edge, or reject an edge
// already held by another customer. Merge and review creation remain later
// slices, so this service never reassigns a non-floating identity.
type BindService struct {
	uow        platformport.UnitOfWork
	store      BindStore
	events     eventport.Appender
	receiptKey []byte
}

func NewBindService(uow platformport.UnitOfWork, store BindStore, events eventport.Appender, receiptKey []byte) *BindService {
	return &BindService{uow: uow, store: store, events: events, receiptKey: append([]byte(nil), receiptKey...)}
}

func (service *BindService) Bind(ctx context.Context, command identityport.BindCommand) (identityport.BindResult, error) {
	normalized, err := Normalize(command.Ref)
	if err != nil {
		return identityport.BindResult{}, err
	}
	if !validBindCommand(command) {
		return identityport.BindResult{}, ErrIdentityBindFailed
	}
	if service == nil || service.uow == nil || service.store == nil || service.events == nil || len(service.receiptKey) < sha256.Size || ctx == nil {
		return identityport.BindResult{}, ErrIdentityBindFailed
	}

	keyDigest, payloadHMAC, err := service.receiptDigests(command, normalized)
	if err != nil {
		return identityport.BindResult{}, errors.Join(ErrIdentityBindFailed, err)
	}
	var result identityport.BindResult
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, err := service.store.ReserveBindReceipt(txCtx, keyDigest, payloadHMAC)
		if err != nil {
			return err
		}
		if receipt.Found {
			if !hmac.Equal(receipt.PayloadHMAC, payloadHMAC) {
				return ErrIdentityBindIdempotencyConflict
			}
			if !validBindResult(receipt.Result) {
				return ErrIdentityBindFailed
			}
			result = receipt.Result
			return nil
		}

		record, err := service.store.BindNormalized(txCtx, normalized, int64(command.CustomerID))
		if err != nil {
			return err
		}
		switch record.Status {
		case identityport.BindBound:
			if record.IdentityID <= 0 {
				return ErrIdentityBindFailed
			}
			result = identityport.BindResult{Status: identityport.BindBound, CustomerID: command.CustomerID}
			if err := service.appendBoundEvent(txCtx, record.IdentityID, command.CustomerID, normalized); err != nil {
				return err
			}
		case identityport.BindAlreadyBound:
			result = identityport.BindResult{Status: identityport.BindAlreadyBound, CustomerID: command.CustomerID}
		case identityport.BindRejected:
			result = identityport.BindResult{Status: identityport.BindRejected}
		default:
			return ErrIdentityBindFailed
		}
		return service.store.CompleteBindReceipt(txCtx, receipt, result)
	})
	if err != nil {
		return identityport.BindResult{}, errors.Join(ErrIdentityBindFailed, err)
	}
	return result, nil
}

func (service *BindService) appendBoundEvent(ctx context.Context, identityID int64, customerID contactport.CustomerID, normalized NormalizedIdentity) error {
	payload, err := json.Marshal(struct {
		IdentityID        int64               `json:"identity_id"`
		CustomerID        int64               `json:"customer_id"`
		Kind              identityport.IDKind `json:"kind"`
		Scope             string              `json:"scope"`
		NormalizerVersion int16               `json:"normalizer_version"`
	}{identityID, int64(customerID), normalized.Kind, normalized.Scope, normalized.NormalizerVersion})
	if err != nil {
		return err
	}
	_, err = service.events.Append(ctx, eventport.Event{
		Type:           "identity.bound",
		CustomerID:     eventport.CustomerID(customerID),
		Payload:        payload,
		OccurredAt:     time.Now().UTC(),
		IdempotencyKey: "identity.bound:" + strconv.FormatInt(identityID, 10),
	})
	return err
}

func (service *BindService) receiptDigests(command identityport.BindCommand, normalized NormalizedIdentity) ([]byte, []byte, error) {
	payload, err := json.Marshal(struct {
		CustomerID int64                  `json:"customer_id"`
		Kind       identityport.IDKind    `json:"kind"`
		Scope      string                 `json:"scope"`
		Value      string                 `json:"normalized_value"`
		Assurance  identityport.Assurance `json:"assurance"`
		Source     string                 `json:"source"`
		Actor      string                 `json:"actor"`
	}{int64(command.CustomerID), normalized.Kind, normalized.Scope, normalized.NormalizedValue, command.Ref.Assurance, command.Ref.Source, string(command.Actor)})
	if err != nil {
		return nil, nil, err
	}
	return hmacDigest(service.receiptKey, "identity.bind.key.v1\x00"+command.IdempotencyKey), hmacDigest(service.receiptKey, "identity.bind.payload.v1\x00"+string(payload)), nil
}

func hmacDigest(key []byte, value string) []byte {
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write([]byte(value))
	return digest.Sum(nil)
}

func validBindCommand(command identityport.BindCommand) bool {
	return command.CustomerID > 0 && validBindText(string(command.Actor), 200) && validBindText(command.IdempotencyKey, 512) && validBindEvidence(command.Ref)
}

func validBindEvidence(ref identityport.IDRef) bool {
	return (ref.Assurance == identityport.AssuranceDeclared || ref.Assurance == identityport.AssuranceVerified) && validBindText(ref.Source, 200)
}

func validBindText(value string, maximum int) bool {
	return value == strings.TrimSpace(value) && value != "" && len(value) <= maximum && !containsControl(value)
}

func validBindResult(result identityport.BindResult) bool {
	switch result.Status {
	case identityport.BindBound, identityport.BindAlreadyBound:
		return result.CustomerID > 0 && result.PrimaryCustomerID == 0 && result.MergeAuditID == 0 && result.ReviewID == 0
	case identityport.BindRejected:
		return result.CustomerID == 0 && result.PrimaryCustomerID == 0 && result.MergeAuditID == 0 && result.ReviewID == 0
	default:
		return false
	}
}
