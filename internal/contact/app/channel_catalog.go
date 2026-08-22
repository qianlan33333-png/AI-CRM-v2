// Package app owns Contact-local channel resources used by customer attribution.
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
	"reflect"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const MaximumChannelListLimit int32 = 500

var (
	ErrInvalidChannel     = errors.New("invalid channel")
	ErrChannelNotFound    = errors.New("channel not found")
	ErrChannelConflict    = errors.New("channel command conflict")
	ErrChannelUnavailable = errors.New("channel catalog unavailable")
)

type Channel struct {
	ID               int64           `json:"id"`
	ChannelCode      string          `json:"channel_code"`
	ChannelName      string          `json:"channel_name"`
	Status           string          `json:"status"`
	LegacyProjection json.RawMessage `json:"legacy_projection"`
	CreatedBy        int64           `json:"created_by"`
	UpdatedBy        int64           `json:"updated_by"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type CreateChannelCommand struct {
	Actor, ChannelID                                 int64
	IdempotencyKey, ChannelCode, ChannelName, Status string
	LegacyProjection                                 json.RawMessage
}

type UpdateChannelCommand struct {
	Actor, ChannelID int64
	IdempotencyKey   string
	Patch            json.RawMessage
}

type ChannelReceipt struct {
	ID                       int64
	Operation, ActorScope    string
	KeyDigest, PayloadDigest [32]byte
	State                    string
	ResultSnapshot           json.RawMessage
}

type ChannelReservation struct {
	Operation, ActorScope    string
	KeyDigest, PayloadDigest [32]byte
	CreatedAt                time.Time
}

type ChannelStore interface {
	ListChannels(context.Context, int32, string, bool) ([]Channel, error)
	GetChannel(context.Context, int64) (Channel, error)
	CreateChannel(context.Context, CreateChannelCommand, time.Time) (Channel, error)
	UpdateChannel(context.Context, Channel, int64, time.Time) (Channel, error)
	ReserveChannel(context.Context, ChannelReservation) (ChannelReceipt, bool, error)
	CompleteChannel(context.Context, int64, json.RawMessage, time.Time) (ChannelReceipt, error)
}

type ChannelService struct {
	uow         platformport.UnitOfWork
	store       ChannelStore
	images      mediaport.ImageMetadataReader
	attachments mediaport.AttachmentMetadataReader
	events      eventport.Appender
	now         func() time.Time
}

func NewChannelService(uow platformport.UnitOfWork, store ChannelStore, events eventport.Appender) *ChannelService {
	return &ChannelService{uow: uow, store: store, events: events, now: time.Now}
}

// NewChannelServiceWithImageReferences wires Media's transaction-bound image
// reader for welcome_image_library_ids in the Contact-owned projection.
func NewChannelServiceWithImageReferences(uow platformport.UnitOfWork, store ChannelStore, images mediaport.ImageMetadataReader, events eventport.Appender) *ChannelService {
	return NewChannelServiceWithMediaReferences(uow, store, images, nil, events)
}

// NewChannelServiceWithMediaReferences keeps the channel-owned JSON
// projection while validating local Media IDs in the same UoW that persists
// it. Attachment validation intentionally fails closed until its reader is
// wired by the Attachment Library composition root.
func NewChannelServiceWithMediaReferences(uow platformport.UnitOfWork, store ChannelStore, images mediaport.ImageMetadataReader, attachments mediaport.AttachmentMetadataReader, events eventport.Appender) *ChannelService {
	return &ChannelService{uow: uow, store: store, images: images, attachments: attachments, events: events, now: time.Now}
}

func (service *ChannelService) ListChannels(ctx context.Context, limit int32, status string, includeArchived bool) ([]Channel, error) {
	status = strings.TrimSpace(status)
	if !channelReady(service) || limit < 1 || limit > MaximumChannelListLimit || status != "" && !validChannelStatus(status) {
		return nil, ErrInvalidChannel
	}
	var rows []Channel
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		rows, err = service.store.ListChannels(tx, limit, status, includeArchived)
		return err
	})
	if err != nil || !validChannels(rows) {
		return nil, classifyChannel(err)
	}
	return cloneChannels(rows), nil
}

func (service *ChannelService) GetChannel(ctx context.Context, id int64) (Channel, error) {
	if !channelReady(service) || id < 1 {
		return Channel{}, ErrChannelNotFound
	}
	var result Channel
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		result, err = service.store.GetChannel(tx, id)
		return err
	})
	if err != nil || !validChannel(result) {
		return Channel{}, classifyChannel(err)
	}
	return cloneChannel(result), nil
}

func (service *ChannelService) CreateChannel(ctx context.Context, command CreateChannelCommand) (Channel, error) {
	if !channelReady(service) {
		return Channel{}, ErrChannelUnavailable
	}
	normalized, err := normalizeCreate(command)
	if err != nil {
		return Channel{}, err
	}
	return service.mutate(ctx, "create", normalized.Actor, normalized.IdempotencyKey, normalized, func(tx context.Context, now time.Time) (Channel, error) {
		if err := service.validateMediaReferences(tx, normalized.LegacyProjection); err != nil {
			return Channel{}, err
		}
		return service.store.CreateChannel(tx, normalized, now)
	})
}

func (service *ChannelService) UpdateChannel(ctx context.Context, command UpdateChannelCommand) (Channel, error) {
	if !channelReady(service) || command.Actor < 1 || command.ChannelID < 1 || !validKey(command.IdempotencyKey) || len(command.Patch) == 0 || !json.Valid(command.Patch) {
		return Channel{}, ErrInvalidChannel
	}
	return service.mutate(ctx, "update", command.Actor, command.IdempotencyKey, command, func(tx context.Context, now time.Time) (Channel, error) {
		current, err := service.store.GetChannel(tx, command.ChannelID)
		if err != nil {
			return Channel{}, err
		}
		merged, err := mergeChannel(current, command.Patch)
		if err != nil {
			return Channel{}, err
		}
		if err := service.validateMediaReferences(tx, merged.LegacyProjection); err != nil {
			return Channel{}, err
		}
		return service.store.UpdateChannel(tx, merged, command.Actor, now)
	})
}

func (service *ChannelService) mutate(ctx context.Context, operation string, actor int64, key string, payload any, apply func(context.Context, time.Time) (Channel, error)) (Channel, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return Channel{}, ErrInvalidChannel
	}
	canonical, err := canonicalJSON(payloadBytes)
	if err != nil {
		return Channel{}, ErrInvalidChannel
	}
	now := service.now().UTC()
	if now.IsZero() {
		return Channel{}, ErrChannelUnavailable
	}
	reservation := ChannelReservation{Operation: operation, ActorScope: fmt.Sprintf("admin:%d", actor), KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: sha256.Sum256(canonical), CreatedAt: now}
	var result Channel
	err = service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, reserveErr := service.store.ReserveChannel(tx, reservation)
		if reserveErr != nil {
			return reserveErr
		}
		if !validChannelReceipt(receipt, reservation) {
			return ErrChannelUnavailable
		}
		if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], reservation.PayloadDigest[:]) != 1 {
			return ErrChannelConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validChannel(result) {
				return ErrChannelUnavailable
			}
			snapshot, snapErr := json.Marshal(result)
			if snapErr != nil || !jsonEquivalent(snapshot, receipt.ResultSnapshot) {
				return ErrChannelUnavailable
			}
			return nil
		}
		result, reserveErr = apply(tx, now)
		if reserveErr != nil {
			return reserveErr
		}
		if !validChannel(result) {
			return ErrChannelUnavailable
		}
		snapshot, snapErr := json.Marshal(result)
		if snapErr != nil {
			return snapErr
		}
		eventPayload, snapErr := json.Marshal(map[string]any{"channel_id": result.ID, "actor": actor})
		if snapErr != nil {
			return snapErr
		}
		eventType := eventport.EvChannelCreated
		if operation == "update" {
			eventType = eventport.EvChannelUpdated
		}
		eventKey := sha256.Sum256([]byte(reservation.ActorScope + "\x00" + operation + "\x00" + key))
		if _, snapErr = service.events.Append(tx, eventport.Event{Type: eventType, Payload: eventPayload, OccurredAt: now, IdempotencyKey: "channel." + operation + ":" + hex.EncodeToString(eventKey[:])}); snapErr != nil {
			return snapErr
		}
		completed, snapErr := service.store.CompleteChannel(tx, receipt.ID, snapshot, now)
		if snapErr != nil || !validChannelReceipt(completed, reservation) || completed.State != "completed" || !jsonEquivalent(completed.ResultSnapshot, snapshot) {
			return ErrChannelUnavailable
		}
		return nil
	})
	if err != nil {
		return Channel{}, classifyChannel(err)
	}
	return cloneChannel(result), nil
}

func normalizeCreate(command CreateChannelCommand) (CreateChannelCommand, error) {
	if command.Actor < 1 || !validKey(command.IdempotencyKey) {
		return CreateChannelCommand{}, ErrInvalidChannel
	}
	command.ChannelCode = strings.TrimSpace(command.ChannelCode)
	if command.ChannelCode == "" {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s", command.Actor, command.IdempotencyKey)))
		command.ChannelCode = "channel_" + hex.EncodeToString(digest[:16])
	}
	command.ChannelName = strings.TrimSpace(command.ChannelName)
	if command.ChannelName == "" {
		command.ChannelName = command.ChannelCode
	}
	command.Status = strings.TrimSpace(command.Status)
	if command.Status == "" {
		command.Status = "active"
	}
	if !validText(command.ChannelCode, 200) || !validText(command.ChannelName, 200) || !validChannelStatus(command.Status) {
		return CreateChannelCommand{}, ErrInvalidChannel
	}
	projection, err := normalizeProjection(command.LegacyProjection, command.ChannelCode, command.ChannelName, command.Status)
	if err != nil {
		return CreateChannelCommand{}, err
	}
	command.LegacyProjection = projection
	return command, nil
}

func mergeChannel(current Channel, patch json.RawMessage) (Channel, error) {
	base, err := object(current.LegacyProjection)
	if err != nil {
		return Channel{}, ErrChannelUnavailable
	}
	changes, err := object(patch)
	if err != nil {
		return Channel{}, ErrInvalidChannel
	}
	for key, value := range changes {
		if key != "schema_version" {
			base[key] = value
		}
	}
	code := current.ChannelCode
	if raw, ok := base["channel_code"]; ok {
		if json.Unmarshal(raw, &code) != nil || strings.TrimSpace(code) != current.ChannelCode {
			return Channel{}, ErrInvalidChannel
		}
	}
	name, status := current.ChannelName, current.Status
	if raw, ok := base["channel_name"]; ok {
		if json.Unmarshal(raw, &name) != nil {
			return Channel{}, ErrInvalidChannel
		}
	}
	if raw, ok := base["status"]; ok {
		if json.Unmarshal(raw, &status) != nil {
			return Channel{}, ErrInvalidChannel
		}
	}
	projection, err := json.Marshal(base)
	if err != nil {
		return Channel{}, ErrInvalidChannel
	}
	projection, err = normalizeProjection(projection, current.ChannelCode, strings.TrimSpace(name), strings.TrimSpace(status))
	if err != nil {
		return Channel{}, err
	}
	current.ChannelName, current.Status, current.LegacyProjection = strings.TrimSpace(name), strings.TrimSpace(status), projection
	return current, nil
}

var channelProjectionKeys = map[string]bool{
	"schema_version": true, "channel_type": true, "carrier_type": true, "channel_name": true, "channel_code": true,
	"scene_value": true, "qr_url": true, "status": true, "owner_staff_id": true, "customer_channel": true,
	"link_url": true, "final_url": true, "welcome_message": true, "welcome_image_library_ids": true,
	"welcome_miniprogram_library_ids": true, "welcome_attachment_library_ids": true, "welcome_group_invite_library_ids": true,
	"auto_accept_friend": true, "entry_tag_id": true, "entry_tag_name": true, "entry_tag_group_name": true,
	"assignment_mode": true, "assignment_strategy": true, "overflow_policy": true, "assignment_config_json": true,
}

func normalizeProjection(raw json.RawMessage, code, name, status string) (json.RawMessage, error) {
	values := map[string]json.RawMessage{}
	if len(raw) > 0 {
		var err error
		values, err = object(raw)
		if err != nil {
			return nil, ErrInvalidChannel
		}
	}
	for key := range values {
		if !channelProjectionKeys[key] {
			return nil, ErrInvalidChannel
		}
	}
	defaults := map[string]any{"schema_version": 1, "channel_type": "qrcode", "carrier_type": "qrcode", "channel_code": code, "channel_name": name, "status": status,
		"scene_value": "", "qr_url": "", "owner_staff_id": "", "customer_channel": "", "link_url": "", "final_url": "", "welcome_message": "",
		"welcome_image_library_ids": []int64{}, "welcome_miniprogram_library_ids": []int64{}, "welcome_attachment_library_ids": []int64{}, "welcome_group_invite_library_ids": []int64{},
		"auto_accept_friend": false, "entry_tag_id": "", "entry_tag_name": "", "entry_tag_group_name": "", "assignment_mode": "single_owner", "assignment_strategy": "ratio", "overflow_policy": "least_loaded", "assignment_config_json": map[string]any{},
	}
	for key, value := range defaults {
		if _, ok := values[key]; !ok {
			encoded, _ := json.Marshal(value)
			values[key] = encoded
		}
	}
	for key, value := range map[string]string{"channel_code": code, "channel_name": name, "status": status} {
		encoded, _ := json.Marshal(value)
		values[key] = encoded
	}
	var schema int
	if json.Unmarshal(values["schema_version"], &schema) != nil || schema != 1 {
		return nil, ErrInvalidChannel
	}
	for _, key := range []string{"channel_type", "carrier_type", "channel_code", "channel_name", "scene_value", "qr_url", "status", "owner_staff_id", "customer_channel", "link_url", "final_url", "welcome_message", "entry_tag_id", "entry_tag_name", "entry_tag_group_name", "assignment_mode", "assignment_strategy", "overflow_policy"} {
		var value string
		if json.Unmarshal(values[key], &value) != nil || len(value) > 10000 {
			return nil, ErrInvalidChannel
		}
	}
	for _, key := range []string{"welcome_image_library_ids", "welcome_miniprogram_library_ids", "welcome_attachment_library_ids", "welcome_group_invite_library_ids"} {
		var ids []int64
		if json.Unmarshal(values[key], &ids) != nil || len(ids) > 12 {
			return nil, ErrInvalidChannel
		}
		for _, id := range ids {
			if id < 1 {
				return nil, ErrInvalidChannel
			}
		}
	}
	var flag bool
	if json.Unmarshal(values["auto_accept_friend"], &flag) != nil {
		return nil, ErrInvalidChannel
	}
	var assignment map[string]any
	if json.Unmarshal(values["assignment_config_json"], &assignment) != nil {
		return nil, ErrInvalidChannel
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, ErrInvalidChannel
	}
	return canonicalJSON(encoded)
}

func (service *ChannelService) validateImageReferences(ctx context.Context, projection json.RawMessage) error {
	values, err := object(projection)
	if err != nil {
		return ErrChannelUnavailable
	}
	var imageIDs []int64
	if err := json.Unmarshal(values["welcome_image_library_ids"], &imageIDs); err != nil {
		return ErrChannelUnavailable
	}
	if len(imageIDs) == 0 {
		return nil
	}
	if service == nil || service.images == nil {
		return ErrChannelUnavailable
	}
	for _, imageID := range imageIDs {
		exists, err := service.images.ImageExists(ctx, imageID)
		if err != nil {
			return ErrChannelUnavailable
		}
		if !exists {
			return ErrInvalidChannel
		}
	}
	return nil
}

func (service *ChannelService) validateAttachmentReferences(ctx context.Context, projection json.RawMessage) error {
	values, err := object(projection)
	if err != nil {
		return ErrChannelUnavailable
	}
	var attachmentIDs []int64
	if err := json.Unmarshal(values["welcome_attachment_library_ids"], &attachmentIDs); err != nil {
		return ErrChannelUnavailable
	}
	if len(attachmentIDs) == 0 {
		return nil
	}
	if service == nil || service.attachments == nil {
		return ErrChannelUnavailable
	}
	for _, attachmentID := range attachmentIDs {
		exists, err := service.attachments.AttachmentExists(ctx, attachmentID)
		if err != nil {
			return ErrChannelUnavailable
		}
		if !exists {
			return ErrInvalidChannel
		}
	}
	return nil
}

func (service *ChannelService) validateMediaReferences(ctx context.Context, projection json.RawMessage) error {
	if err := service.validateImageReferences(ctx, projection); err != nil {
		return err
	}
	return service.validateAttachmentReferences(ctx, projection)
}

func object(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var result map[string]json.RawMessage
	if decoder.Decode(&result) != nil || result == nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return nil, ErrInvalidChannel
	}
	return result, nil
}
func canonicalJSON(raw []byte) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return nil, ErrInvalidChannel
	}
	return json.Marshal(value)
}
func jsonEquivalent(left, right []byte) bool {
	var a, b any
	da, db := json.NewDecoder(bytes.NewReader(left)), json.NewDecoder(bytes.NewReader(right))
	da.UseNumber()
	db.UseNumber()
	return da.Decode(&a) == nil && db.Decode(&b) == nil && reflect.DeepEqual(a, b)
}
func validChannelStatus(v string) bool { return v == "active" || v == "inactive" || v == "archived" }
func validText(v string, max int) bool { return v != "" && strings.TrimSpace(v) == v && len(v) <= max }
func validKey(v string) bool           { return len(v) >= 16 && len(v) <= 128 && strings.TrimSpace(v) == v }
func validChannel(v Channel) bool {
	return v.ID > 0 && validText(v.ChannelCode, 200) && validText(v.ChannelName, 200) && validChannelStatus(v.Status) && v.CreatedBy > 0 && v.UpdatedBy > 0 && !v.CreatedAt.IsZero() && !v.UpdatedAt.Before(v.CreatedAt) && json.Valid(v.LegacyProjection)
}
func validChannels(v []Channel) bool {
	for _, item := range v {
		if !validChannel(item) {
			return false
		}
	}
	return true
}
func validChannelReceipt(r ChannelReceipt, x ChannelReservation) bool {
	return r.ID > 0 && r.Operation == x.Operation && r.ActorScope == x.ActorScope && subtle.ConstantTimeCompare(r.KeyDigest[:], x.KeyDigest[:]) == 1 && len(r.ResultSnapshot) == 0 && r.State == "in_progress" || r.ID > 0 && r.Operation == x.Operation && r.ActorScope == x.ActorScope && subtle.ConstantTimeCompare(r.KeyDigest[:], x.KeyDigest[:]) == 1 && r.State == "completed" && json.Valid(r.ResultSnapshot)
}
func channelReady(s *ChannelService) bool {
	return s != nil && s.uow != nil && s.store != nil && s.events != nil && s.now != nil
}
func cloneChannel(v Channel) Channel {
	v.LegacyProjection = append([]byte(nil), v.LegacyProjection...)
	return v
}
func cloneChannels(v []Channel) []Channel {
	out := make([]Channel, len(v))
	for i := range v {
		out[i] = cloneChannel(v[i])
	}
	return out
}
func classifyChannel(err error) error {
	if err == nil {
		return ErrChannelUnavailable
	}
	switch {
	case errors.Is(err, ErrInvalidChannel), errors.Is(err, ErrChannelNotFound), errors.Is(err, ErrChannelConflict), errors.Is(err, ErrChannelUnavailable):
		return err
	default:
		return ErrChannelUnavailable
	}
}
