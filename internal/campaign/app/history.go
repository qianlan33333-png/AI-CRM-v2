package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

// CampaignHistoryWriter persists only non-executable V1 history and its
// same-transaction receipt. It has no current Campaign, queue, event, or
// Provider dependency.
type CampaignHistoryWriter struct {
	store   campaignport.CampaignHistoryStore
	journal campaignport.CampaignHistoryJournal
}

func NewCampaignHistoryWriter(store campaignport.CampaignHistoryStore, journal campaignport.CampaignHistoryJournal) *CampaignHistoryWriter {
	return &CampaignHistoryWriter{store: store, journal: journal}
}

func (writer *CampaignHistoryWriter) WriteSegment(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value campaignport.HistoricalCampaignSegment) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignHistoryReady(writer, ctx) || value.ID != 0 || value.SourcePayloadDigest != payloadDigest || !validHistoricalCampaignSegment(value, false) {
		return campaignHistoryInvalidOrUnavailable(writer, ctx)
	}
	value = normalizeHistoricalCampaignSegment(value)
	return writeCampaignHistory(ctx, writer.journal, "segments", sourceIdentifier, payloadDigest, value,
		func(v campaignport.HistoricalCampaignSegment) int64 { return v.ID },
		func(v campaignport.HistoricalCampaignSegment, id int64) campaignport.HistoricalCampaignSegment {
			v.ID = id
			return normalizeHistoricalCampaignSegment(v)
		},
		HistoricalCampaignSegmentDigest, writer.store.CreateHistoricalCampaignSegment, writer.store.GetHistoricalCampaignSegment)
}

func (writer *CampaignHistoryWriter) WriteMember(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value campaignport.HistoricalCampaignMember) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignHistoryReady(writer, ctx) || value.ID != 0 || value.SourcePayloadDigest != payloadDigest || !validHistoricalCampaignMember(value, false) {
		return campaignHistoryInvalidOrUnavailable(writer, ctx)
	}
	value = normalizeHistoricalCampaignMember(value)
	return writeCampaignHistory(ctx, writer.journal, "members", sourceIdentifier, payloadDigest, value,
		func(v campaignport.HistoricalCampaignMember) int64 { return v.ID },
		func(v campaignport.HistoricalCampaignMember, id int64) campaignport.HistoricalCampaignMember {
			v.ID = id
			return normalizeHistoricalCampaignMember(v)
		},
		HistoricalCampaignMemberDigest, writer.store.CreateHistoricalCampaignMember, writer.store.GetHistoricalCampaignMember)
}

func (writer *CampaignHistoryWriter) WritePlan(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value campaignport.HistoricalBroadcastPlan) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignHistoryReady(writer, ctx) || value.ID != 0 || value.SourcePayloadDigest != payloadDigest || !validHistoricalBroadcastPlan(value, false) {
		return campaignHistoryInvalidOrUnavailable(writer, ctx)
	}
	value = normalizeHistoricalBroadcastPlan(value)
	return writeCampaignHistory(ctx, writer.journal, "plans", sourceIdentifier, payloadDigest, value,
		func(v campaignport.HistoricalBroadcastPlan) int64 { return v.ID },
		func(v campaignport.HistoricalBroadcastPlan, id int64) campaignport.HistoricalBroadcastPlan {
			v.ID = id
			return normalizeHistoricalBroadcastPlan(v)
		},
		HistoricalBroadcastPlanDigest, writer.store.CreateHistoricalBroadcastPlan, writer.store.GetHistoricalBroadcastPlan)
}

func (writer *CampaignHistoryWriter) WriteRecipient(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value campaignport.HistoricalBroadcastRecipient) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignHistoryReady(writer, ctx) || value.ID != 0 || value.SourcePayloadDigest != payloadDigest || !validHistoricalBroadcastRecipient(value, false) {
		return campaignHistoryInvalidOrUnavailable(writer, ctx)
	}
	value = normalizeHistoricalBroadcastRecipient(value)
	return writeCampaignHistory(ctx, writer.journal, "recipients", sourceIdentifier, payloadDigest, value,
		func(v campaignport.HistoricalBroadcastRecipient) int64 { return v.ID },
		func(v campaignport.HistoricalBroadcastRecipient, id int64) campaignport.HistoricalBroadcastRecipient {
			v.ID = id
			return normalizeHistoricalBroadcastRecipient(v)
		},
		HistoricalBroadcastRecipientDigest, writer.store.CreateHistoricalBroadcastRecipient, writer.store.GetHistoricalBroadcastRecipient)
}

