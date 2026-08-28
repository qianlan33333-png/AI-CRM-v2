// Package v1campaigndefinitionhistory decodes authenticated V1 Campaign
// definitions and steps as inert historical facts. It has no store, writer,
// lifecycle, queue, or Provider dependency.
package v1campaigndefinitionhistory

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	DefinitionTableID = "public/campaigns"
	StepTableID       = "public/campaign_steps"
)

var (
	ErrArchiveRow = errors.New("campaign definition archive row invalid")
	ErrFact       = errors.New("campaign definition historical fact invalid")
)

// OpaqueDigest binds private V1 material to the immutable archive without
// retaining recoverable actors, tokens, JSON configuration, or runtime data.
type OpaqueDigest [sha256.Size]byte

// SourceEnvelope is archive provenance, not a V2 identifier or a source key.
type SourceEnvelope struct {
	SourceKeyDigest OpaqueDigest
	PayloadDigest   OpaqueDigest
	FieldDigest     OpaqueDigest
}

// DefinitionFact preserves a V1 definition exactly as a non-executable
// observation. Its status, code and timestamps are never validated against
// current Campaign lifecycle rules.
type DefinitionFact struct {
	SourceID int64
	Source   SourceEnvelope

	Code         string
	DisplayName  string
	Intent       string
	AnchorMode   string
	AnchorDate   string
	ReviewStatus string
	RunStatus    string

	ApprovedAt   *time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
	PausedAt     *time.Time
	PausedReason string
	CreatedAt    time.Time
	UpdatedAt    time.Time

	PrivateDigest OpaqueDigest
	RedactedRoots []string
}

// StepFact preserves a V1 step source reference and display-safe content. Its
// campaign and segment IDs remain V1 references until a later importer proves
// a target relation.
type StepFact struct {
	SourceID         int64
	CampaignSourceID int64
	SegmentSourceID  int64
	Source           SourceEnvelope
	StepIndex        int32
	DayOffset        int32
	SendTime         string
	Timezone         string
	ContentMasked    string
	StopOnReply      bool
	SkipRecentDays   int32
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ContentDigest    OpaqueDigest
	PrivateDigest    OpaqueDigest
	RedactedRoots    []string
}

