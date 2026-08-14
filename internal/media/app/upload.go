package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidUpload = errors.New("invalid image upload")
	ErrConflict      = errors.New("image upload conflict")
	ErrUnavailable   = errors.New("image upload unavailable")
)

type Reservation struct {
	ActorScope               string
	KeyDigest, PayloadDigest [32]byte
	CreatedAt                time.Time
}
type Receipt struct {
	ID                       int64
	ActorScope               string
	KeyDigest, PayloadDigest [32]byte
	State                    string
	ResultSnapshot           json.RawMessage
}
type CreateInput struct {
	Command   mediaport.UploadCommand
	MediaType string
	Width     int32
	Height    int32
	Checksum  [32]byte
	Now       time.Time
}
type Store interface {
	Reserve(context.Context, Reservation) (Receipt, bool, error)
	Create(context.Context, CreateInput) (mediaport.Image, error)
	Complete(context.Context, int64, json.RawMessage, time.Time) (Receipt, error)
}
type Service struct {
	uow    platformport.UnitOfWork
	store  Store
	events eventport.Appender
	now    func() time.Time
}

func NewService(uow platformport.UnitOfWork, store Store, events eventport.Appender) *Service {
	return &Service{uow: uow, store: store, events: events, now: time.Now}
}

func (s *Service) Upload(ctx context.Context, command mediaport.UploadCommand) (mediaport.Image, error) {
	command.Name = strings.TrimSpace(command.Name)
	command.Description = strings.TrimSpace(command.Description)
	command.Tags = strings.TrimSpace(command.Tags)
	command.Category = strings.TrimSpace(command.Category)
	inspection, err := domain.Inspect(command.FileName, command.DeclaredType, command.Content)
	if err != nil || command.Actor < 1 || len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 128 ||
		strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey || len(command.Name) > 200 ||
		len(command.Description) > 10_000 || len(command.Tags) > 10_000 || len(command.Category) > 200 {
		return mediaport.Image{}, ErrInvalidUpload
	}
	if s == nil || s.uow == nil || s.store == nil || s.events == nil {
		return mediaport.Image{}, ErrUnavailable
	}
	now := s.now().UTC()
	if now.IsZero() {
		return mediaport.Image{}, ErrUnavailable
	}
	checksum := sha256.Sum256(command.Content)
	payload, err := json.Marshal(struct {
		FileName, MediaType, Name, Description, Tags, Category string
		Size, Width, Height                                    int32
		Checksum                                               string
	}{command.FileName, inspection.MediaType, command.Name, command.Description, command.Tags, command.Category,
		int32(len(command.Content)), inspection.Width, inspection.Height, hex.EncodeToString(checksum[:])})
	if err != nil {
		return mediaport.Image{}, ErrUnavailable
	}
	actorScope := fmt.Sprintf("admin:%d", command.Actor)
	reservation := Reservation{ActorScope: actorScope, KeyDigest: sha256.Sum256([]byte(command.IdempotencyKey)), PayloadDigest: sha256.Sum256(payload), CreatedAt: now}
	var result mediaport.Image
	err = s.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := s.store.Reserve(tx, reservation)
		if reserveErr != nil || !validReceipt(receipt, reservation) {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validImage(result) {
				return ErrUnavailable
			}
			canonical, marshalErr := json.Marshal(result)
			if marshalErr != nil || !jsonEquivalent(canonical, receipt.ResultSnapshot) {
				return ErrUnavailable
			}
			return nil
		}
		result, reserveErr = s.store.Create(tx, CreateInput{Command: command, MediaType: inspection.MediaType, Width: inspection.Width, Height: inspection.Height, Checksum: checksum, Now: now})
		if reserveErr != nil || !validImage(result) {
			return ErrUnavailable
		}
		snapshot, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return ErrUnavailable
		}
		eventPayload, marshalErr := json.Marshal(struct {
			ImageID int64 `json:"image_id"`
			Actor   int64 `json:"actor"`
		}{result.ID, command.Actor})
		if marshalErr != nil {
			return ErrUnavailable
		}
		eventDigest := sha256.Sum256([]byte(actorScope + "\x00" + command.IdempotencyKey))
		if _, reserveErr = s.events.Append(tx, eventport.Event{Type: eventport.EvMediaImageCreated, Payload: eventPayload, OccurredAt: now, IdempotencyKey: "media.image_created:" + hex.EncodeToString(eventDigest[:])}); reserveErr != nil {
			return reserveErr
		}
		completed, completeErr := s.store.Complete(tx, receipt.ID, snapshot, now)
		if completeErr != nil || completed.State != "completed" || !jsonEquivalent(snapshot, completed.ResultSnapshot) {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return mediaport.Image{}, ErrConflict
		}
		return mediaport.Image{}, ErrUnavailable
	}
	return result, nil
}

func validReceipt(receipt Receipt, expected Reservation) bool {
	return receipt.ID > 0 && receipt.ActorScope == expected.ActorScope &&
		subtle.ConstantTimeCompare(receipt.KeyDigest[:], expected.KeyDigest[:]) == 1 &&
		(receipt.State == "in_progress" || receipt.State == "completed")
}
func validImage(image mediaport.Image) bool {
	return image.ID > 0 && image.FileName != "" && image.FileSize > 0 && image.Width > 0 && image.Height > 0 && !image.CreatedAt.IsZero() && !image.UpdatedAt.IsZero()
}
func jsonEquivalent(left, right []byte) bool {
	var a, b any
	return decodeExact(left, &a) == nil && decodeExact(right, &b) == nil && fmt.Sprintf("%#v", a) == fmt.Sprintf("%#v", b)
}
func decodeExact(value []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrUnavailable
	}
	return nil
}