func (writer *CampaignHistoryWriter) WriteMessage(ctx context.Context, sourceIdentifier string, payloadDigest [sha256.Size]byte, value campaignport.HistoricalBroadcastMessage) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignHistoryReady(writer, ctx) || value.ID != 0 || value.SourcePayloadDigest != payloadDigest || !validHistoricalBroadcastMessage(value, false) {
		return campaignHistoryInvalidOrUnavailable(writer, ctx)
	}
	value = normalizeHistoricalBroadcastMessage(value)
	return writeCampaignHistory(ctx, writer.journal, "messages", sourceIdentifier, payloadDigest, value,
		func(v campaignport.HistoricalBroadcastMessage) int64 { return v.ID },
		func(v campaignport.HistoricalBroadcastMessage, id int64) campaignport.HistoricalBroadcastMessage {
			v.ID = id
			return normalizeHistoricalBroadcastMessage(v)
		},
		HistoricalBroadcastMessageDigest, writer.store.CreateHistoricalBroadcastMessage, writer.store.GetHistoricalBroadcastMessage)
}

// HistoricalCampaignSegmentDigest includes every stored field, including the
// generated target ID, so a replay cannot normalize away target drift.
func HistoricalCampaignSegmentDigest(value campaignport.HistoricalCampaignSegment) ([sha256.Size]byte, error) {
	if !validHistoricalCampaignSegment(value, true) {
		return [sha256.Size]byte{}, campaignport.ErrCampaignHistoryInvalid
	}
	return campaignHistoryDigest("segment", normalizeHistoricalCampaignSegment(value))
}

func HistoricalCampaignMemberDigest(value campaignport.HistoricalCampaignMember) ([sha256.Size]byte, error) {
	if !validHistoricalCampaignMember(value, true) {
		return [sha256.Size]byte{}, campaignport.ErrCampaignHistoryInvalid
	}
	return campaignHistoryDigest("member", normalizeHistoricalCampaignMember(value))
}

func HistoricalBroadcastPlanDigest(value campaignport.HistoricalBroadcastPlan) ([sha256.Size]byte, error) {
	if !validHistoricalBroadcastPlan(value, true) {
		return [sha256.Size]byte{}, campaignport.ErrCampaignHistoryInvalid
	}
	return campaignHistoryDigest("plan", normalizeHistoricalBroadcastPlan(value))
}

func HistoricalBroadcastRecipientDigest(value campaignport.HistoricalBroadcastRecipient) ([sha256.Size]byte, error) {
	if !validHistoricalBroadcastRecipient(value, true) {
		return [sha256.Size]byte{}, campaignport.ErrCampaignHistoryInvalid
	}
	return campaignHistoryDigest("recipient", normalizeHistoricalBroadcastRecipient(value))
}

func HistoricalBroadcastMessageDigest(value campaignport.HistoricalBroadcastMessage) ([sha256.Size]byte, error) {
	if !validHistoricalBroadcastMessage(value, true) {
		return [sha256.Size]byte{}, campaignport.ErrCampaignHistoryInvalid
	}
	return campaignHistoryDigest("message", normalizeHistoricalBroadcastMessage(value))
}

// This private helper shares only the identical receipt/replay protocol; it
// does not relax the distinct validation of the five history facts.
func writeCampaignHistory[T any](
	ctx context.Context, journal campaignport.CampaignHistoryJournal, kind, source string, payload [sha256.Size]byte, value T,
	id func(T) int64, withID func(T, int64) T, digest func(T) ([sha256.Size]byte, error),
	create func(context.Context, T) (T, error), get func(context.Context, int64) (T, error),
) (campaignport.CampaignHistoryReceipt, error) {
	empty := campaignport.CampaignHistoryReceipt{}
	if !validCampaignHistorySourceIdentifier(source) || payload == ([sha256.Size]byte{}) {
		return empty, campaignport.ErrCampaignHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, campaignport.ErrCampaignHistoryInvalid
	}
	receipt, found, err := journal.LoadCampaignHistory(ctx, kind, source)
	if err != nil {
		return empty, campaignHistoryError(err)
	}
	if found {
		if receipt.SourceIdentifier != source || receipt.PayloadDigest != payload || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
			return empty, campaignport.ErrCampaignHistoryConflict
		}
		actual, getErr := get(ctx, receipt.TargetID)
		if getErr != nil {
			return empty, campaignHistoryError(getErr)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || id(actual) != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, campaignport.ErrCampaignHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create(ctx, withID(value, 0))
	if err != nil {
		return empty, campaignHistoryError(err)
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, id(actual)))
	if actualErr != nil || expectedErr != nil || id(actual) < 1 || actualDigest != expectedDigest {
		return empty, campaignport.ErrCampaignHistoryConflict
	}
	receipt = campaignport.CampaignHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: id(actual), TargetDigest: actualDigest}
	if err = journal.RecordCampaignHistory(ctx, kind, receipt); err != nil {
		return empty, campaignHistoryError(err)
	}
	return receipt, nil
}

