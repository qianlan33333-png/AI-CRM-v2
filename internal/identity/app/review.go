package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	MergeReviewDefaultLimit = int32(50)
	MergeReviewMaximumLimit = int32(100)
	mergeReviewCursorLimit  = 512
	mergeReviewCommandLimit = 512
	mergeReviewReasonLimit  = 500
	mergeReviewCursorV2     = 2
)

var (
	ErrMergeReviewInvalid     = errors.New("invalid identity merge review command")
	ErrMergeReviewNotFound    = errors.New("identity merge review not found")
	ErrMergeReviewConflict    = errors.New("identity merge review conflict")
	ErrMergeReviewUnavailable = errors.New("identity merge review unavailable")
)

type MergeReviewRecord struct {
	ReviewID            int64
	Status              identityport.MergeReviewStatus
	Kind                identityport.IDKind
	Scope               string
	NormalizedValue     string
	IdentityID          int64
	IdentityFingerprint []byte
	FingerprintVersion  int16
	CustomerIDs         []contactport.CustomerID
	PolicyVersion       string
	Version             int64
	CreatedAt           time.Time
	ResolvedAt          *time.Time
}

// MergeReviewHistoryRecord contains only fields required by the closed read DTO.
// It deliberately excludes normalized identity values, provider identifiers,
// raw payloads and policy details. It retains only the versioned secret-backed
// HMAC required by the frozen administrator contract.
type MergeReviewHistoryRecord struct {
	ReviewID            int64
	Status              identityport.MergeReviewStatus
	Kind                identityport.IDKind
	Scope               string
	CustomerIDs         []contactport.CustomerID
	IdentityFingerprint []byte
	FingerprintVersion  int16
	Version             int64
	CreatedAt           time.Time
	ResolvedAt          *time.Time
}

type MergeReviewReceipt struct {
	ID          int64
	Found       bool
	PayloadHMAC []byte
	ReviewID    int64
	Status      identityport.MergeReviewStatus
}

type ManualMergeAudit struct {
	PrimaryCustomerID  contactport.CustomerID
	MergedCustomerID   contactport.CustomerID
	PolicyVersion      string
	ReviewFingerprint  []byte
	FingerprintVersion int16
	Actor              contactport.Actor
	Detail             json.RawMessage
}

type MergeReviewStore interface {
	ListMergeReviewsByStatus(context.Context, identityport.MergeReviewStatus, int64, int32) ([]MergeReviewHistoryRecord, error)
	ReserveMergeReviewReceipt(context.Context, string, []byte, []byte) (MergeReviewReceipt, error)
	LockMergeReview(context.Context, int64) (MergeReviewRecord, error)
	LockActiveMergeReviewCustomers(context.Context, []contactport.CustomerID) ([]contactport.CustomerID, error)
	RebindIdentitiesForCustomerMerge(context.Context, contactport.CustomerID, contactport.CustomerID) error
	InsertManualCustomerMergeAudit(context.Context, ManualMergeAudit) (int64, error)
	ResolveMergeReview(context.Context, int64, int64, identityport.MergeReviewStatus) (MergeReviewRecord, error)
	CompleteMergeReviewReceipt(context.Context, MergeReviewReceipt, MergeReviewRecord, int64) error
}

type MergeReviewService struct {
	uow        platformport.UnitOfWork
	store      MergeReviewStore
	contacts   contactport.MergePort
	events     eventport.Appender
	receiptKey []byte
}

func NewMergeReviewService(
	uow platformport.UnitOfWork,
	store MergeReviewStore,
	contacts contactport.MergePort,
	events eventport.Appender,
	receiptKey []byte,
) *MergeReviewService {
	return &MergeReviewService{
		uow: uow, store: store, contacts: contacts, events: events,
		receiptKey: append([]byte(nil), receiptKey...),
	}
}

func (service *MergeReviewService) ListMergeReviews(
	ctx context.Context,
	cursor string,
	limit int32,
) (identityport.MergeReviewPage, error) {
	return service.ListMergeReviewsByStatus(ctx, identityport.MergeReviewPending, cursor, limit)
}

