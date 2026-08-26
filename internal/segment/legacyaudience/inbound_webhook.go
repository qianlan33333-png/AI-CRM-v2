package legacyaudience

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	AIAudienceWebhookClientID = "aicrm-webhook-ai-audience"
	InboundWebhookReceived    = "received"
)

type InboundWebhookIdentity struct {
	ClientID         string
	TransportEventID string
}

type InboundWebhookInput struct {
	PackageID        int64
	ClientID         string
	TransportEventID string
	ExternalEventID  string
	MemberEventID    *int64
	Status           string
	Message          json.RawMessage
	Action           json.RawMessage
	PayloadDigest    [sha256.Size]byte
}

type InboundWebhookReceipt struct {
	ID                    int64     `json:"id"`
	PackageID             int64     `json:"package_id"`
	State                 string    `json:"status"`
	ExternalEventIDDigest [32]byte  `json:"-"`
	PayloadDigest         [32]byte  `json:"-"`
	CreatedAt             time.Time `json:"created_at"`
}

type InboundWebhookResult struct {
	Receipt  InboundWebhookReceipt
	Replayed bool
}

type InboundWebhookReservation struct {
	PackageID              int64
	ClientID               string
	TransportEventIDDigest [32]byte
	ExternalEventIDDigest  [32]byte
	PayloadDigest          [32]byte
	MemberEventID          *int64
	CallbackStatus         string
	Message                json.RawMessage
	Action                 json.RawMessage
	CreatedAt              time.Time
}

type InboundWebhookTransportReplay struct {
	ReceiptID     int64
	PayloadDigest [32]byte
}

type InboundWebhookRepository interface {
	PackageExistsForInbound(context.Context, int64) (bool, error)
	ReserveInboundWebhook(context.Context, InboundWebhookReservation) (InboundWebhookReceipt, bool, error)
}

type InboundWebhookApplication interface {
	Accept(context.Context, InboundWebhookInput) (InboundWebhookResult, error)
}

type InboundWebhookService struct {
	uow    UnitOfWork
	repo   InboundWebhookRepository
	events EventAppender
	now    func() time.Time
}

func NewInboundWebhookService(uow UnitOfWork, repo InboundWebhookRepository, events EventAppender) (*InboundWebhookService, error) {
	if nilInterface(uow) || nilInterface(repo) || nilInterface(events) {
		return nil, ErrUnavailable
	}
	return &InboundWebhookService{uow: uow, repo: repo, events: events, now: time.Now}, nil
}

func (service *InboundWebhookService) Accept(ctx context.Context, input InboundWebhookInput) (InboundWebhookResult, error) {
	if ctx == nil || service == nil || service.now == nil || validateInboundWebhookInput(input) != nil {
		return InboundWebhookResult{}, ErrInvalidInput
	}
	reservation := InboundWebhookReservation{
		PackageID: input.PackageID, ClientID: input.ClientID,
		TransportEventIDDigest: sha256.Sum256([]byte(input.TransportEventID)),
		ExternalEventIDDigest:  sha256.Sum256([]byte(input.ExternalEventID)),
		PayloadDigest:          input.PayloadDigest, MemberEventID: cloneInt64(input.MemberEventID),
		CallbackStatus: input.Status, Message: append(json.RawMessage(nil), input.Message...),
		Action: append(json.RawMessage(nil), input.Action...), CreatedAt: service.now().UTC(),
	}
	if reservation.CreatedAt.IsZero() {
		return InboundWebhookResult{}, ErrUnavailable
	}
	var result InboundWebhookResult
	err := service.uow.Within(ctx, func(tx context.Context) error {
		exists, err := service.repo.PackageExistsForInbound(tx, input.PackageID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		receipt, created, err := service.repo.ReserveInboundWebhook(tx, reservation)
		if err != nil {
			return err
		}
		if !validInboundWebhookReceipt(receipt, reservation) {
			return ErrUnavailable
		}
		if created {
			payload, marshalErr := json.Marshal(struct {
				ReceiptID     int64  `json:"receipt_id"`
				PackageID     int64  `json:"package_id"`
				PayloadDigest string `json:"payload_digest"`
			}{receipt.ID, receipt.PackageID, "sha256:" + hex.EncodeToString(receipt.PayloadDigest[:])})
			if marshalErr != nil {
				return ErrUnavailable
			}
			if err = service.events.Append(tx, LocalEvent{
				Type: "ai_audience.inbound_webhook.received", Payload: payload,
				OccurredAt: receipt.CreatedAt, IdempotencyKey: "ai_audience.inbound_webhook.received:" + strconv.FormatInt(receipt.PackageID, 10) + ":" + hex.EncodeToString(receipt.ExternalEventIDDigest[:]),
			}); err != nil {
				return err
			}
		}
		result = InboundWebhookResult{Receipt: receipt, Replayed: !created}
		return nil
	})
	if err != nil {
		return InboundWebhookResult{}, classifyServiceError(err)
	}
	return result, nil
}

func validateInboundWebhookInput(input InboundWebhookInput) error {
	if input.PackageID < 1 || input.ClientID != AIAudienceWebhookClientID || !validInboundEventID(input.TransportEventID, 16) ||
		!validInboundEventID(input.ExternalEventID, 1) || len(input.Status) > 64 || strings.TrimSpace(input.Status) != input.Status ||
		!utf8.ValidString(input.Status) || input.PayloadDigest == ([32]byte{}) || !validJSONObject(input.Message) || !validJSONObject(input.Action) {
		return ErrInvalidInput
	}
	if input.MemberEventID != nil && *input.MemberEventID < 1 {
		return ErrInvalidInput
	}
	return nil
}

func validInboundEventID(value string, minimum int) bool {
	if len(value) < minimum || len(value) > 256 || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character == 0x7f {
			return false
		}
	}
	return true
}

func validJSONObject(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid([]byte(trimmed))
}

func validInboundWebhookReceipt(receipt InboundWebhookReceipt, reservation InboundWebhookReservation) bool {
	return receipt.ID > 0 && receipt.PackageID == reservation.PackageID && receipt.State == InboundWebhookReceived &&
		receipt.ExternalEventIDDigest == reservation.ExternalEventIDDigest && receipt.PayloadDigest == reservation.PayloadDigest &&
		!receipt.CreatedAt.IsZero()
}
