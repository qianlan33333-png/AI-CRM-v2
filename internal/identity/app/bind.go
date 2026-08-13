package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

const VerifiedPhoneMergeReviewPolicy = "verified_phone_manual_review_v1"

// BindReceipt is the completed durable result of one Bind idempotency key.
// The raw key and identity value are never persisted in the receipt.
type BindReceipt struct {
	ID          int64
	Found       bool
	PayloadHMAC []byte
	Result      identityport.BindResult
}

type BindRecord struct {
	Status                           identityport.BindStatus
	IdentityID                       int64
	ExistingCustomerID               contactport.CustomerID
	RequestedHasVerifiedWeCom        bool
	ExistingCustomerHasVerifiedWeCom bool
}

// AutoMergeAudit is the redacted, identity-owned audit fact for one automatic
// customer merge. It intentionally carries no raw or normalized identity
// value.
type AutoMergeAudit struct {
	PrimaryCustomerID  contactport.CustomerID
	MergedCustomerID   contactport.CustomerID
	PolicyVersion      string
	ReviewFingerprint  []byte
	FingerprintVersion int16
	Actor              contactport.Actor
	Detail             json.RawMessage
}

// BindStore keeps the receipt reservation, identity edge and completion in the
// same transaction supplied by BindService's UnitOfWork.
type BindStore interface {
	ReserveBindReceipt(context.Context, []byte, []byte) (BindReceipt, error)
	BindNormalized(context.Context, NormalizedIdentity, int64) (BindRecord, error)
	RebindIdentitiesForCustomerMerge(context.Context, contactport.CustomerID, contactport.CustomerID) error
	InsertAutoCustomerMergeAudit(context.Context, AutoMergeAudit) (int64, error)
	InsertVerifiedPhoneMergeReview(context.Context, int64, []contactport.CustomerID, []byte) (int64, error)
	CompleteBindReceipt(context.Context, BindReceipt, identityport.BindResult) error
}

// BindService implements the narrow I-3/I-4 decision boundary. It may bind
// a floating edge, replay a same-customer edge, reject an edge held by another
// customer, automatically merge only the verified-unionid policy case, or
// create a verified-phone manual review. Review resolution and every other
// merge policy remain later slices.
type BindService struct {
	uow        platformport.UnitOfWork
	store      BindStore
	contacts   contactport.MergePort
	events     eventport.Appender
	receiptKey []byte
}

func NewBindService(uow platformport.UnitOfWork, store BindStore, events eventport.Appender, receiptKey []byte) *BindService {
	return NewBindServiceWithMergePort(uow, store, nil, events, receiptKey)
}

