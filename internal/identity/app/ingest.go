package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrIdentityIngestFailed              = errors.New("identity ingest failed")
	ErrIdentityIngestIdempotencyConflict = errors.New("identity ingest idempotency conflict")
)

const IngestAttributionPolicy = "identity_ingest_attribution_v1"

type IngestReceipt struct {
	ID          int64
	Found       bool
	PayloadHMAC []byte
	Result      identityport.IngestResult
}

type PendingIngest struct {
	Status         identityport.IngestStatus
	IdentityIDs    []int64
	EventType      string
	Payload        json.RawMessage
	Source         string
	IdempotencyKey string
	OccurredAt     time.Time
}

type IngestStore interface {
	UpsertNormalized(context.Context, NormalizedIdentity) (int64, bool, error)
	LookupNormalized(context.Context, NormalizedIdentity) (ResolveRecord, error)
	ReserveIngestReceipt(context.Context, []byte, []byte) (IngestReceipt, error)
	InsertPendingIngest(context.Context, PendingIngest) (int64, error)
	CompleteIngestReceipt(context.Context, IngestReceipt, identityport.IngestResult) error
}

type IngestService struct {
	uow        platformport.UnitOfWork
	store      IngestStore
	contacts   contactport.MergePort
	events     eventport.Appender
	receiptKey []byte
}

func NewIngestService(
	uow platformport.UnitOfWork,
	store IngestStore,
	contacts contactport.MergePort,
	events eventport.Appender,
	receiptKey []byte,
) *IngestService {
	return &IngestService{uow: uow, store: store, contacts: contacts, events: events, receiptKey: append([]byte(nil), receiptKey...)}
}

type normalizedIngestRef struct {
	Identity  NormalizedIdentity
	Assurance identityport.Assurance
	Source    string
}

type normalizedIngestCommand struct {
	Refs           []normalizedIngestRef
	EventType      string
	Payload        json.RawMessage
	Source         string
	OccurredAt     time.Time
	IdempotencyKey string
}

func (service *IngestService) Ingest(ctx context.Context, command identityport.IngestCommand) (identityport.IngestResult, error) {
	normalized, err := normalizeIngestCommand(command)
	if err != nil {
		return identityport.IngestResult{}, err
	}
	if service == nil || service.uow == nil || service.store == nil || service.contacts == nil || service.events == nil ||
		len(service.receiptKey) < sha256.Size || ctx == nil {
		return identityport.IngestResult{}, ErrIdentityIngestFailed
	}
	keyDigest, payloadHMAC, err := service.receiptDigests(normalized)
	if err != nil {
		return identityport.IngestResult{}, errors.Join(ErrIdentityIngestFailed, err)
	}

	var result identityport.IngestResult
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, err := service.store.ReserveIngestReceipt(txCtx, keyDigest, payloadHMAC)
		if err != nil {
			return err
		}
		if receipt.Found {
			if !hmac.Equal(receipt.PayloadHMAC, payloadHMAC) {
				return ErrIdentityIngestIdempotencyConflict
			}
			if !validIngestResult(receipt.Result) {
				return ErrIdentityIngestFailed
			}
			result = receipt.Result
			return nil
		}

		identityIDs := make([]int64, 0, len(normalized.Refs))
		roots := make(map[int64]struct{})
		conflict := false
		for _, ref := range normalized.Refs {
			identityID, created, err := service.store.UpsertNormalized(txCtx, ref.Identity)
			if err != nil {
				return err
			}
			if identityID <= 0 {
				return ErrIdentityIngestFailed
			}
			identityIDs = append(identityIDs, identityID)
			if created {
				if err = appendIdentityCreatedEvent(txCtx, service.events, identityID, ref.Identity); err != nil {
					return err
				}
			}
			record, err := service.store.LookupNormalized(txCtx, ref.Identity)
			if err != nil {
				return err
			}
			if record.Conflict {
				conflict = true
			}
			if record.CustomerID > 0 {
				roots[record.CustomerID] = struct{}{}
			}
		}
		sort.Slice(identityIDs, func(i, j int) bool { return identityIDs[i] < identityIDs[j] })

		switch {
		case !conflict && len(roots) == 1:
			var customerID int64
			for customerID = range roots {
			}
			eventID, err := service.contacts.AppendExternalEvent(txCtx, contactport.ExternalEventCommand{
				CustomerID:     contactport.CustomerID(customerID),
				EventType:      normalized.EventType,
				Payload:        normalized.Payload,
				Actor:          contactport.Actor(normalized.Source),
				OccurredAt:     normalized.OccurredAt,
				IdempotencyKey: normalized.IdempotencyKey,
			})
			if err != nil {
				return err
			}
			if eventID <= 0 {
				return ErrIdentityIngestFailed
			}
			if _, err = service.events.Append(txCtx, eventport.Event{
				Type:           normalized.EventType,
				CustomerID:     eventport.CustomerID(customerID),
				Payload:        normalized.Payload,
				OccurredAt:     normalized.OccurredAt,
				IdempotencyKey: "identity.ingest:" + hex.EncodeToString(keyDigest),
			}); err != nil {
				return err
			}
			result = identityport.IngestResult{Status: identityport.IngestAttributed, CustomerID: contactport.CustomerID(customerID), EventID: eventID}
		default:
			status := identityport.IngestPending
			if conflict || len(roots) > 1 {
				status = identityport.IngestConflict
			}
			pendingID, err := service.store.InsertPendingIngest(txCtx, PendingIngest{
				Status: status, IdentityIDs: identityIDs, EventType: normalized.EventType, Payload: normalized.Payload,
				Source: normalized.Source, IdempotencyKey: normalized.IdempotencyKey, OccurredAt: normalized.OccurredAt,
			})
			if err != nil {
				return err
			}
			if pendingID <= 0 {
				return ErrIdentityIngestFailed
			}
			if err = service.appendPendingEvent(txCtx, pendingID, status); err != nil {
				return err
			}
			result = identityport.IngestResult{Status: status, PendingEventID: pendingID}
		}
		return service.store.CompleteIngestReceipt(txCtx, receipt, result)
	})
	if err != nil {
		return identityport.IngestResult{}, errors.Join(ErrIdentityIngestFailed, err)
	}
	return result, nil
}

