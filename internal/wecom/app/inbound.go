package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	wecomcallback "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/callback"
)

const (
	InboundContactJobKind = "wecom_contact_inbound"
	InboundLease          = 2 * time.Minute
)

var (
	ErrInvalidInboundService = errors.New("invalid WeCom inbound service")
	ErrInvalidInboundMessage = errors.New("invalid WeCom inbound message")
	ErrUnsupportedInbound    = errors.New("unsupported WeCom inbound event")
	ErrInboundAlreadyDone    = errors.New("WeCom inbound fact is already complete or leased")
	ErrInboundProcess        = errors.New("WeCom inbound processing failed")
)

// InboundJobArgs is the only River payload emitted by this package. The job
// contains no provider request and therefore cannot itself produce an
// external effect.
type InboundJobArgs struct {
	InboxID int64 `json:"inbox_id"`
}

func (InboundJobArgs) Kind() string { return InboundContactJobKind }

type InboundSource string

const (
	InboundSourceCallback InboundSource = "callback_inbox"
	InboundSourceSync     InboundSource = "directory_sync"
)

type InboundEnvelope struct {
	Source         InboundSource
	SourceKey      string
	CorpID         string
	EventType      string
	ExternalUserID string
	// ExternalContact is present only for change_external_contact callbacks.
	// Its sensitive fields are digest-only; persistence of the typed fields is
	// deliberately owned by the CH03 integration migration.
	ExternalContact *ExternalContactCallbackFact
	RawPayload      []byte
	OccurredAt      time.Time
	InitialState    string
}

type InboundReservation struct {
	ID       int64
	Inserted bool
	State    string
}

type InboundRecord struct {
	ID             int64
	Source         InboundSource
	SourceKey      string
	CorpID         string
	EventType      string
	ExternalUserID string
	RawPayload     []byte
	OccurredAt     time.Time
	State          string
	AttemptCount   int32
	LeaseFence     int64
	LeaseOwner     string
	RiverJobID     int64
}

type InboundStore interface {
	ReserveInbound(context.Context, InboundEnvelope) (InboundReservation, error)
	MarkInboundQueued(context.Context, int64, int64) error
	ClaimInbound(context.Context, int64, string, time.Time) (InboundRecord, error)
	CompleteInbound(context.Context, int64, int64, string) error
	FailInbound(context.Context, int64, int64, string) error
}

// JobInserter is deliberately transaction-context based. Implementations
// insert into River using the transaction already opened by UnitOfWork.
type JobInserter interface {
	Insert(context.Context, InboundJobArgs) (int64, error)
}

type inboundUnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}

type InboundService struct {
	uow       inboundUnitOfWork
	store     InboundStore
	jobs      JobInserter
	processor *IdentityContactProcessor
	corpID    string
	clock     func() time.Time
}

func NewInboundService(
	uow inboundUnitOfWork,
	store InboundStore,
	jobs JobInserter,
	processor *IdentityContactProcessor,
	corpID string,
	clock func() time.Time,
) (*InboundService, error) {
	if isNilDependency(uow) || isNilDependency(store) || isNilDependency(jobs) ||
		!validCorpID(corpID) || clock == nil {
		return nil, ErrInvalidInboundService
	}
	return &InboundService{uow: uow, store: store, jobs: jobs, processor: processor, corpID: corpID, clock: clock}, nil
}

// Dispatch is the callback.MessageDispatcher implementation. It only
// validates/de-duplicates and queues a local fact; Identity processing is
// always deferred to the critical River worker.
func (service *InboundService) Dispatch(ctx context.Context, message []byte) error {
	if service == nil || ctx == nil || isNilDependency(service.uow) || isNilDependency(service.store) ||
		isNilDependency(service.jobs) || !validCorpID(service.corpID) || service.clock == nil {
		return ErrInvalidInboundService
	}
	envelope, err := ParseCallbackEnvelope(message, service.corpID)
	if err != nil {
		return err
	}
	return service.accept(ctx, envelope)
}