func (service *MergeReviewService) ListMergeReviewsByStatus(
	ctx context.Context,
	status identityport.MergeReviewStatus,
	cursor string,
	limit int32,
) (identityport.MergeReviewPage, error) {
	if !service.ready(ctx) {
		return identityport.MergeReviewPage{}, ErrMergeReviewUnavailable
	}
	if !status.Valid() {
		return identityport.MergeReviewPage{}, ErrMergeReviewInvalid
	}
	if limit == 0 {
		limit = MergeReviewDefaultLimit
	}
	if limit < 1 || limit > MergeReviewMaximumLimit || len(cursor) > mergeReviewCursorLimit {
		return identityport.MergeReviewPage{}, ErrMergeReviewInvalid
	}
	afterID, err := decodeMergeReviewCursor(status, cursor)
	if err != nil {
		return identityport.MergeReviewPage{}, errors.Join(ErrMergeReviewInvalid, err)
	}
	var records []MergeReviewHistoryRecord
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		records, storeErr = service.store.ListMergeReviewsByStatus(txCtx, status, afterID, limit+1)
		return storeErr
	})
	if err != nil {
		return identityport.MergeReviewPage{}, errors.Join(ErrMergeReviewUnavailable, err)
	}
	if len(records) > int(limit)+1 {
		return identityport.MergeReviewPage{}, ErrMergeReviewUnavailable
	}
	hasMore := len(records) > int(limit)
	if hasMore {
		records = records[:limit]
	}
	page := identityport.MergeReviewPage{Items: make([]identityport.MergeReview, 0, len(records))}
	for index, record := range records {
		if index > 0 && records[index-1].ReviewID >= record.ReviewID {
			return identityport.MergeReviewPage{}, ErrMergeReviewUnavailable
		}
		converted, convertErr := publicMergeReviewHistory(record)
		if convertErr != nil || converted.Status != status {
			return identityport.MergeReviewPage{}, ErrMergeReviewUnavailable
		}
		page.Items = append(page.Items, converted)
	}
	if hasMore {
		page.NextCursor, err = encodeMergeReviewCursor(status, records[len(records)-1].ReviewID)
		if err != nil {
			return identityport.MergeReviewPage{}, errors.Join(ErrMergeReviewUnavailable, err)
		}
	}
	return page, nil
}

func (service *MergeReviewService) ApproveMergeReview(
	ctx context.Context,
	command identityport.ApproveMergeReviewCommand,
) (identityport.MergeReview, error) {
	if !validApproveMergeReview(command) {
		return identityport.MergeReview{}, ErrMergeReviewInvalid
	}
	return service.resolve(ctx, reviewDecision{
		reviewID: command.ReviewID, expectedVersion: command.ExpectedVersion,
		primaryCustomerID: command.PrimaryCustomerID, reason: command.Reason,
		actor: command.Actor, idempotencyKey: command.IdempotencyKey,
		status: identityport.MergeReviewApproved,
	})
}

func (service *MergeReviewService) RejectMergeReview(
	ctx context.Context,
	command identityport.RejectMergeReviewCommand,
) (identityport.MergeReview, error) {
	if !validRejectMergeReview(command) {
		return identityport.MergeReview{}, ErrMergeReviewInvalid
	}
	return service.resolve(ctx, reviewDecision{
		reviewID: command.ReviewID, expectedVersion: command.ExpectedVersion,
		reason: command.Reason, actor: command.Actor, idempotencyKey: command.IdempotencyKey,
		status: identityport.MergeReviewRejected,
	})
}

type reviewDecision struct {
	reviewID, expectedVersion int64
	primaryCustomerID         contactport.CustomerID
	reason                    string
	actor                     contactport.Actor
	idempotencyKey            string
	status                    identityport.MergeReviewStatus
}