func validHistoricalCampaignSegment(value campaignport.HistoricalCampaignSegment, stored bool) bool {
	return validCampaignHistoryID(value.ID, stored) && value.SourceID > 0 && value.CampaignSourceID > 0 &&
		(value.SourceParentState == "observed" || value.SourceParentState == "missing_campaign") &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && validCampaignHistoryText(value.Code, value.Label) && validCampaignHistoryTime(value.CreatedAt, stored)
}

func validHistoricalCampaignMember(value campaignport.HistoricalCampaignMember, stored bool) bool {
	return validCampaignHistoryID(value.ID, stored) && value.SourceID > 0 && value.SegmentHistoryID > 0 && validCampaignHistoryOptionalID(value.CustomerID) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && validCampaignHistoryText(value.AnchorDate, value.OriginalStatus, value.StopReason) &&
		validCampaignHistoryTime(value.JoinedAt, stored) && validCampaignHistoryOptionalTime(value.NextDueAt, stored) &&
		validCampaignHistoryOptionalTime(value.LastStepSentAt, stored) && validCampaignHistoryTime(value.CreatedAt, stored) && validCampaignHistoryTime(value.UpdatedAt, stored)
}

func validHistoricalBroadcastPlan(value campaignport.HistoricalBroadcastPlan, stored bool) bool {
	return validCampaignHistoryID(value.ID, stored) && value.SourceID > 0 && value.SourcePlanID != "" &&
		value.RuntimeDigest != ([sha256.Size]byte{}) && value.SourcePayloadDigest != ([sha256.Size]byte{}) &&
		validCampaignHistoryText(value.SourcePlanID, value.DisplayName, value.Intent, value.ContentStrategy, value.ContentTemplateMasked, value.OriginalStatus, value.OriginalReviewStatus, value.OriginalRunStatus) &&
		validCampaignHistoryOptionalTime(value.CommittedAt, stored) && validCampaignHistoryOptionalTime(value.ExpiresAt, stored) && validCampaignHistoryTime(value.CreatedAt, stored) && validCampaignHistoryTime(value.UpdatedAt, stored)
}

func validHistoricalBroadcastRecipient(value campaignport.HistoricalBroadcastRecipient, stored bool) bool {
	return validCampaignHistoryID(value.ID, stored) && value.SourceID > 0 && value.PlanHistoryID > 0 && validCampaignHistoryOptionalID(value.CustomerID) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && validCampaignHistoryText(value.DisplayName, value.OriginalApprovalStatus, value.OriginalSendStatus) &&
		validCampaignHistoryOptionalTime(value.ApprovedAt, stored) && validCampaignHistoryOptionalTime(value.RejectedAt, stored) && validCampaignHistoryTime(value.CreatedAt, stored) && validCampaignHistoryTime(value.UpdatedAt, stored)
}

func validHistoricalBroadcastMessage(value campaignport.HistoricalBroadcastMessage, stored bool) bool {
	return validCampaignHistoryID(value.ID, stored) && value.SourceID > 0 && value.PlanHistoryID > 0 && value.RecipientHistoryID > 0 && validCampaignHistoryOptionalID(value.CustomerID) &&
		value.ContentPayloadDigest != ([sha256.Size]byte{}) && value.AttachmentsDigest != ([sha256.Size]byte{}) && value.SourcePayloadDigest != ([sha256.Size]byte{}) &&
		validCampaignHistoryText(value.OriginalSendTime, value.ContentMasked, value.OriginalStatus) && validCampaignHistoryOptionalTime(value.SentAt, stored) &&
		validCampaignHistoryTime(value.CreatedAt, stored) && validCampaignHistoryTime(value.UpdatedAt, stored)
}

func normalizeHistoricalCampaignSegment(value campaignport.HistoricalCampaignSegment) campaignport.HistoricalCampaignSegment {
	value.CreatedAt = normalizeCampaignHistoryTime(value.CreatedAt)
	return value
}

