package v1domain

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type channelContactJSON struct {
	ID                    *int64          `json:"id"`
	ChannelID             *int64          `json:"channel_id"`
	UnionID               json.RawMessage `json:"unionid"`
	OwnerStaffID          *string         `json:"owner_staff_id"`
	FirstChannelEnteredAt *time.Time      `json:"first_channel_entered_at"`
	LastChannelEnteredAt  *time.Time      `json:"last_channel_entered_at"`
	EnterCount            *int32          `json:"enter_count"`
	CreatedAt             *time.Time      `json:"created_at"`
	UpdatedAt             *time.Time      `json:"updated_at"`
}

type channelAssigneeJSON struct {
	ID                  *int64          `json:"id"`
	ChannelID           *int64          `json:"channel_id"`
	StaffID             *string         `json:"staff_id"`
	DisplayNameSnapshot *string         `json:"display_name_snapshot"`
	Priority            *int32          `json:"priority"`
	RatioPercent        json.RawMessage `json:"ratio_percent"`
	MaxScans24h         json.RawMessage `json:"max_scans_24h"`
	Status              *string         `json:"status"`
	CreatedAt           *string         `json:"created_at"`
	UpdatedAt           *string         `json:"updated_at"`
}

func (importer *ChannelImporter) importContact(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, channels map[int64]int64) channelRowOutcome {
	value, unionID, sourceChannelID, reason := channelContactDefinition(row)
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		if value == nil {
			value, err := recordChannelTerminal(tx, journal, row, "quarantine", reason)
			replayed = value
			return err
		}
		channelID, found := channels[sourceChannelID]
		if !found {
			value, err := recordChannelTerminal(tx, journal, row, "quarantine", "missing_channel_definition")
			replayed = value
			return err
		}
		value.Contact.ChannelID = channelID
		if unionID != "" {
			customerID, err := importer.resolver.ResolveHistoricalChannelCustomer(tx, unionID)
			if err != nil {
				return err
			}
			value.Contact.CustomerID = customerID
		}
		receipt, err := importer.relations.ImportContact(tx, *value)
		if err != nil {
			return err
		}
		if !sameHistoricalChannelContactReceipt(receipt, *value) {
			return ErrConflict
		}
		recorded, found, err := journal.LoadTerminal(tx, value.SourceIdentifier)
		if err != nil || !found || !sameHistoricalChannelTerminal(recorded, receipt) {
			return ErrConflict
		}
		replayed = receipt.Replayed
		return nil
	})
	if err != nil {
		return channelRowOutcome{err: err}
	}
	if value == nil || !hasChannel(channels, sourceChannelID) {
		return channelRowOutcome{quarantined: 1, replayed: replayed}
	}
	return channelRowOutcome{imported: 1, replayed: replayed}
}

func (importer *ChannelImporter) importAssignee(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, channels map[int64]int64) channelRowOutcome {
	value, sourceChannelID, reason := channelAssigneeDefinition(row)
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		if value == nil {
			value, err := recordChannelTerminal(tx, journal, row, "quarantine", reason)
			replayed = value
			return err
		}
		channelID, found := channels[sourceChannelID]
		if !found {
			value, err := recordChannelTerminal(tx, journal, row, "quarantine", "missing_channel_definition")
			replayed = value
			return err
		}
		value.Assignee.ChannelID = channelID
		receipt, err := importer.relations.ImportAssignee(tx, *value)
		if err != nil {
			return err
		}
		if !sameHistoricalChannelAssigneeReceipt(receipt, *value) {
			return ErrConflict
		}
		recorded, found, err := journal.LoadTerminal(tx, value.SourceIdentifier)
		if err != nil || !found || !sameHistoricalChannelTerminal(recorded, receipt) {
			return ErrConflict
		}
		replayed = receipt.Replayed
		return nil
	})
	if err != nil {
		return channelRowOutcome{err: err}
	}
	if value == nil || !hasChannel(channels, sourceChannelID) {
		return channelRowOutcome{quarantined: 1, replayed: replayed}
	}
	return channelRowOutcome{imported: 1, replayed: replayed}
}

func channelContactDefinition(row v1archive.ArchivedRow) (*contactport.HistoricalChannelContactDefinition, string, int64, string) {
	if redactedChannelContact(row.RedactedFields) {
		return nil, "", 0, "redacted_channel_contact"
	}
	var source channelContactJSON
	if json.Unmarshal(row.Payload, &source) != nil || source.ID == nil || source.ChannelID == nil || source.OwnerStaffID == nil ||
		source.FirstChannelEnteredAt == nil || source.LastChannelEnteredAt == nil || source.EnterCount == nil || source.CreatedAt == nil || source.UpdatedAt == nil {
		return nil, "", 0, "invalid_channel_contact"
	}
	unionID, ok := channelOptionalSourceText(source.UnionID)
	if !ok || *source.ID < 1 || *source.ChannelID < 1 || *source.EnterCount < 1 || source.FirstChannelEnteredAt.IsZero() || source.LastChannelEnteredAt.IsZero() ||
		source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() || source.LastChannelEnteredAt.Before(*source.FirstChannelEnteredAt) || source.UpdatedAt.Before(*source.CreatedAt) {
		return nil, "", 0, "invalid_channel_contact"
	}
	return &contactport.HistoricalChannelContactDefinition{SourceIdentifier: SourceIdentifier(row.SourceKeyHMAC), PayloadDigest: row.PayloadHMAC,
		Contact: contactport.HistoricalChannelContact{SourceContactID: *source.ID, OwnerReference: *source.OwnerStaffID,
			FirstEnteredAt: *source.FirstChannelEnteredAt, LastEnteredAt: *source.LastChannelEnteredAt, EnterCount: *source.EnterCount,
			CreatedAt: *source.CreatedAt, UpdatedAt: *source.UpdatedAt}}, unionID, *source.ChannelID, ""
}