func (service *InboundService) accept(ctx context.Context, envelope InboundEnvelope) error {
	return service.uow.Within(ctx, func(txCtx context.Context) error {
		reservation, err := service.store.ReserveInbound(txCtx, envelope)
		if err != nil || !reservation.Inserted || envelope.InitialState == "skipped" {
			return err
		}
		jobID, err := service.jobs.Insert(txCtx, InboundJobArgs{InboxID: reservation.ID})
		if err != nil || jobID <= 0 {
			return errors.Join(ErrInboundProcess, err)
		}
		return service.store.MarkInboundQueued(txCtx, reservation.ID, jobID)
	})
}

// Process claims one local inbox fact and invokes only the existing Identity
// port. Attributed, pending, and conflict are all terminal local outcomes;
// transaction failures are returned to River for retry.
func (service *InboundService) Process(ctx context.Context, inboxID int64, owner string) error {
	if service == nil || ctx == nil || inboxID <= 0 || strings.TrimSpace(owner) != owner || owner == "" ||
		isNilDependency(service.uow) || isNilDependency(service.store) || service.processor == nil || service.clock == nil {
		return ErrInvalidInboundService
	}
	var record InboundRecord
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		claimed, claimErr := service.store.ClaimInbound(txCtx, inboxID, owner, service.clock().UTC().Add(InboundLease))
		if claimErr != nil {
			return claimErr
		}
		record = claimed
		return nil
	})
	if errors.Is(err, ErrInboundAlreadyDone) {
		return nil
	}
	if err != nil {
		return err
	}
	if record.State == "skipped" || record.ExternalUserID == "" {
		return service.complete(ctx, record, "skipped")
	}
	result, processErr := service.processor.Process(ctx, IdentityContactFact{
		Source:         identityContactSource(record.Source),
		FactID:         record.SourceKey,
		CorpID:         record.CorpID,
		ExternalUserID: record.ExternalUserID,
		OccurredAt:     record.OccurredAt,
	})
	if processErr != nil {
		_ = service.uow.Within(ctx, func(txCtx context.Context) error {
			return service.store.FailInbound(txCtx, record.ID, record.LeaseFence, processErr.Error())
		})
		return errors.Join(ErrInboundProcess, processErr)
	}
	state := "processed"
	if result.Status == identityport.IngestPending {
		state = "pending_identity"
	} else if result.Status == identityport.IngestConflict {
		state = "conflict"
	}
	return service.complete(ctx, record, state)
}

func (service *InboundService) complete(ctx context.Context, record InboundRecord, state string) error {
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		return service.store.CompleteInbound(txCtx, record.ID, record.LeaseFence, state)
	})
	if err != nil {
		return errors.Join(ErrInboundProcess, err)
	}
	return nil
}

func identityContactSource(source InboundSource) IdentityContactFactSource {
	if source == InboundSourceSync {
		return IdentityContactFromSync
	}
	return IdentityContactFromCallback
}