func (service *MergeReviewService) resolve(ctx context.Context, decision reviewDecision) (identityport.MergeReview, error) {
	if !service.ready(ctx) || service.contacts == nil || service.events == nil {
		return identityport.MergeReview{}, ErrMergeReviewUnavailable
	}
	operation := "merge_review_reject"
	if decision.status == identityport.MergeReviewApproved {
		operation = "merge_review_approve"
	}
	keyDigest, payloadHMAC, err := service.reviewReceiptDigests(operation, decision)
	if err != nil {
		return identityport.MergeReview{}, errors.Join(ErrMergeReviewUnavailable, err)
	}
	var result identityport.MergeReview
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, reserveErr := service.store.ReserveMergeReviewReceipt(txCtx, operation, keyDigest, payloadHMAC)
		if reserveErr != nil {
			return reserveErr
		}
		if receipt.Found {
			if !hmac.Equal(receipt.PayloadHMAC, payloadHMAC) || receipt.ReviewID != decision.reviewID || receipt.Status != decision.status {
				return ErrMergeReviewConflict
			}
			record, loadErr := service.store.LockMergeReview(txCtx, receipt.ReviewID)
			if loadErr != nil {
				return loadErr
			}
			result, loadErr = publicMergeReview(record)
			return loadErr
		}

		record, lockErr := service.store.LockMergeReview(txCtx, decision.reviewID)
		if lockErr != nil {
			return lockErr
		}
		if record.Status != identityport.MergeReviewPending || record.Version != decision.expectedVersion ||
			!validReviewEvidence(service.receiptKey, record) {
			return ErrMergeReviewConflict
		}
		roots, lockErr := service.store.LockActiveMergeReviewCustomers(txCtx, record.CustomerIDs)
		if lockErr != nil {
			return lockErr
		}
		if !sameCustomerIDs(roots, record.CustomerIDs) {
			return ErrMergeReviewConflict
		}

		mergeAuditID := int64(0)
		if decision.status == identityport.MergeReviewApproved {
			if !containsCustomerID(roots, decision.primaryCustomerID) {
				return ErrMergeReviewConflict
			}
			mergedCustomerID := roots[0]
			if mergedCustomerID == decision.primaryCustomerID {
				mergedCustomerID = roots[1]
			}
			if mergeErr := service.contacts.MergeCustomers(txCtx, contactport.MergeCustomersCommand{
				PrimaryID: decision.primaryCustomerID, MergedID: mergedCustomerID,
				Actor: decision.actor, Reason: decision.reason,
			}); mergeErr != nil {
				return mergeErr
			}
			if mergeErr := service.store.RebindIdentitiesForCustomerMerge(txCtx, decision.primaryCustomerID, mergedCustomerID); mergeErr != nil {
				return mergeErr
			}
			detail, detailErr := mergeReviewAuditDetail(record)
			if detailErr != nil {
				return detailErr
			}
			mergeAuditID, lockErr = service.store.InsertManualCustomerMergeAudit(txCtx, ManualMergeAudit{
				PrimaryCustomerID: decision.primaryCustomerID, MergedCustomerID: mergedCustomerID,
				PolicyVersion: record.PolicyVersion, ReviewFingerprint: record.IdentityFingerprint,
				FingerprintVersion: record.FingerprintVersion, Actor: decision.actor, Detail: detail,
			})
			if lockErr != nil || mergeAuditID <= 0 {
				if lockErr != nil {
					return lockErr
				}
				return ErrMergeReviewUnavailable
			}
			if eventErr := service.appendManualMergeEvent(txCtx, decision.primaryCustomerID, mergedCustomerID, mergeAuditID, record.PolicyVersion); eventErr != nil {
				return eventErr
			}
		}

		resolved, resolveErr := service.store.ResolveMergeReview(txCtx, decision.reviewID, decision.expectedVersion, decision.status)
		if resolveErr != nil {
			return resolveErr
		}
		if completeErr := service.store.CompleteMergeReviewReceipt(txCtx, receipt, resolved, mergeAuditID); completeErr != nil {
			return completeErr
		}
		result, resolveErr = publicMergeReview(resolved)
		return resolveErr
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrMergeReviewNotFound):
			return identityport.MergeReview{}, errors.Join(ErrMergeReviewNotFound, err)
		case errors.Is(err, ErrMergeReviewConflict), errors.Is(err, contactport.ErrMergeConflict), errors.Is(err, contactport.ErrMergeCustomerNotFound):
			return identityport.MergeReview{}, errors.Join(ErrMergeReviewConflict, err)
		default:
			return identityport.MergeReview{}, errors.Join(ErrMergeReviewUnavailable, err)
		}
	}
	return result, nil
}

func (service *MergeReviewService) ready(ctx context.Context) bool {
	return service != nil && ctx != nil && service.uow != nil && service.store != nil && len(service.receiptKey) >= 32
}

func (service *MergeReviewService) reviewReceiptDigests(operation string, decision reviewDecision) ([]byte, []byte, error) {
	payload, err := json.Marshal(struct {
		ReviewID          int64  `json:"review_id"`
		ExpectedVersion   int64  `json:"expected_version"`
		PrimaryCustomerID int64  `json:"primary_customer_id,omitempty"`
		Reason            string `json:"reason"`
		Actor             string `json:"actor"`
	}{decision.reviewID, decision.expectedVersion, int64(decision.primaryCustomerID), decision.reason, string(decision.actor)})
	if err != nil {
		return nil, nil, err
	}
	return hmacDigest(service.receiptKey, "identity."+operation+".key.v1\x00"+decision.idempotencyKey),
		hmacDigest(service.receiptKey, "identity."+operation+".payload.v1\x00"+string(payload)), nil
}

