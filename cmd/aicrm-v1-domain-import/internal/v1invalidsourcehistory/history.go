// Package v1invalidsourcehistory selects sealed V1 quarantine rows as inert,
// source-only facts. It neither writes targets nor makes invalid definitions
// executable.
package v1invalidsourcehistory

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

const (
	ContactTagsTable       = "public/contact_tags"
	AutomationChannelTable = "public/automation_channel"
	ImageLibraryTable      = "public/image_library"
	AttachmentLibraryTable = "public/attachment_library"
	RadarLinksTable        = "public/radar_links"
)

var (
	ErrInvalidSelection = errors.New("invalid V1 invalid-source selection")
	ErrSealedDrift      = errors.New("sealed V1 invalid-source drift")
)

type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

type TerminalScope struct {
	ImportVersion, TableID, TargetDomain, TargetTable string
}

type TerminalReceipt struct {
	SourceKeyDigest, PayloadDigest, TargetDigest [sha256.Size]byte
	Disposition, Reason, TargetID                string
	Metadata                                     map[string]any
	Verified                                     bool
}

// TerminalLoader must read the old, sealed generic receipt under this exact
// source scope. It does not record a receipt or open a target writer.
type TerminalLoader interface {
	LoadTerminal(context.Context, TerminalScope, string) (TerminalReceipt, bool, error)
}

type Options struct {
	ArchiveRunID  string
	SourceHMACKey []byte
}

type SelectedUnboundTag struct {
	SourceIdentifier string
	SourceOrdinal    int64
	Fact             contactport.HistoricalUnboundTag
}

type SelectedInvalidChannel struct {
	SourceIdentifier string
	SourceOrdinal    int64
	Fact             contactport.HistoricalInvalidChannel
}

type SelectedInvalidAsset struct {
	SourceIdentifier string
	SourceOrdinal    int64
	Fact             mediaport.HistoricalInvalidAsset
}

type SelectedInvalidRadarLink struct {
	SourceIdentifier string
	SourceOrdinal    int64
	Fact             radarport.HistoricalInvalidRadarLink
}

type Summary struct {
	UnboundTags, InvalidChannels, Images, Attachments, RadarLinks int
}

func (value Summary) Total() int {
	return value.UnboundTags + value.InvalidChannels + value.Images + value.Attachments + value.RadarLinks
}

type Selection struct {
	UnboundTags     []SelectedUnboundTag
	InvalidChannels []SelectedInvalidChannel
	InvalidAssets   []SelectedInvalidAsset
	InvalidRadar    []SelectedInvalidRadarLink
}

func (value Selection) Summary() Summary {
	summary := Summary{UnboundTags: len(value.UnboundTags), InvalidChannels: len(value.InvalidChannels), RadarLinks: len(value.InvalidRadar)}
	for _, asset := range value.InvalidAssets {
		switch asset.Fact.Kind {
		case "image":
			summary.Images++
		case "attachment":
			summary.Attachments++
		}
	}
	return summary
}

func Select(ctx context.Context, archive ArchiveSource, terminals TerminalLoader, options Options) (Selection, error) {
	if ctx == nil || archive == nil || terminals == nil || options.ArchiveRunID == "" || len(options.SourceHMACKey) < sha256.Size {
		return Selection{}, ErrInvalidSelection
	}
	result := Selection{UnboundTags: []SelectedUnboundTag{}, InvalidChannels: []SelectedInvalidChannel{}, InvalidAssets: []SelectedInvalidAsset{}, InvalidRadar: []SelectedInvalidRadarLink{}}
	for _, spec := range invalidSourceSpecs {
		next := int64(1)
		err := archive.EachTableRow(ctx, options.ArchiveRunID, spec.scope.TableID, func(row v1archive.ArchivedRow) error {
			if !validEnvelope(row, spec.scope.TableID, next, options.SourceHMACKey) {
				return ErrSealedDrift
			}
			next++
			terminal, found, err := terminals.LoadTerminal(ctx, spec.scope, sourceIdentifier(row.SourceKeyHMAC))
			if err != nil || !found || !validTerminal(row, terminal) {
				return ErrSealedDrift
			}
			if terminal.Disposition != "quarantine" {
				return nil
			}
			if terminal.Reason != spec.reason || terminal.TargetID != "" || terminal.TargetDigest != ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 {
				return ErrSealedDrift
			}
			if err := spec.append(&result, row, options.SourceHMACKey); err != nil {
				return err
			}
			return nil
		})
		if err != nil || next == 1 {
			return Selection{}, ErrSealedDrift
		}
	}
	return result, nil
}

type invalidSourceSpec struct {
	scope  TerminalScope
	reason string
	append func(*Selection, v1archive.ArchivedRow, []byte) error
}

