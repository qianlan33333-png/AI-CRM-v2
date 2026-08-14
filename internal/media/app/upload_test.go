package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

type memoryState struct {
	receipts map[string]Receipt
	images   []mediaport.Image
	events   []eventport.Event
	fail     bool
}
type memoryUOW struct{ state *memoryState }
type memoryStore struct{ state *memoryState }
type memoryEvents struct{ state *memoryState }

func (u memoryUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	backupReceipts := make(map[string]Receipt, len(u.state.receipts))
	for key, value := range u.state.receipts {
		backupReceipts[key] = value
	}
	images, events := append([]mediaport.Image(nil), u.state.images...), append([]eventport.Event(nil), u.state.events...)
	if err := fn(ctx); err != nil {
		u.state.receipts, u.state.images, u.state.events = backupReceipts, images, events
		return err
	}
	return nil
}
func receiptKey(actor string, digest [32]byte) string { return actor + ":" + string(digest[:]) }
func (s memoryStore) Reserve(_ context.Context, input Reservation) (Receipt, bool, error) {
	key := receiptKey(input.ActorScope, input.KeyDigest)
	if old, ok := s.state.receipts[key]; ok {
		return old, false, nil
	}
	value := Receipt{ID: int64(len(s.state.receipts) + 1), ActorScope: input.ActorScope, KeyDigest: input.KeyDigest, PayloadDigest: input.PayloadDigest, State: "in_progress"}
	s.state.receipts[key] = value
	return value, true, nil
}
func (s memoryStore) Create(_ context.Context, input CreateInput) (mediaport.Image, error) {
	value := mediaport.Image{ID: int64(len(s.state.images) + 1), Name: input.Command.Name, FileName: input.Command.FileName,
		FileSize: int32(len(input.Command.Content)), MimeType: input.MediaType, Width: input.Width, Height: input.Height,
		Description: input.Command.Description, Tags: input.Command.Tags, Category: input.Command.Category,
		CreatedAt: input.Now, UpdatedAt: input.Now}
	s.state.images = append(s.state.images, value)
	return value, nil
}
func (s memoryStore) Complete(_ context.Context, id int64, snapshot json.RawMessage, _ time.Time) (Receipt, error) {
	for key, value := range s.state.receipts {
		if value.ID == id {
			value.State, value.ResultSnapshot = "completed", append([]byte(nil), snapshot...)
			s.state.receipts[key] = value
			return value, nil
		}
	}
	return Receipt{}, ErrUnavailable
}
func (e memoryEvents) Append(_ context.Context, value eventport.Event) (eventport.EventID, error) {
	if e.state.fail {
		return 0, errors.New("event unavailable")
	}
	e.state.events = append(e.state.events, value)
	return eventport.EventID(len(e.state.events)), nil
}

func TestUploadActorScopedReplayConflictAndRollback(t *testing.T) {
	state := &memoryState{receipts: map[string]Receipt{}}
	service := NewService(memoryUOW{state}, memoryStore{state}, memoryEvents{state})
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	command := validCommand(t)
	first, err := service.Upload(context.Background(), command)
	if err != nil || len(state.images) != 1 || len(state.events) != 1 || state.events[0].Type != eventport.EvMediaImageCreated {
		t.Fatalf("create failed: %#v %v images=%d events=%d", first, err, len(state.images), len(state.events))
	}
	replay, err := service.Upload(context.Background(), command)
	if err != nil || replay != first || len(state.images) != 1 || len(state.events) != 1 {
		t.Fatalf("replay changed effects: %#v %v", replay, err)
	}
	conflict := command
	conflict.Name = "different"
	if _, err = service.Upload(context.Background(), conflict); !errors.Is(err, ErrConflict) || len(state.images) != 1 || len(state.events) != 1 {
		t.Fatalf("conflict not isolated: %v", err)
	}
	otherActor := command
	otherActor.Actor = 8
	if _, err = service.Upload(context.Background(), otherActor); err != nil || len(state.images) != 2 || len(state.events) != 2 || state.events[0].IdempotencyKey == state.events[1].IdempotencyKey {
		t.Fatalf("actor isolation failed: %v", err)
	}
	state.fail = true
	failing := command
	failing.IdempotencyKey = "rollback-key-000001"
	if _, err = service.Upload(context.Background(), failing); !errors.Is(err, ErrUnavailable) || len(state.images) != 2 || len(state.events) != 2 || len(state.receipts) != 2 {
		t.Fatalf("rollback failed: %v state=%#v", err, state)
	}
}

func validCommand(t *testing.T) mediaport.UploadCommand {
	t.Helper()
	var data bytes.Buffer
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.Set(0, 0, color.RGBA{R: 5, A: 255})
	if err := png.Encode(&data, pixel); err != nil {
		t.Fatal(err)
	}
	return mediaport.UploadCommand{Actor: 7, IdempotencyKey: "upload-key-000001", FileName: "image.png", DeclaredType: "image/png", Content: data.Bytes(), Name: "image", Description: "desc", Tags: "a,b", Category: "cover"}
}