func normalizeIngestCommand(command identityport.IngestCommand) (normalizedIngestCommand, error) {
	if len(command.Refs) == 0 || !validIngestText(command.EventType, 200) || !validIngestText(command.Source, 200) ||
		!validIngestText(command.IdempotencyKey, 512) || command.OccurredAt.IsZero() {
		return normalizedIngestCommand{}, ErrIdentityIngestFailed
	}
	payload, err := canonicalJSONObject(command.Payload)
	if err != nil {
		return normalizedIngestCommand{}, ErrIdentityIngestFailed
	}
	byKey := make(map[string]normalizedIngestRef, len(command.Refs))
	for _, ref := range command.Refs {
		if !validBindEvidence(ref) || ref.Source != command.Source {
			return normalizedIngestCommand{}, ErrIdentityIngestFailed
		}
		identity, err := Normalize(ref)
		if err != nil {
			return normalizedIngestCommand{}, err
		}
		key := string(identity.Kind) + "\x00" + identity.Scope + "\x00" + identity.NormalizedValue
		next := normalizedIngestRef{Identity: identity, Assurance: ref.Assurance, Source: ref.Source}
		if current, found := byKey[key]; found {
			if current.Assurance != next.Assurance || current.Source != next.Source {
				return normalizedIngestCommand{}, ErrIdentityIngestFailed
			}
			continue
		}
		byKey[key] = next
	}
	refs := make([]normalizedIngestRef, 0, len(byKey))
	for _, ref := range byKey {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		left, right := refs[i].Identity, refs[j].Identity
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		return left.NormalizedValue < right.NormalizedValue
	})
	return normalizedIngestCommand{
		Refs: refs, EventType: command.EventType, Payload: payload, Source: command.Source,
		OccurredAt: command.OccurredAt.UTC().Truncate(time.Microsecond), IdempotencyKey: command.IdempotencyKey,
	}, nil
}