var invalidSourceSpecs = []invalidSourceSpec{
	{scope: TerminalScope{ImportVersion: "v1-static-a1", TableID: ContactTagsTable, TargetDomain: "contact", TargetTable: "customer_tags"}, reason: "invalid_contact_tag", append: appendUnboundTag},
	{scope: TerminalScope{ImportVersion: "v1-channel-a1", TableID: AutomationChannelTable, TargetDomain: "contact", TargetTable: "channels"}, reason: "invalid_channel_definition", append: appendInvalidChannel},
	{scope: TerminalScope{ImportVersion: "v1-static-a1", TableID: ImageLibraryTable, TargetDomain: "media", TargetTable: "media_images"}, reason: "invalid_static_media_definition", append: appendInvalidImage},
	{scope: TerminalScope{ImportVersion: "v1-static-a1", TableID: AttachmentLibraryTable, TargetDomain: "media", TargetTable: "media_attachments"}, reason: "invalid_static_media_definition", append: appendInvalidAttachment},
	{scope: TerminalScope{ImportVersion: "v1-domain-a1", TableID: RadarLinksTable, TargetDomain: "radar", TargetTable: "radar_links"}, reason: "invalid_radar_definition", append: appendInvalidRadar},
}

func validEnvelope(row v1archive.ArchivedRow, table string, ordinal int64, key []byte) bool {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
		return false
	}
	payload, payloadErr := v1archive.PayloadHMAC(key, strings.TrimPrefix(table, "public/"), row.Payload)
	fields, fieldsErr := v1archive.FieldHMAC(key, strings.TrimPrefix(table, "public/"), row.RedactedFields)
	return payloadErr == nil && fieldsErr == nil && payload == row.PayloadHMAC && fields == row.FieldHMAC
}

func validTerminal(row v1archive.ArchivedRow, receipt TerminalReceipt) bool {
	if !receipt.Verified || receipt.SourceKeyDigest != row.SourceKeyHMAC || receipt.PayloadDigest != row.PayloadHMAC {
		return false
	}
	switch receipt.Disposition {
	case "import":
		return receipt.Reason == "" && receipt.TargetID != "" && receipt.TargetDigest != ([sha256.Size]byte{})
	case "archive", "quarantine":
		return receipt.Reason != "" && receipt.TargetID == "" && receipt.TargetDigest == ([sha256.Size]byte{})
	default:
		return false
	}
}

func appendUnboundTag(result *Selection, row v1archive.ArchivedRow, key []byte) error {
	var value struct {
		TagID     string    `json:"tag_id"`
		UnionID   string    `json:"unionid"`
		CreatedAt time.Time `json:"created_at"`
	}
	if json.Unmarshal(row.Payload, &value) != nil {
		return ErrSealedDrift
	}
	fact := contactport.HistoricalUnboundTag{SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC,
		PrivateDigest: privateDigest(key, "tag", row.Payload), RedactedRoots: copyRoots(row.RedactedFields), TagSourceID: value.TagID, CreatedAt: value.CreatedAt, QuarantineReason: "invalid_contact_tag"}
	if value.UnionID != "" {
		fact.UnionIDDigest = privateDigest(key, "tag-unionid", []byte(value.UnionID))
	}
	result.UnboundTags = append(result.UnboundTags, SelectedUnboundTag{SourceIdentifier: sourceIdentifier(row.SourceKeyHMAC), SourceOrdinal: row.SourceOrdinal, Fact: fact})
	return nil
}

func appendInvalidChannel(result *Selection, row v1archive.ArchivedRow, key []byte) error {
	var value struct {
		ID          *int64     `json:"id"`
		ChannelCode *string    `json:"channel_code"`
		ChannelName *string    `json:"channel_name"`
		ChannelType *string    `json:"channel_type"`
		CarrierType *string    `json:"carrier_type"`
		CreatedAt   *time.Time `json:"created_at"`
		UpdatedAt   *time.Time `json:"updated_at"`
	}
	if json.Unmarshal(row.Payload, &value) != nil {
		return ErrSealedDrift
	}
	fact := contactport.HistoricalInvalidChannel{SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC,
		PrivateDigest: privateDigest(key, "channel", row.Payload), RedactedRoots: copyRoots(row.RedactedFields), SourceID: int64OrZero(value.ID), Code: stringOrEmpty(value.ChannelCode), Name: stringOrEmpty(value.ChannelName), ChannelType: stringOrEmpty(value.ChannelType), CarrierType: stringOrEmpty(value.CarrierType), CreatedAt: timeOrZero(value.CreatedAt), UpdatedAt: timeOrZero(value.UpdatedAt), QuarantineReason: "invalid_channel_definition"}
	result.InvalidChannels = append(result.InvalidChannels, SelectedInvalidChannel{SourceIdentifier: sourceIdentifier(row.SourceKeyHMAC), SourceOrdinal: row.SourceOrdinal, Fact: fact})
	return nil
}