func (service *MergeReviewService) appendManualMergeEvent(
	ctx context.Context,
	primary, merged contactport.CustomerID,
	auditID int64,
	policy string,
) error {
	payload, err := json.Marshal(eventport.CustomerMergedPayload{
		PrimaryCustomerID: eventport.CustomerID(primary), MergedCustomerID: eventport.CustomerID(merged),
		MergeAuditID: auditID, Mode: eventport.CustomerMergeManual, PolicyVersion: policy,
	})
	if err != nil {
		return err
	}
	_, err = service.events.Append(ctx, eventport.Event{
		Type: "customer.merged", CustomerID: eventport.CustomerID(primary), Payload: payload,
		OccurredAt: time.Now().UTC(), IdempotencyKey: "customer.merged:" + strconv.FormatInt(auditID, 10),
	})
	return err
}

func mergeReviewAuditDetail(record MergeReviewRecord) (json.RawMessage, error) {
	fingerprint, err := mergeReviewFingerprint(record)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		PolicyVersion      string `json:"policy_version"`
		Mode               string `json:"mode"`
		FingerprintVersion int16  `json:"fingerprint_version"`
		Fingerprint        string `json:"fingerprint"`
	}{record.PolicyVersion, string(eventport.CustomerMergeManual), record.FingerprintVersion, fingerprint})
}

func publicMergeReview(record MergeReviewRecord) (identityport.MergeReview, error) {
	if !validMergeReviewRecord(record) {
		return identityport.MergeReview{}, ErrMergeReviewUnavailable
	}
	return publicMergeReviewHistory(MergeReviewHistoryRecord{
		ReviewID: record.ReviewID, Status: record.Status, Kind: record.Kind, Scope: record.Scope,
		CustomerIDs: record.CustomerIDs, IdentityFingerprint: record.IdentityFingerprint,
		FingerprintVersion: record.FingerprintVersion, Version: record.Version, CreatedAt: record.CreatedAt,
		ResolvedAt: record.ResolvedAt,
	})
}

func publicMergeReviewHistory(record MergeReviewHistoryRecord) (identityport.MergeReview, error) {
	if !validMergeReviewHistoryRecord(record) {
		return identityport.MergeReview{}, ErrMergeReviewUnavailable
	}
	fingerprint, err := formatMergeReviewFingerprint(record.IdentityFingerprint, record.FingerprintVersion)
	if err != nil {
		return identityport.MergeReview{}, err
	}
	return identityport.MergeReview{
		ReviewID: record.ReviewID, Status: record.Status, Kind: record.Kind, Scope: record.Scope,
		CustomerIDs:         append([]contactport.CustomerID(nil), record.CustomerIDs...),
		IdentityFingerprint: fingerprint,
		Version:             record.Version, CreatedAt: record.CreatedAt.UTC(), ResolvedAt: cloneTime(record.ResolvedAt),
	}, nil
}

func validReviewEvidence(key []byte, record MergeReviewRecord) bool {
	if !validMergeReviewRecord(record) || record.Status != identityport.MergeReviewPending || len(key) < 32 {
		return false
	}
	material := string(record.Kind) + "\x00" + record.Scope + "\x00" + record.NormalizedValue
	digest := hmacDigest(key, "identity.bind.merge.review.v1\x00"+material)
	return hmac.Equal(record.IdentityFingerprint, digest[:16])
}

func validMergeReviewRecord(record MergeReviewRecord) bool {
	if record.IdentityID <= 0 || strings.TrimSpace(record.NormalizedValue) == "" ||
		strings.TrimSpace(record.PolicyVersion) == "" || len(record.IdentityFingerprint) != 16 ||
		record.FingerprintVersion <= 0 {
		return false
	}
	return validMergeReviewHistoryRecord(MergeReviewHistoryRecord{
		ReviewID: record.ReviewID, Status: record.Status, Kind: record.Kind, Scope: record.Scope,
		CustomerIDs: record.CustomerIDs, IdentityFingerprint: record.IdentityFingerprint,
		FingerprintVersion: record.FingerprintVersion, Version: record.Version, CreatedAt: record.CreatedAt,
		ResolvedAt: record.ResolvedAt,
	})
}