func normalizeHistoricalCampaignMember(value campaignport.HistoricalCampaignMember) campaignport.HistoricalCampaignMember {
	value.JoinedAt, value.CreatedAt, value.UpdatedAt = normalizeCampaignHistoryTime(value.JoinedAt), normalizeCampaignHistoryTime(value.CreatedAt), normalizeCampaignHistoryTime(value.UpdatedAt)
	value.NextDueAt, value.LastStepSentAt, value.CustomerID = normalizeCampaignHistoryTimePointer(value.NextDueAt), normalizeCampaignHistoryTimePointer(value.LastStepSentAt), cloneCampaignHistoryID(value.CustomerID)
	return value
}

func normalizeHistoricalBroadcastPlan(value campaignport.HistoricalBroadcastPlan) campaignport.HistoricalBroadcastPlan {
	value.CommittedAt, value.ExpiresAt = normalizeCampaignHistoryTimePointer(value.CommittedAt), normalizeCampaignHistoryTimePointer(value.ExpiresAt)
	value.CreatedAt, value.UpdatedAt = normalizeCampaignHistoryTime(value.CreatedAt), normalizeCampaignHistoryTime(value.UpdatedAt)
	value.CampaignSourceID, value.SegmentSourceID = cloneCampaignHistoryID(value.CampaignSourceID), cloneCampaignHistoryID(value.SegmentSourceID)
	return value
}

func normalizeHistoricalBroadcastRecipient(value campaignport.HistoricalBroadcastRecipient) campaignport.HistoricalBroadcastRecipient {
	value.ApprovedAt, value.RejectedAt = normalizeCampaignHistoryTimePointer(value.ApprovedAt), normalizeCampaignHistoryTimePointer(value.RejectedAt)
	value.CreatedAt, value.UpdatedAt, value.CustomerID = normalizeCampaignHistoryTime(value.CreatedAt), normalizeCampaignHistoryTime(value.UpdatedAt), cloneCampaignHistoryID(value.CustomerID)
	return value
}

func normalizeHistoricalBroadcastMessage(value campaignport.HistoricalBroadcastMessage) campaignport.HistoricalBroadcastMessage {
	value.SentAt = normalizeCampaignHistoryTimePointer(value.SentAt)
	value.CreatedAt, value.UpdatedAt, value.CustomerID = normalizeCampaignHistoryTime(value.CreatedAt), normalizeCampaignHistoryTime(value.UpdatedAt), cloneCampaignHistoryID(value.CustomerID)
	return value
}

func campaignHistoryReady(writer *CampaignHistoryWriter, ctx context.Context) bool {
	return writer != nil && ctx != nil && ctx.Err() == nil && !campaignHistoryNil(writer.store) && !campaignHistoryNil(writer.journal)
}

func campaignHistoryInvalidOrUnavailable(writer *CampaignHistoryWriter, ctx context.Context) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignHistoryReady(writer, ctx) {
		return campaignport.CampaignHistoryReceipt{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return campaignport.CampaignHistoryReceipt{}, campaignport.ErrCampaignHistoryInvalid
}

func campaignHistoryNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func validCampaignHistorySourceIdentifier(value string) bool {
	if len(value) != hex.EncodedLen(sha256.Size) || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && !allCampaignHistoryZero(decoded)
}

func allCampaignHistoryZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}

func validCampaignHistoryID(value int64, stored bool) bool {
	return (stored && value > 0) || (!stored && value == 0)
}

func validCampaignHistoryOptionalID(value *int64) bool { return value == nil || *value > 0 }

func validCampaignHistoryText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return false
		}
	}
	return true
}

func validCampaignHistoryTime(value time.Time, _ bool) bool {
	return !value.IsZero()
}

func validCampaignHistoryOptionalTime(value *time.Time, canonical bool) bool {
	return value == nil || validCampaignHistoryTime(*value, canonical)
}

func normalizeCampaignHistoryTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func normalizeCampaignHistoryTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := normalizeCampaignHistoryTime(*value)
	return &copy
}

func cloneCampaignHistoryID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func campaignHistoryDigest(kind string, value any) ([sha256.Size]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{Kind: kind, Value: value})
	if err != nil {
		return [sha256.Size]byte{}, campaignport.ErrCampaignHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func campaignHistoryError(err error) error {
	switch {
	case errors.Is(err, campaignport.ErrCampaignHistoryInvalid):
		return campaignport.ErrCampaignHistoryInvalid
	case errors.Is(err, campaignport.ErrCampaignHistoryConflict):
		return campaignport.ErrCampaignHistoryConflict
	default:
		return campaignport.ErrCampaignHistoryUnavailable
	}
}