// ParseCallbackEnvelope parses the already-authenticated/decrypted WeCom XML.
// It intentionally supports only the external-contact events in this package
// plus enter_agent, which is retained as a durable skipped callback fact.
func ParseCallbackEnvelope(message []byte, corpID string) (InboundEnvelope, error) {
	if len(message) == 0 || len(message) > 1<<20 || !utf8.Valid(message) || !validCorpID(corpID) {
		return InboundEnvelope{}, ErrInvalidInboundMessage
	}
	var payload struct {
		XMLName         xml.Name `xml:"xml"`
		ToUserName      string   `xml:"ToUserName"`
		CreateTime      int64    `xml:"CreateTime"`
		MsgType         string   `xml:"MsgType"`
		Event           string   `xml:"Event"`
		ChangeType      string   `xml:"ChangeType"`
		ExternalUserID  string   `xml:"ExternalUserID"`
		ExternalUserID2 string   `xml:"ExternalUserId"`
	}
	decoder := xml.NewDecoder(strings.NewReader(string(message)))
	trailing := error(nil)
	if err := decoder.Decode(&payload); err != nil {
		return InboundEnvelope{}, errors.Join(ErrInvalidInboundMessage, wecomcallback.ErrUnknownCallbackEvent)
	} else {
		trailing = decoder.Decode(&struct{}{})
	}
	if trailing != io.EOF || payload.XMLName.Local != "xml" ||
		payload.ToUserName != corpID || payload.MsgType != "event" || payload.CreateTime <= 0 {
		return InboundEnvelope{}, errors.Join(ErrInvalidInboundMessage, wecomcallback.ErrUnknownCallbackEvent)
	}
	externalUserID := payload.ExternalUserID
	if externalUserID == "" {
		externalUserID = payload.ExternalUserID2
	}
	if payload.Event != "enter_agent" && payload.Event != "change_external_contact" {
		return InboundEnvelope{}, errors.Join(ErrUnsupportedInbound, wecomcallback.ErrUnknownCallbackEvent)
	}
	if payload.Event == externalContactEvent {
		fact, err := ParseExternalContactCallbackFact(message, corpID)
		if err != nil {
			return InboundEnvelope{}, err
		}
		digest := sha256.Sum256(message)
		initialState := "skipped"
		if fact.IsEntrant() {
			initialState = "pending"
		}
		return InboundEnvelope{
			Source: InboundSourceCallback, SourceKey: "sha256:" + hex.EncodeToString(digest[:]), CorpID: corpID,
			EventType: fact.EventType(), ExternalUserID: fact.ExternalUserID, ExternalContact: &fact,
			RawPayload: redactCallbackSecrets(message), OccurredAt: fact.OccurredAt, InitialState: initialState,
		}, nil
	}
	eventType := payload.Event
	if payload.ChangeType != "" {
		eventType += ":" + payload.ChangeType
	}
	digest := sha256.Sum256(message)
	initialState := "pending"
	if payload.Event == "enter_agent" {
		initialState = "skipped"
	}
	return InboundEnvelope{
		Source: InboundSourceCallback, SourceKey: "sha256:" + hex.EncodeToString(digest[:]), CorpID: corpID,
		EventType: eventType, ExternalUserID: externalUserID, RawPayload: append([]byte(nil), message...),
		OccurredAt: time.Unix(payload.CreateTime, 0).UTC(), InitialState: initialState,
	}, nil
}

// redactCallbackSecrets preserves the callback's non-secret audit payload
// while ensuring secret lifecycle values cannot reach durable storage through
// the legacy RawPayload column. Typed parsing above has already retained only
// their digests. The encoder also rejects malformed XML before this function
// runs.
func redactCallbackSecrets(message []byte) []byte {
	decoder := xml.NewDecoder(strings.NewReader(string(message)))
	var builder strings.Builder
	encoder := xml.NewEncoder(&builder)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}
		start, isStart := token.(xml.StartElement)
		if !isStart || !callbackSecretElement(start.Name.Local) {
			if encoder.EncodeToken(token) != nil {
				return nil
			}
			continue
		}
		if encoder.EncodeToken(start) != nil || encoder.EncodeToken(xml.CharData("[redacted]")) != nil {
			return nil
		}
		depth := 1
		for depth > 0 {
			next, nextErr := decoder.Token()
			if nextErr != nil {
				return nil
			}
			switch next.(type) {
			case xml.StartElement:
				depth++
			case xml.EndElement:
				depth--
			}
		}
		if encoder.EncodeToken(xml.EndElement{Name: start.Name}) != nil {
			return nil
		}
	}
	if encoder.Flush() != nil {
		return nil
	}
	return []byte(builder.String())
}

func callbackSecretElement(name string) bool {
	return name == "WelcomeCode" || name == "Source" || name == "FailReason"
}

func validCorpID(value string) bool {
	return validText(value, 256) && !strings.ContainsAny(value, " \t\r\n")
}

func validText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value && utf8.ValidString(value)
}

var _ interface {
	Dispatch(context.Context, []byte) error
} = (*InboundService)(nil)