func validMergeReviewHistoryRecord(record MergeReviewHistoryRecord) bool {
	if record.ReviewID <= 0 || record.Version <= 0 || record.CreatedAt.IsZero() ||
		(record.Kind != identityport.KindPhone && record.Kind != identityport.KindUnionID) ||
		strings.TrimSpace(record.Scope) == "" || len(record.IdentityFingerprint) != 16 ||
		record.FingerprintVersion <= 0 || len(record.CustomerIDs) != 2 ||
		record.CustomerIDs[0] <= 0 || record.CustomerIDs[0] >= record.CustomerIDs[1] {
		return false
	}
	switch record.Status {
	case identityport.MergeReviewPending:
		return record.ResolvedAt == nil
	case identityport.MergeReviewApproved, identityport.MergeReviewRejected:
		return record.ResolvedAt != nil && !record.ResolvedAt.IsZero() && !record.ResolvedAt.Before(record.CreatedAt)
	default:
		return false
	}
}

func mergeReviewFingerprint(record MergeReviewRecord) (string, error) {
	return formatMergeReviewFingerprint(record.IdentityFingerprint, record.FingerprintVersion)
}

func formatMergeReviewFingerprint(fingerprint []byte, version int16) (string, error) {
	if len(fingerprint) != 16 || version <= 0 {
		return "", ErrMergeReviewUnavailable
	}
	return fmt.Sprintf("hmac-sha256-v%d:%s", version, base64.RawURLEncoding.EncodeToString(fingerprint)), nil
}

func validApproveMergeReview(command identityport.ApproveMergeReviewCommand) bool {
	return command.ReviewID > 0 && command.ExpectedVersion > 0 && command.PrimaryCustomerID > 0 &&
		validReviewText(command.Reason, mergeReviewReasonLimit) && validReviewText(string(command.Actor), 200) &&
		validReviewText(command.IdempotencyKey, mergeReviewCommandLimit)
}

func validRejectMergeReview(command identityport.RejectMergeReviewCommand) bool {
	return command.ReviewID > 0 && command.ExpectedVersion > 0 &&
		validReviewText(command.Reason, mergeReviewReasonLimit) && validReviewText(string(command.Actor), 200) &&
		validReviewText(command.IdempotencyKey, mergeReviewCommandLimit)
}

func validReviewText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maximum && utf8.ValidString(value) && !containsControl(value)
}

func sameCustomerIDs(left, right []contactport.CustomerID) bool {
	return len(left) == 2 && len(right) == 2 && left[0] == right[0] && left[1] == right[1]
}

func containsCustomerID(ids []contactport.CustomerID, id contactport.CustomerID) bool {
	return len(ids) == 2 && (ids[0] == id || ids[1] == id)
}

type mergeReviewCursor struct {
	Version   int                            `json:"v"`
	Operation string                         `json:"operation"`
	Status    identityport.MergeReviewStatus `json:"status"`
	Sort      string                         `json:"sort"`
	AfterID   int64                          `json:"after_id"`
}

func encodeMergeReviewCursor(status identityport.MergeReviewStatus, afterID int64) (string, error) {
	if !status.Valid() || afterID <= 0 {
		return "", ErrMergeReviewInvalid
	}
	encoded, err := json.Marshal(mergeReviewCursor{
		Version: mergeReviewCursorV2, Operation: "listIdentityMergeReviews", Status: status, Sort: "id_asc", AfterID: afterID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeMergeReviewCursor(status identityport.MergeReviewStatus, raw string) (int64, error) {
	if !status.Valid() {
		return 0, ErrMergeReviewInvalid
	}
	if raw == "" {
		return 0, nil
	}
	if len(raw) > mergeReviewCursorLimit || strings.Contains(raw, "=") {
		return 0, ErrMergeReviewInvalid
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return 0, ErrMergeReviewInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var cursor mergeReviewCursor
	if err = decoder.Decode(&cursor); err != nil {
		return 0, ErrMergeReviewInvalid
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return 0, ErrMergeReviewInvalid
	}
	if cursor.Version != mergeReviewCursorV2 || cursor.Operation != "listIdentityMergeReviews" ||
		cursor.Status != status || cursor.Sort != "id_asc" || cursor.AfterID <= 0 {
		return 0, ErrMergeReviewInvalid
	}
	return cursor.AfterID, nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