func appendInvalidImage(result *Selection, row v1archive.ArchivedRow, key []byte) error {
	asset, err := invalidAsset(row, key, "image")
	if err != nil {
		return err
	}
	result.InvalidAssets = append(result.InvalidAssets, SelectedInvalidAsset{SourceIdentifier: sourceIdentifier(row.SourceKeyHMAC), SourceOrdinal: row.SourceOrdinal, Fact: asset})
	return nil
}

func appendInvalidAttachment(result *Selection, row v1archive.ArchivedRow, key []byte) error {
	asset, err := invalidAsset(row, key, "attachment")
	if err != nil {
		return err
	}
	result.InvalidAssets = append(result.InvalidAssets, SelectedInvalidAsset{SourceIdentifier: sourceIdentifier(row.SourceKeyHMAC), SourceOrdinal: row.SourceOrdinal, Fact: asset})
	return nil
}

func invalidAsset(row v1archive.ArchivedRow, key []byte, kind string) (mediaport.HistoricalInvalidAsset, error) {
	var value struct {
		ID         *int64     `json:"id"`
		Name       *string    `json:"name"`
		FileName   *string    `json:"file_name"`
		MIMEType   *string    `json:"mime_type"`
		FileSize   *int64     `json:"file_size"`
		DataBase64 *string    `json:"data_base64"`
		Enabled    *bool      `json:"enabled"`
		CreatedAt  *time.Time `json:"created_at"`
		UpdatedAt  *time.Time `json:"updated_at"`
	}
	if json.Unmarshal(row.Payload, &value) != nil {
		return mediaport.HistoricalInvalidAsset{}, ErrSealedDrift
	}
	content, err := base64.StdEncoding.Strict().DecodeString(stringOrEmpty(value.DataBase64))
	if err != nil {
		return mediaport.HistoricalInvalidAsset{}, ErrSealedDrift
	}
	return mediaport.HistoricalInvalidAsset{SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC,
		PrivateDigest: privateDigest(key, "asset", row.Payload), RedactedRoots: copyRoots(row.RedactedFields), Kind: kind, SourceID: int64OrZero(value.ID), Name: stringOrEmpty(value.Name), FileName: stringOrEmpty(value.FileName), MIMEType: stringOrEmpty(value.MIMEType), FileSize: int64OrZero(value.FileSize), OriginalEnabled: boolOrFalse(value.Enabled), ContentDigest: privateDigest(key, "asset-content", content), CreatedAt: timeOrZero(value.CreatedAt), UpdatedAt: timeOrZero(value.UpdatedAt), QuarantineReason: "invalid_static_media_definition"}, nil
}

func appendInvalidRadar(result *Selection, row v1archive.ArchivedRow, key []byte) error {
	var value struct {
		ID          *int64     `json:"id"`
		Code        *string    `json:"code"`
		Title       *string    `json:"title"`
		OriginalURL *string    `json:"original_url"`
		CreatedAt   *time.Time `json:"created_at"`
		UpdatedAt   *time.Time `json:"updated_at"`
	}
	if json.Unmarshal(row.Payload, &value) != nil {
		return ErrSealedDrift
	}
	result.InvalidRadar = append(result.InvalidRadar, SelectedInvalidRadarLink{SourceIdentifier: sourceIdentifier(row.SourceKeyHMAC), SourceOrdinal: row.SourceOrdinal,
		Fact: radarport.HistoricalInvalidRadarLink{SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC, PrivateDigest: privateDigest(key, "radar", row.Payload), RedactedRoots: copyRoots(row.RedactedFields), SourceID: int64OrZero(value.ID), Code: stringOrEmpty(value.Code), Title: stringOrEmpty(value.Title), DestinationURLDigest: privateDigest(key, "radar-url", []byte(stringOrEmpty(value.OriginalURL))), CreatedAt: timeOrZero(value.CreatedAt), UpdatedAt: timeOrZero(value.UpdatedAt), QuarantineReason: "invalid_radar_definition"}})
	return nil
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func int64OrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
func boolOrFalse(value *bool) bool { return value != nil && *value }
func timeOrZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func privateDigest(key []byte, domain string, value []byte) [sha256.Size]byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("aicrm/v1-invalid-source-history/" + domain + "/v1\x00"))
	_, _ = mac.Write(value)
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func copyRoots(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func sourceIdentifier(value [sha256.Size]byte) string { return hex.EncodeToString(value[:]) }