func channelAssigneeDefinition(row v1archive.ArchivedRow) (*contactport.HistoricalChannelAssigneeDefinition, int64, string) {
	if redactedChannelAssignee(row.RedactedFields) {
		return nil, 0, "redacted_channel_assignee"
	}
	var source channelAssigneeJSON
	if json.Unmarshal(row.Payload, &source) != nil || source.ID == nil || source.ChannelID == nil || source.StaffID == nil || source.DisplayNameSnapshot == nil ||
		source.Priority == nil || source.Status == nil || source.CreatedAt == nil || source.UpdatedAt == nil {
		return nil, 0, "invalid_channel_assignee"
	}
	ratio, ratioOK := channelOptionalInt32(source.RatioPercent)
	maxScans, maxScansOK := channelOptionalInt32(source.MaxScans24h)
	created, createdOK := channelCivilTime(*source.CreatedAt)
	updated, updatedOK := channelCivilTime(*source.UpdatedAt)
	if !ratioOK || !maxScansOK || !createdOK || !updatedOK || *source.ID < 1 || *source.ChannelID < 1 || *source.Priority < 0 || *source.Status == "" ||
		(ratio != nil && (*ratio < 0 || *ratio > 100)) || (maxScans != nil && *maxScans < 0) || updated < created {
		return nil, 0, "invalid_channel_assignee"
	}
	return &contactport.HistoricalChannelAssigneeDefinition{SourceIdentifier: SourceIdentifier(row.SourceKeyHMAC), PayloadDigest: row.PayloadHMAC,
		Assignee: contactport.HistoricalChannelAssignee{SourceAssigneeID: *source.ID, StaffReference: *source.StaffID, DisplayNameSnapshot: *source.DisplayNameSnapshot,
			Priority: *source.Priority, RatioPercent: ratio, MaxScans24h: maxScans, Status: *source.Status, SourceCreatedAt: created, SourceUpdatedAt: updated}}, *source.ChannelID, ""
}

func sameHistoricalChannelContactReceipt(receipt contactport.HistoricalChannelReceipt, definition contactport.HistoricalChannelContactDefinition) bool {
	return receipt.SourceIdentifier == definition.SourceIdentifier && receipt.PayloadDigest == definition.PayloadDigest && receipt.TargetID > 0 && receipt.TargetDigest != [32]byte{}
}

func sameHistoricalChannelAssigneeReceipt(receipt contactport.HistoricalChannelReceipt, definition contactport.HistoricalChannelAssigneeDefinition) bool {
	return receipt.SourceIdentifier == definition.SourceIdentifier && receipt.PayloadDigest == definition.PayloadDigest && receipt.TargetID > 0 && receipt.TargetDigest != [32]byte{}
}

func channelOptionalSourceText(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	if string(raw) == "null" {
		return "", true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || value != strings.TrimSpace(value) {
		return "", false
	}
	return value, true
}

func channelOptionalInt32(raw json.RawMessage) (*int32, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	if string(raw) == "null" {
		return nil, true
	}
	var value int32
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	return &value, true
}

func channelCivilTime(value string) (string, bool) {
	if value == "" || strings.ContainsAny(value, "Zz") || len(value) < len("2006-01-02T15:04:05") || strings.Contains(value[10:], "+") || strings.Contains(value[10:], "-") {
		return "", false
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts) == 2 && (len(parts[1]) == 0 || len(parts[1]) > 6) {
		return "", false
	}
	parsed, err := time.Parse("2006-01-02T15:04:05", value)
	if err != nil {
		return "", false
	}
	return parsed.Format("2006-01-02T15:04:05.000000"), true
}

func redactedChannelContact(fields []string) bool {
	return channelRedacted(fields, "id", "channel_id", "unionid", "union_id", "owner_staff_id", "first_channel_entered_at", "last_channel_entered_at", "enter_count", "created_at", "updated_at")
}

func redactedChannelAssignee(fields []string) bool {
	return channelRedacted(fields, "id", "channel_id", "staff_id", "display_name_snapshot", "priority", "ratio_percent", "max_scans_24h", "status", "created_at", "updated_at")
}

func channelRedacted(fields []string, required ...string) bool {
	for _, field := range fields {
		for _, retained := range required {
			if field == retained {
				return true
			}
		}
	}
	return false
}

func hasChannel(channels map[int64]int64, sourceID int64) bool {
	_, found := channels[sourceID]
	return found
}