type definitionJSON struct {
	ID                int64           `json:"id"`
	CampaignCode      string          `json:"campaign_code"`
	DisplayName       string          `json:"display_name"`
	Intent            string          `json:"intent"`
	AnchorMode        string          `json:"anchor_mode"`
	AnchorDate        string          `json:"anchor_date"`
	ReviewStatus      string          `json:"review_status"`
	RunStatus         string          `json:"run_status"`
	CreatedByAgent    string          `json:"created_by_agent"`
	CreatedBySession  string          `json:"created_by_session"`
	TraceID           string          `json:"trace_id"`
	OwnerUserID       string          `json:"owner_userid"`
	ApprovalTokenHash string          `json:"approval_token_hash"`
	ApprovedBy        string          `json:"approved_by"`
	ApprovedAt        *time.Time      `json:"approved_at"`
	StartedAt         *time.Time      `json:"started_at"`
	FinishedAt        *time.Time      `json:"finished_at"`
	PausedAt          *time.Time      `json:"paused_at"`
	PausedReason      string          `json:"paused_reason"`
	Metadata          json.RawMessage `json:"metadata_json"`
	Stats             json.RawMessage `json:"stats_json"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type stepJSON struct {
	ID                      int64           `json:"id"`
	CampaignID              int64           `json:"campaign_id"`
	CampaignSegmentID       int64           `json:"campaign_segment_id"`
	StepIndex               int32           `json:"step_index"`
	DayOffset               int32           `json:"day_offset"`
	SendTime                string          `json:"send_time"`
	Timezone                string          `json:"timezone"`
	ContentText             string          `json:"content_text"`
	ContentPayload          json.RawMessage `json:"content_payload_json"`
	StopOnReply             bool            `json:"stop_on_reply"`
	SkipRecentlyTouchedDays int32           `json:"skip_if_recently_touched_days"`
	AgentRunID              string          `json:"agent_run_id"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

var definitionFields = []string{
	"id", "campaign_code", "display_name", "intent", "anchor_mode", "anchor_date", "review_status", "run_status",
	"created_by_agent", "created_by_session", "trace_id", "owner_userid", "approval_token_hash", "approved_by",
	"approved_at", "started_at", "finished_at", "paused_at", "paused_reason", "metadata_json", "stats_json", "created_at", "updated_at",
}

var stepFields = []string{
	"id", "campaign_id", "campaign_segment_id", "step_index", "day_offset", "send_time", "timezone", "content_text",
	"content_payload_json", "stop_on_reply", "skip_if_recently_touched_days", "agent_run_id", "created_at", "updated_at",
}

// AdaptDefinition validates the immutable archive envelope and returns a
// display-safe, non-executable V1 definition. It intentionally accepts source
// IDs, statuses and temporal ordering that current Campaigns reject.
func AdaptDefinition(row v1archive.ArchivedRow, sourceHMACKey []byte) (DefinitionFact, error) {
	fields, source, err := archiveFields(row, DefinitionTableID, sourceHMACKey)
	if err != nil {
		return DefinitionFact{}, err
	}
	var value definitionJSON
	if err := decodeExact(fields, row.Payload, &value, definitionFields,
		map[string]bool{"approved_at": true, "started_at": true, "finished_at": true, "paused_at": true},
		map[string]bool{"metadata_json": true, "stats_json": true}); err != nil {
		return DefinitionFact{}, err
	}
	if err := verifySourceID(sourceHMACKey, DefinitionTableID, value.ID, row.SourceKeyHMAC); err != nil {
		return DefinitionFact{}, err
	}
	code, err := maskText(value.CampaignCode)
	if err != nil {
		return DefinitionFact{}, err
	}
	displayName, err := maskText(value.DisplayName)
	if err != nil {
		return DefinitionFact{}, err
	}
	intent, err := maskText(value.Intent)
	if err != nil {
		return DefinitionFact{}, err
	}
	anchorMode, err := maskText(value.AnchorMode)
	if err != nil {
		return DefinitionFact{}, err
	}
	anchorDate, err := maskText(value.AnchorDate)
	if err != nil {
		return DefinitionFact{}, err
	}
	reviewStatus, err := maskText(value.ReviewStatus)
	if err != nil {
		return DefinitionFact{}, err
	}
	runStatus, err := maskText(value.RunStatus)
	if err != nil {
		return DefinitionFact{}, err
	}
	pausedReason, err := maskText(value.PausedReason)
	if err != nil {
		return DefinitionFact{}, err
	}
	private, err := opaqueDigest(sourceHMACKey, "definition-private", DefinitionTableID, row.Payload)
	if err != nil {
		return DefinitionFact{}, err
	}
	return DefinitionFact{
		SourceID: value.ID, Source: source,
		Code: code, DisplayName: displayName, Intent: intent, AnchorMode: anchorMode, AnchorDate: anchorDate,
		ReviewStatus: reviewStatus, RunStatus: runStatus, ApprovedAt: value.ApprovedAt, StartedAt: value.StartedAt,
		FinishedAt: value.FinishedAt, PausedAt: value.PausedAt, PausedReason: pausedReason,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, PrivateDigest: private, RedactedRoots: redactionRoots(row.RedactedFields),
	}, nil
}

// AdaptStep validates the immutable archive envelope and returns an inert V1
// step. It neither resolves source parents nor normalizes its schedule.
func AdaptStep(row v1archive.ArchivedRow, sourceHMACKey []byte) (StepFact, error) {
	fields, source, err := archiveFields(row, StepTableID, sourceHMACKey)
	if err != nil {
		return StepFact{}, err
	}
	var value stepJSON
	if err := decodeExact(fields, row.Payload, &value, stepFields, nil, map[string]bool{"content_payload_json": true}); err != nil {
		return StepFact{}, err
	}
	if err := verifySourceID(sourceHMACKey, StepTableID, value.ID, row.SourceKeyHMAC); err != nil {
		return StepFact{}, err
	}
	sendTime, err := maskText(value.SendTime)
	if err != nil {
		return StepFact{}, err
	}
	timezone, err := maskText(value.Timezone)
	if err != nil {
		return StepFact{}, err
	}
	content, err := maskText(value.ContentText)
	if err != nil {
		return StepFact{}, err
	}
	contentDigest, err := opaqueDigest(sourceHMACKey, "step-content", StepTableID, fields["content_text"])
	if err != nil {
		return StepFact{}, err
	}
	private, err := opaqueDigest(sourceHMACKey, "step-private", StepTableID, row.Payload)
	if err != nil {
		return StepFact{}, err
	}
	return StepFact{
		SourceID: value.ID, CampaignSourceID: value.CampaignID, SegmentSourceID: value.CampaignSegmentID, Source: source,
		StepIndex: value.StepIndex, DayOffset: value.DayOffset, SendTime: sendTime, Timezone: timezone,
		ContentMasked: content, StopOnReply: value.StopOnReply, SkipRecentDays: value.SkipRecentlyTouchedDays,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, ContentDigest: contentDigest, PrivateDigest: private, RedactedRoots: redactionRoots(row.RedactedFields),
	}, nil
}

func archiveFields(row v1archive.ArchivedRow, tableID string, sourceHMACKey []byte) (map[string]json.RawMessage, SourceEnvelope, error) {
	zero := [sha256.Size]byte{}
	if len(sourceHMACKey) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal < 1 ||
		row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	canonical, paths, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) || !sameStrings(paths, row.RedactedFields) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	sourceTable := strings.TrimPrefix(tableID, "public/")
	payload, err := v1archive.PayloadHMAC(sourceHMACKey, sourceTable, row.Payload)
	if err != nil || payload != row.PayloadHMAC {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	field, err := v1archive.FieldHMAC(sourceHMACKey, sourceTable, row.RedactedFields)
	if err != nil || field != row.FieldHMAC {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, SourceEnvelope{}, ErrFact
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, SourceEnvelope{}, ErrFact
	}
	return fields, SourceEnvelope{
		SourceKeyDigest: OpaqueDigest(row.SourceKeyHMAC),
		PayloadDigest:   OpaqueDigest(row.PayloadHMAC),
		FieldDigest:     OpaqueDigest(row.FieldHMAC),
	}, nil
}

func decodeExact(fields map[string]json.RawMessage, payload []byte, target any, names []string, nullable, jsonMayBeNull map[string]bool) error {
	if len(fields) != len(names) {
		return ErrFact
	}
	for _, name := range names {
		raw, found := fields[name]
		if !found || !json.Valid(raw) || (!nullable[name] && !jsonMayBeNull[name] && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return ErrFact
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(target); err != nil {
		return ErrFact
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrFact
	}
	return nil
}

func verifySourceID(sourceHMACKey []byte, tableID string, sourceID int64, actual [sha256.Size]byte) error {
	key := []byte("[" + strconv.FormatInt(sourceID, 10) + "]")
	expected, err := v1archive.SourceKeyHMAC(sourceHMACKey, strings.TrimPrefix(tableID, "public/"), key)
	if err != nil || !hmac.Equal(expected[:], actual[:]) {
		return ErrArchiveRow
	}
	return nil
}

func opaqueDigest(key []byte, purpose, table string, value []byte) (OpaqueDigest, error) {
	if len(key) < sha256.Size || purpose == "" || table == "" || !json.Valid(value) {
		return OpaqueDigest{}, ErrArchiveRow
	}
	mac := hmac.New(sha256.New, key)
	for _, part := range [][]byte{[]byte("aicrm/v1-campaign-definition-history/" + purpose + "/v1"), []byte(table), value} {
		var length [4]byte
		length[0] = byte(len(part) >> 24)
		length[1] = byte(len(part) >> 16)
		length[2] = byte(len(part) >> 8)
		length[3] = byte(len(part))
		_, _ = mac.Write(length[:])
		_, _ = mac.Write(part)
	}
	var result OpaqueDigest
	copy(result[:], mac.Sum(nil))
	return result, nil
}

// maskText follows the existing historical display rule. It masks continuous
// Chinese mobile numbers while preserving all other source text, including
// empty strings and line breaks. Invalid UTF-8 and NUL cannot be rendered.
func maskText(value string) (string, error) {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", ErrFact
	}
	var masked strings.Builder
	masked.Grow(len(value))
	for offset := 0; offset < len(value); {
		end := offset
		if value[offset] == '+' {
			end++
		}
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		if phoneLike(value[offset:end]) {
			masked.WriteString("[masked-phone]")
			offset = end
			continue
		}
		_, width := utf8.DecodeRuneInString(value[offset:])
		masked.WriteString(value[offset : offset+width])
		offset += width
	}
	return masked.String(), nil
}

func phoneLike(value string) bool {
	digits := strings.TrimPrefix(value, "+86")
	return len(digits) == 11 && digits[0] == '1' && digits[1] >= '3' && digits[1] <= '9'
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func redactionRoots(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		root := path
		if index := strings.IndexAny(root, ".[\""); index >= 0 {
			root = root[:index]
		}
		if root == "" || (len(result) > 0 && result[len(result)-1] == root) {
			continue
		}
		result = append(result, root)
	}
	return result
}