func NewBindServiceWithMergePort(uow platformport.UnitOfWork, store BindStore, contacts contactport.MergePort, events eventport.Appender, receiptKey []byte) *BindService {
	return &BindService{uow: uow, store: store, contacts: contacts, events: events, receiptKey: append([]byte(nil), receiptKey...)}
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
			merged, attempted, err := service.mergeVerifiedUnionID(txCtx, command, normalized, record)
			if err != nil {
				return err
			}
			if attempted {
				result = merged
				break
			}
			review, attempted, err := service.createVerifiedPhoneMergeReview(txCtx, command, normalized, record)
			if err != nil {
				return err
			}
			if attempted {
				result = review
				break
			}
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

func (service *BindService) createVerifiedPhoneMergeReview(
	ctx context.Context,
	command identityport.BindCommand,
	normalized NormalizedIdentity,
	record BindRecord,
) (identityport.BindResult, bool, error) {
	if normalized.Kind != identityport.KindPhone || command.Ref.Assurance != identityport.AssuranceVerified ||
		record.IdentityID <= 0 || record.ExistingCustomerID <= 0 || record.ExistingCustomerID == command.CustomerID {
		return identityport.BindResult{}, false, nil
	}
	candidates := []contactport.CustomerID{command.CustomerID, record.ExistingCustomerID}
	if candidates[1] < candidates[0] {
		candidates[0], candidates[1] = candidates[1], candidates[0]
	}
	fingerprint, _ := service.mergeReviewFingerprint(normalized)
	reviewID, err := service.store.InsertVerifiedPhoneMergeReview(ctx, record.IdentityID, candidates, fingerprint)
	if err != nil || reviewID <= 0 {
		if err != nil {
			return identityport.BindResult{}, true, err
		}
		return identityport.BindResult{}, true, ErrIdentityBindFailed
	}
	if err = service.appendMergeReviewCreatedEvent(ctx, command.CustomerID, reviewID, candidates); err != nil {
		return identityport.BindResult{}, true, err
	}
	return identityport.BindResult{Status: identityport.BindManualReview, ReviewID: reviewID}, true, nil
}

func (service *BindService) mergeVerifiedUnionID(
	ctx context.Context,
	command identityport.BindCommand,
	normalized NormalizedIdentity,
	record BindRecord,
) (identityport.BindResult, bool, error) {
	if normalized.Kind != identityport.KindUnionID || command.Ref.Assurance != identityport.AssuranceVerified ||
		record.IdentityID <= 0 || record.ExistingCustomerID <= 0 || record.ExistingCustomerID == command.CustomerID ||
		record.RequestedHasVerifiedWeCom == record.ExistingCustomerHasVerifiedWeCom {
		return identityport.BindResult{}, false, nil
	}
	if service.contacts == nil {
		return identityport.BindResult{}, true, ErrIdentityBindFailed
	}

	primary, merged := command.CustomerID, record.ExistingCustomerID
	if record.ExistingCustomerHasVerifiedWeCom {
		primary, merged = record.ExistingCustomerID, command.CustomerID
	}
	fingerprint, displayFingerprint := service.mergeFingerprint(normalized)
	detail, err := json.Marshal(struct {
		PolicyVersion      string `json:"policy_version"`
		Mode               string `json:"mode"`
		FingerprintVersion int16  `json:"fingerprint_version"`
		Fingerprint        string `json:"fingerprint"`
	}{identityport.MergePolicyVerifiedUnionIDUniqueWeCom, string(eventport.CustomerMergeAuto), bindReceiptKeyVersion, displayFingerprint})
	if err != nil {
		return identityport.BindResult{}, true, err
	}
	if err = service.contacts.MergeCustomers(ctx, contactport.MergeCustomersCommand{
		PrimaryID: primary,
		MergedID:  merged,
		Actor:     command.Actor,
		Reason:    identityport.MergePolicyVerifiedUnionIDUniqueWeCom,
	}); err != nil {
		return identityport.BindResult{}, true, err
	}
	if err = service.store.RebindIdentitiesForCustomerMerge(ctx, primary, merged); err != nil {
		return identityport.BindResult{}, true, err
	}
	auditID, err := service.store.InsertAutoCustomerMergeAudit(ctx, AutoMergeAudit{
		PrimaryCustomerID:  primary,
		MergedCustomerID:   merged,
		PolicyVersion:      identityport.MergePolicyVerifiedUnionIDUniqueWeCom,
		ReviewFingerprint:  fingerprint,
		FingerprintVersion: bindReceiptKeyVersion,
		Actor:              command.Actor,
		Detail:             detail,
	})
	if err != nil || auditID <= 0 {
		if err != nil {
			return identityport.BindResult{}, true, err
		}
		return identityport.BindResult{}, true, ErrIdentityBindFailed
	}
	payload, err := json.Marshal(eventport.CustomerMergedPayload{
		PrimaryCustomerID: eventport.CustomerID(primary),
		MergedCustomerID:  eventport.CustomerID(merged),
		MergeAuditID:      auditID,
		Mode:              eventport.CustomerMergeAuto,
		PolicyVersion:     identityport.MergePolicyVerifiedUnionIDUniqueWeCom,
	})
	if err != nil {
		return identityport.BindResult{}, true, err
	}
	if _, err = service.events.Append(ctx, eventport.Event{
		Type:           eventport.EvCustomerMerged,
		CustomerID:     eventport.CustomerID(primary),
		Payload:        payload,
		OccurredAt:     time.Now().UTC(),
		IdempotencyKey: "customer.merged:" + strconv.FormatInt(auditID, 10),
	}); err != nil {
		return identityport.BindResult{}, true, err
	}
	return identityport.BindResult{
		Status:            identityport.BindMerged,
		CustomerID:        command.CustomerID,
		PrimaryCustomerID: primary,
		MergeAuditID:      auditID,
	}, true, nil
}

func (service *BindService) mergeFingerprint(normalized NormalizedIdentity) ([]byte, string) {
	material := string(normalized.Kind) + "\x00" + normalized.Scope + "\x00" + normalized.NormalizedValue
	digest := hmacDigest(service.receiptKey, "identity.bind.merge.audit.v1\x00"+material)
	fingerprint := append([]byte(nil), digest[:16]...)
	return fingerprint, "hmac-sha256-v1:" + base64.RawURLEncoding.EncodeToString(fingerprint)
}

func (service *BindService) mergeReviewFingerprint(normalized NormalizedIdentity) ([]byte, string) {
	material := string(normalized.Kind) + "\x00" + normalized.Scope + "\x00" + normalized.NormalizedValue
	digest := hmacDigest(service.receiptKey, "identity.bind.merge.review.v1\x00"+material)
	fingerprint := append([]byte(nil), digest[:16]...)
	return fingerprint, "hmac-sha256-v1:" + base64.RawURLEncoding.EncodeToString(fingerprint)
}

func (service *BindService) appendMergeReviewCreatedEvent(
	ctx context.Context,
	requestedCustomerID contactport.CustomerID,
	reviewID int64,
	candidates []contactport.CustomerID,
) error {
	if reviewID <= 0 || len(candidates) != 2 || candidates[0] <= 0 || candidates[0] >= candidates[1] {
		return ErrIdentityBindFailed
	}
	payload, err := json.Marshal(struct {
		ReviewID     int64   `json:"review_id"`
		CandidateIDs []int64 `json:"candidate_customer_ids"`
		Policy       string  `json:"policy_version"`
	}{reviewID, []int64{int64(candidates[0]), int64(candidates[1])}, VerifiedPhoneMergeReviewPolicy})
	if err != nil {
		return err
	}
	_, err = service.events.Append(ctx, eventport.Event{
		Type:           "identity.merge_review.created",
		CustomerID:     eventport.CustomerID(requestedCustomerID),
		Payload:        payload,
		OccurredAt:     time.Now().UTC(),
		IdempotencyKey: "identity.merge_review.created:" + strconv.FormatInt(reviewID, 10),
	})
	return err
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
	case identityport.BindMerged:
		return result.CustomerID > 0 && result.PrimaryCustomerID > 0 && result.MergeAuditID > 0 && result.ReviewID == 0
	case identityport.BindManualReview:
		return result.CustomerID == 0 && result.PrimaryCustomerID == 0 && result.MergeAuditID == 0 && result.ReviewID > 0
	case identityport.BindRejected:
		return result.CustomerID == 0 && result.PrimaryCustomerID == 0 && result.MergeAuditID == 0 && result.ReviewID == 0
	default:
		return false
	}
}
