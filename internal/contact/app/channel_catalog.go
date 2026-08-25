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
	"strconv"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
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
	ID               int64             `json:"id"`
	ChannelCode      string            `json:"channel_code"`
	ChannelName      string            `json:"channel_name"`
	Status           string            `json:"status"`
	LegacyProjection json.RawMessage   `json:"legacy_projection"`
	CreatedBy        int64             `json:"created_by"`
	UpdatedBy        int64             `json:"updated_by"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Assignees        []ChannelAssignee `json:"assignees,omitempty"`
}

type ChannelAssignee struct {
	WeComUserID  string `json:"wecom_userid"`
	DisplayName  string `json:"display_name"`
	Status       string `json:"status"`
	Priority     int32  `json:"priority"`
	RatioPercent *int32 `json:"ratio_percent,omitempty"`
	MaxScans24h  *int32 `json:"max_scans_24h,omitempty"`
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
	uow          platformport.UnitOfWork
	store        ChannelStore
	images       mediaport.ImageMetadataReader
	attachments  mediaport.ChannelAttachmentReferenceReader
	miniprograms mediaport.ChannelMiniProgramReferenceReader
	groupInvites mediaport.ChannelGroupInviteReferenceReader
	tags         contactport.TagReferenceReader
	staff        contactport.EligibleStaffReferenceReader
	events       eventport.Appender
	now          func() time.Time
}

func NewChannelService(uow platformport.UnitOfWork, store ChannelStore, events eventport.Appender) *ChannelService {
	return &ChannelService{uow: uow, store: store, events: events, now: time.Now}
}

// NewChannelServiceWithImageReferences wires Media's transaction-bound image
// reader for welcome_image_library_ids in the Contact-owned projection.
func NewChannelServiceWithImageReferences(uow platformport.UnitOfWork, store ChannelStore, images mediaport.ImageMetadataReader, events eventport.Appender) *ChannelService {
	return &ChannelService{uow: uow, store: store, images: images, events: events, now: time.Now}
}

// NewChannelServiceWithMediaReferences preserves the Attachment Library seam
// while requiring its Channel-specific enabled-only reader.
func NewChannelServiceWithMediaReferences(uow platformport.UnitOfWork, store ChannelStore, images mediaport.ImageMetadataReader, attachments mediaport.ChannelAttachmentReferenceReader, events eventport.Appender) *ChannelService {
	return &ChannelService{uow: uow, store: store, images: images, attachments: attachments, events: events, now: time.Now}
}

// NewChannelServiceWithLocalReferences validates Contact-owned tags and staff
// in the same UnitOfWork as the channel mutation. It intentionally does not
// accept Attachment, MiniProgram, or GroupInvite dependencies before their
// authoritative shared reference contracts are available.
func NewChannelServiceWithLocalReferences(uow platformport.UnitOfWork, store ChannelStore, images mediaport.ImageMetadataReader, tags contactport.TagReferenceReader, staff contactport.EligibleStaffReferenceReader, events eventport.Appender) *ChannelService {
	return &ChannelService{uow: uow, store: store, images: images, tags: tags, staff: staff, events: events, now: time.Now}
}

// NewChannelServiceWithReferences validates every local welcome reference in
// the same transaction that persists the Channel projection.
func NewChannelServiceWithReferences(uow platformport.UnitOfWork, store ChannelStore, images mediaport.ImageMetadataReader, attachments mediaport.ChannelAttachmentReferenceReader, miniprograms mediaport.ChannelMiniProgramReferenceReader, groupInvites mediaport.ChannelGroupInviteReferenceReader, tags contactport.TagReferenceReader, staff contactport.EligibleStaffReferenceReader, events eventport.Appender) *ChannelService {
	return &ChannelService{uow: uow, store: store, images: images, attachments: attachments, miniprograms: miniprograms, groupInvites: groupInvites, tags: tags, staff: staff, events: events, now: time.Now}
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
		if err != nil {
			return err
		}
		return service.hydrateAssignees(tx, rows)
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
		if err != nil {
			return err
		}
		return service.hydrateChannelAssignees(tx, &result)
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
		if err := service.validateLocalReferences(tx, normalized.LegacyProjection); err != nil {
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
		if err := service.validateLocalReferences(tx, merged.LegacyProjection); err != nil {
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
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || !validChannelReceiptSnapshot(result) {
				return ErrChannelUnavailable
			}
			if reserveErr = service.hydrateChannelAssignees(tx, &result); reserveErr != nil {
				return reserveErr
			}
			if !validChannel(result) {
				return ErrChannelUnavailable
			}
			return nil
		}
		result, reserveErr = apply(tx, now)
		if reserveErr != nil {
			return reserveErr
		}
		if reserveErr = service.hydrateChannelAssignees(tx, &result); reserveErr != nil {
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
	"assignment_mode": true, "assignment_strategy": true, "overflow_policy": true, "assignment_config_json": true, "assignees": true,
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
	assignees, ownerStaffID, explicitAssignees, err := normalizeChannelAssigneeProjection(values["assignees"], values["assignment_mode"], values["assignment_strategy"])
	if err != nil {
		return nil, err
	}
	if explicitAssignees {
		encoded, marshalErr := json.Marshal(assignees)
		if marshalErr != nil {
			return nil, ErrInvalidChannel
		}
		values["assignees"] = encoded
		owner, marshalErr := json.Marshal(ownerStaffID)
		if marshalErr != nil {
			return nil, ErrInvalidChannel
		}
		values["owner_staff_id"] = owner
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

func (service *ChannelService) validateLocalReferences(ctx context.Context, projection json.RawMessage) error {
	if err := service.validateImageReferences(ctx, projection); err != nil {
		return err
	}
	if err := service.validateMaterialReferences(ctx, projection); err != nil {
		return err
	}
	if err := service.validateEntryTag(ctx, projection); err != nil {
		return err
	}
	_, err := service.assigneesForProjection(ctx, projection)
	return err
}

func (service *ChannelService) validateMaterialReferences(ctx context.Context, projection json.RawMessage) error {
	values, err := object(projection)
	if err != nil {
		return ErrChannelUnavailable
	}
	for _, reference := range []struct {
		key      string
		eligible func(context.Context, int64) (bool, error)
	}{
		{"welcome_attachment_library_ids", func(ctx context.Context, id int64) (bool, error) {
			if service == nil || service.attachments == nil {
				return false, ErrChannelUnavailable
			}
			return service.attachments.ChannelAttachmentEligible(ctx, id)
		}},
		{"welcome_miniprogram_library_ids", func(ctx context.Context, id int64) (bool, error) {
			if service == nil || service.miniprograms == nil {
				return false, ErrChannelUnavailable
			}
			return service.miniprograms.ChannelMiniProgramEligible(ctx, id)
		}},
		{"welcome_group_invite_library_ids", func(ctx context.Context, id int64) (bool, error) {
			if service == nil || service.groupInvites == nil {
				return false, ErrChannelUnavailable
			}
			return service.groupInvites.ChannelGroupInviteEligible(ctx, id)
		}},
	} {
		var ids []int64
		if json.Unmarshal(values[reference.key], &ids) != nil {
			return ErrChannelUnavailable
		}
		for _, id := range ids {
			eligible, lookupErr := reference.eligible(ctx, id)
			if lookupErr != nil {
				return ErrChannelUnavailable
			}
			if !eligible {
				return ErrInvalidChannel
			}
		}
	}
	return nil
}

func (service *ChannelService) validateEntryTag(ctx context.Context, projection json.RawMessage) error {
	values, err := object(projection)
	if err != nil {
		return ErrChannelUnavailable
	}
	var id, name, groupName string
	if json.Unmarshal(values["entry_tag_id"], &id) != nil || json.Unmarshal(values["entry_tag_name"], &name) != nil || json.Unmarshal(values["entry_tag_group_name"], &groupName) != nil {
		return ErrChannelUnavailable
	}
	if id == "" && name == "" && groupName == "" {
		return nil
	}
	if !canonicalPositiveID(id) || name == "" || service == nil || service.tags == nil {
		return ErrInvalidChannel
	}
	tagID, _ := strconv.ParseInt(id, 10, 64)
	tag, err := service.tags.LockActiveTag(ctx, tagID)
	if err != nil {
		if errors.Is(err, contactport.ErrTagReferenceNotFound) {
			return ErrInvalidChannel
		}
		return ErrChannelUnavailable
	}
	if tag.ID != tagID || tag.Name != name {
		return ErrInvalidChannel
	}
	if tag.GroupName == nil {
		if groupName == "" {
			return nil
		}
		return ErrInvalidChannel
	}
	if *tag.GroupName != groupName {
		return ErrInvalidChannel
	}
	return nil
}

func (service *ChannelService) hydrateAssignees(ctx context.Context, channels []Channel) error {
	for index := range channels {
		if err := service.hydrateChannelAssignees(ctx, &channels[index]); err != nil {
			return err
		}
	}
	return nil
}

func (service *ChannelService) hydrateChannelAssignees(ctx context.Context, channel *Channel) error {
	if channel == nil {
		return ErrChannelUnavailable
	}
	assignees, err := service.assigneesForProjection(ctx, channel.LegacyProjection)
	if errors.Is(err, ErrInvalidChannel) {
		return ErrChannelUnavailable
	}
	if err != nil {
		return err
	}
	channel.Assignees = assignees
	return nil
}

type channelAssigneeProjection struct {
	StaffID      string `json:"staff_id"`
	Status       string `json:"status"`
	Priority     int32  `json:"priority"`
	RatioPercent *int32 `json:"ratio_percent,omitempty"`
	MaxScans24h  *int32 `json:"max_scans_24h,omitempty"`
}

func normalizeChannelAssigneeProjection(raw json.RawMessage, modeRaw, strategyRaw json.RawMessage) ([]channelAssigneeProjection, string, bool, error) {
	if len(raw) == 0 {
		return nil, "", false, nil
	}
	var mode, strategy string
	if json.Unmarshal(modeRaw, &mode) != nil || json.Unmarshal(strategyRaw, &strategy) != nil ||
		(mode != "single_owner" && mode != "multi_staff") || (strategy != "ratio" && strategy != "cap_switch") {
		return nil, "", false, ErrInvalidChannel
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil, "", false, ErrInvalidChannel
	}
	result := make([]channelAssigneeProjection, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	activeCount, ratioTotal := 0, int64(0)
	owner := ""
	for index, rawValue := range values {
		fields, err := object(rawValue)
		if err != nil {
			return nil, "", false, ErrInvalidChannel
		}
		for key := range fields {
			if key != "staff_id" && key != "status" && key != "priority" && key != "ratio_percent" && key != "max_scans_24h" && key != "display_name" && key != "display_name_snapshot" {
				return nil, "", false, ErrInvalidChannel
			}
		}
		var item channelAssigneeProjection
		if json.Unmarshal(fields["staff_id"], &item.StaffID) != nil || !validText(item.StaffID, 200) {
			return nil, "", false, ErrInvalidChannel
		}
		if _, duplicate := seen[item.StaffID]; duplicate {
			return nil, "", false, ErrInvalidChannel
		}
		seen[item.StaffID] = struct{}{}
		item.Status = "active"
		if candidate, ok := fields["status"]; ok && json.Unmarshal(candidate, &item.Status) != nil || item.Status != "active" && item.Status != "inactive" && item.Status != "archived" {
			return nil, "", false, ErrInvalidChannel
		}
		item.Priority = int32(index + 1)
		if candidate, ok := fields["priority"]; ok && (json.Unmarshal(candidate, &item.Priority) != nil || item.Priority < 1) {
			return nil, "", false, ErrInvalidChannel
		}
		if item.Status == "active" {
			activeCount++
			if owner == "" {
				owner = item.StaffID
			}
			if strategy == "ratio" {
				var ratio int32
				if candidate, ok := fields["ratio_percent"]; !ok || json.Unmarshal(candidate, &ratio) != nil || ratio < 1 {
					return nil, "", false, ErrInvalidChannel
				}
				item.RatioPercent = &ratio
				ratioTotal += int64(ratio)
			} else {
				var cap int32
				if candidate, ok := fields["max_scans_24h"]; !ok || json.Unmarshal(candidate, &cap) != nil || cap < 1 {
					return nil, "", false, ErrInvalidChannel
				}
				item.MaxScans24h = &cap
			}
		}
		result = append(result, item)
	}
	if activeCount < 1 || activeCount > 5 || mode == "single_owner" && activeCount != 1 || strategy == "ratio" && ratioTotal != 100 {
		return nil, "", false, ErrInvalidChannel
	}
	return result, owner, true, nil
}

func (service *ChannelService) assigneesForProjection(ctx context.Context, projection json.RawMessage) ([]ChannelAssignee, error) {
	values, err := object(projection)
	if err != nil {
		return nil, ErrChannelUnavailable
	}
	var owner string
	if json.Unmarshal(values["owner_staff_id"], &owner) != nil {
		return nil, ErrChannelUnavailable
	}
	configured, primary, explicit, err := normalizeChannelAssigneeProjection(values["assignees"], values["assignment_mode"], values["assignment_strategy"])
	if err != nil {
		return nil, err
	}
	if !explicit {
		if owner == "" {
			return []ChannelAssignee{}, nil
		}
		configured, primary = []channelAssigneeProjection{{StaffID: owner, Status: "active", Priority: 1, RatioPercent: channelInt32(100)}}, owner
	} else if owner != primary {
		return nil, ErrInvalidChannel
	}
	if service == nil || service.staff == nil {
		return nil, ErrChannelUnavailable
	}
	result := make([]ChannelAssignee, 0, len(configured))
	for _, configuredAssignee := range configured {
		if configuredAssignee.Status != "active" {
			continue
		}
		entry, lookupErr := service.staff.LockEligibleStaffByWeComUserID(ctx, configuredAssignee.StaffID)
		if errors.Is(lookupErr, contactport.ErrStaffReferenceNotFound) {
			return nil, ErrInvalidChannel
		}
		if lookupErr != nil || entry.WeComUserID != configuredAssignee.StaffID || !validText(entry.DisplayName, 200) || entry.UpdatedAt.IsZero() {
			return nil, ErrChannelUnavailable
		}
		result = append(result, ChannelAssignee{WeComUserID: entry.WeComUserID, DisplayName: entry.DisplayName, Status: configuredAssignee.Status, Priority: configuredAssignee.Priority, RatioPercent: cloneChannelInt32(configuredAssignee.RatioPercent), MaxScans24h: cloneChannelInt32(configuredAssignee.MaxScans24h)})
	}
	return result, nil
}

func channelInt32(value int32) *int32 { return &value }

func cloneChannelInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func canonicalPositiveID(value string) bool {
	if value == "" || value[0] == '0' {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
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
	if !(v.ID > 0 && validText(v.ChannelCode, 200) && validText(v.ChannelName, 200) && validChannelStatus(v.Status) && v.CreatedBy > 0 && v.UpdatedBy > 0 && !v.CreatedAt.IsZero() && !v.UpdatedAt.Before(v.CreatedAt) && json.Valid(v.LegacyProjection)) {
		return false
	}
	seen := map[string]struct{}{}
	for _, assignee := range v.Assignees {
		if !validText(assignee.WeComUserID, 200) || !validText(assignee.DisplayName, 200) ||
			(assignee.Status != "active" && assignee.Status != "inactive" && assignee.Status != "archived") || assignee.Priority < 1 {
			return false
		}
		if assignee.RatioPercent != nil && *assignee.RatioPercent < 1 || assignee.MaxScans24h != nil && *assignee.MaxScans24h < 1 {
			return false
		}
		if _, duplicate := seen[assignee.WeComUserID]; duplicate {
			return false
		}
		seen[assignee.WeComUserID] = struct{}{}
	}
	return true
}

// validChannelReceiptSnapshot accepts the historical completed-receipt shape.
// Before CH01, a receipt could retain only the stable WeCom user ID and the
// display-name snapshot for each assignee. Replay re-hydrates that local
// projection from the authoritative staff reader, so it must not require
// fields introduced by the current response projection.
func validChannelReceiptSnapshot(v Channel) bool {
	assignees := v.Assignees
	v.Assignees = nil
	if !validChannel(v) {
		return false
	}
	seen := map[string]struct{}{}
	for _, assignee := range assignees {
		if !validText(assignee.WeComUserID, 200) || !validText(assignee.DisplayName, 200) ||
			(assignee.Status != "" && assignee.Status != "active" && assignee.Status != "inactive" && assignee.Status != "archived") || assignee.Priority < 0 ||
			assignee.RatioPercent != nil && *assignee.RatioPercent < 1 || assignee.MaxScans24h != nil && *assignee.MaxScans24h < 1 {
			return false
		}
		if _, duplicate := seen[assignee.WeComUserID]; duplicate {
			return false
		}
		seen[assignee.WeComUserID] = struct{}{}
	}
	return true
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
	v.Assignees = append([]ChannelAssignee(nil), v.Assignees...)
	for index := range v.Assignees {
		v.Assignees[index].RatioPercent = cloneChannelInt32(v.Assignees[index].RatioPercent)
		v.Assignees[index].MaxScans24h = cloneChannelInt32(v.Assignees[index].MaxScans24h)
	}
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