func canonicalJSONObject(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, ErrIdentityIngestFailed
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, ErrIdentityIngestFailed
	}
	if err := canonicalizeJSONNumbers(object); err != nil {
		return nil, ErrIdentityIngestFailed
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

type canonicalJSONNumber string

func (number canonicalJSONNumber) MarshalJSON() ([]byte, error) {
	return []byte(number), nil
}

func canonicalizeJSONNumbers(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if number, ok := child.(json.Number); ok {
				canonical, err := canonicalNumber(string(number))
				if err != nil {
					return err
				}
				typed[key] = canonicalJSONNumber(canonical)
				continue
			}
			if err := canonicalizeJSONNumbers(child); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if number, ok := child.(json.Number); ok {
				canonical, err := canonicalNumber(string(number))
				if err != nil {
					return err
				}
				typed[index] = canonicalJSONNumber(canonical)
				continue
			}
			if err := canonicalizeJSONNumbers(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func canonicalNumber(value string) (string, error) {
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	exponent := int64(0)
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		parsed, err := strconv.ParseInt(value[index+1:], 10, 32)
		if err != nil {
			return "", err
		}
		exponent = parsed
		value = value[:index]
	}
	integer, fraction := value, ""
	if index := strings.IndexByte(value, '.'); index >= 0 {
		integer, fraction = value[:index], value[index+1:]
	}
	exponent -= int64(len(fraction))
	digits := strings.TrimLeft(integer+fraction, "0")
	if digits == "" {
		return "0", nil
	}
	trimmed := strings.TrimRight(digits, "0")
	exponent += int64(len(digits) - len(trimmed))
	digits = trimmed
	exponent += int64(len(digits) - 1)
	mantissa := digits[:1]
	if len(digits) > 1 {
		mantissa += "." + digits[1:]
	}
	if negative {
		mantissa = "-" + mantissa
	}
	if exponent != 0 {
		mantissa += "e" + strconv.FormatInt(exponent, 10)
	}
	return mantissa, nil
}

func (service *IngestService) receiptDigests(command normalizedIngestCommand) ([]byte, []byte, error) {
	type receiptRef struct {
		Kind      identityport.IDKind    `json:"kind"`
		Scope     string                 `json:"scope"`
		Value     string                 `json:"normalized_value"`
		Assurance identityport.Assurance `json:"assurance"`
		Source    string                 `json:"source"`
	}
	refs := make([]receiptRef, 0, len(command.Refs))
	for _, ref := range command.Refs {
		refs = append(refs, receiptRef{ref.Identity.Kind, ref.Identity.Scope, ref.Identity.NormalizedValue, ref.Assurance, ref.Source})
	}
	payload, err := json.Marshal(struct {
		Refs       []receiptRef     `json:"refs"`
		EventType  string           `json:"event_type"`
		Payload    *json.RawMessage `json:"payload"`
		Source     string           `json:"source"`
		OccurredAt time.Time        `json:"occurred_at"`
	}{refs, command.EventType, &command.Payload, command.Source, command.OccurredAt})
	if err != nil {
		return nil, nil, err
	}
	return hmacDigest(service.receiptKey, "identity.ingest.key.v1\x00"+command.IdempotencyKey),
		hmacDigest(service.receiptKey, "identity.ingest.payload.v1\x00"+string(payload)), nil
}

func (service *IngestService) appendPendingEvent(ctx context.Context, pendingID int64, status identityport.IngestStatus) error {
	payload, err := json.Marshal(struct {
		PendingEventID int64                     `json:"pending_event_id"`
		Status         identityport.IngestStatus `json:"status"`
		PolicyVersion  string                    `json:"policy_version"`
	}{pendingID, status, IngestAttributionPolicy})
	if err != nil {
		return err
	}
	_, err = service.events.Append(ctx, eventport.Event{
		Type: "identity.ingest." + string(status), Payload: payload, OccurredAt: time.Now().UTC(),
		IdempotencyKey: "identity.ingest." + string(status) + ":" + strconv.FormatInt(pendingID, 10),
	})
	return err
}

func appendIdentityCreatedEvent(ctx context.Context, events eventport.Appender, identityID int64, normalized NormalizedIdentity) error {
	payload, err := json.Marshal(struct {
		IdentityID        int64               `json:"identity_id"`
		Kind              identityport.IDKind `json:"kind"`
		Scope             string              `json:"scope"`
		NormalizerVersion int16               `json:"normalizer_version"`
	}{identityID, normalized.Kind, normalized.Scope, normalized.NormalizerVersion})
	if err != nil {
		return err
	}
	_, err = events.Append(ctx, eventport.Event{
		Type: "identity.created", Payload: payload, OccurredAt: time.Now().UTC(),
		IdempotencyKey: "identity.created:" + strconv.FormatInt(identityID, 10),
	})
	return err
}

func validIngestResult(result identityport.IngestResult) bool {
	switch result.Status {
	case identityport.IngestAttributed:
		return result.CustomerID > 0 && result.EventID > 0 && result.PendingEventID == 0
	case identityport.IngestPending, identityport.IngestConflict:
		return result.CustomerID == 0 && result.EventID == 0 && result.PendingEventID > 0
	default:
		return false
	}
}

func validIngestText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value) && !containsControl(value)
}
