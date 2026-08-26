package store

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	groupopsmaterial "github.com/qianlan33333-png/AI-CRM-v2/internal/media/groupopsmaterial"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

// GroupOpsMaterialRepository owns the local source/lease join. Both methods
// require the caller's UoW: source rows are locked while acceptance persists
// the resulting snapshot, and receipt rows are locked while freezing it.
type GroupOpsMaterialRepository struct{ providerScopeDigest string }

type GroupOpsUploadPreparation struct {
	ID                                                        int64
	SourceKind, SourceDigest, ProviderScopeDigest, UploadKind string
	SourceID, ExternalEffectID                                int64
	State                                                     string
}

var _ mediaport.GroupOpsMaterialSourceCapturer = (*GroupOpsMaterialRepository)(nil)
var _ groupopsmaterial.PreparedPlanReader = (*GroupOpsMaterialRepository)(nil)
var _ mediaapp.GroupOpsMaterialPreparationStore = (*GroupOpsMaterialRepository)(nil)
var _ mediaapp.GroupOpsMaterialUploadAttemptStore = (*GroupOpsMaterialRepository)(nil)

func NewGroupOpsMaterialRepository(providerScopeDigest string) (*GroupOpsMaterialRepository, error) {
	if !groupOpsDigest(providerScopeDigest) {
		return nil, groupopsmaterial.ErrUnavailable
	}
	return &GroupOpsMaterialRepository{providerScopeDigest: providerScopeDigest}, nil
}

func (repository *GroupOpsMaterialRepository) CaptureGroupOpsMaterialSources(ctx context.Context, plan mediaport.GroupOpsMaterialPlan) (mediaport.GroupOpsMaterialSourceSnapshot, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || mediaport.ValidateGroupOpsMaterialPlan(plan) != nil {
		return mediaport.GroupOpsMaterialSourceSnapshot{}, groupOpsMaterialUnavailable(err)
	}
	snapshot := mediaport.GroupOpsMaterialSourceSnapshot{SchemaVersion: 1, References: make([]mediaport.GroupOpsMaterialSourceReference, 0, len(plan.References))}
	for _, reference := range plan.References {
		source, captureErr := captureGroupOpsSource(ctx, query, reference)
		if captureErr != nil {
			return mediaport.GroupOpsMaterialSourceSnapshot{}, captureErr
		}
		snapshot.References = append(snapshot.References, source)
	}
	if mediaport.ValidateGroupOpsMaterialSourceSnapshot(snapshot) != nil {
		return mediaport.GroupOpsMaterialSourceSnapshot{}, groupopsmaterial.ErrUnavailable
	}
	return snapshot, nil
}

func (repository *GroupOpsMaterialRepository) ReadPreparedGroupOpsPlan(ctx context.Context, sources mediaport.GroupOpsMaterialSourceSnapshot, requiredThrough time.Time) (groupopsmaterial.PreparedPlan, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || requiredThrough.IsZero() || mediaport.ValidateGroupOpsMaterialSourceSnapshot(sources) != nil {
		return groupopsmaterial.PreparedPlan{}, groupOpsMaterialUnavailable(err)
	}
	plan := groupopsmaterial.PreparedPlan{Items: make([]groupopsmaterial.PreparedMaterial, 0, len(sources.References))}
	for _, source := range sources.References {
		item, readErr := repository.readPreparedSource(ctx, query, source, requiredThrough)
		if readErr != nil || (source.Reference.Kind != "group_invite" && !item.ReadyUntil.After(requiredThrough)) {
			return groupopsmaterial.PreparedPlan{}, groupOpsMaterialUnavailable(readErr)
		}
		plan.Items = append(plan.Items, item)
	}
	return plan, nil
}

// BindGroupOpsUploadPreparation persists the Media-owned typed fact after EER
// accepted the effect and before it is queued. The same external effect may
// replay, but it may never be rebound to another source or provider scope.
func (repository *GroupOpsMaterialRepository) BindGroupOpsUploadPreparation(ctx context.Context, value mediaapp.GroupOpsMaterialPreparation) (bool, error) {
	_, inserted, err := repository.bindGroupOpsUploadPreparation(ctx, value.SourceKind, value.SourceID, value.SourceDigest, repository.providerScopeDigest, value.UploadKind, value.EffectID, value.CreatedAt)
	return inserted, err
}

func (repository *GroupOpsMaterialRepository) HasSufficientGroupOpsUploadLease(ctx context.Context, sourceKind string, sourceID int64, sourceDigest, providerScopeDigest, uploadKind string, requiredThrough time.Time) (bool, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || sourceID < 1 || requiredThrough.IsZero() || providerScopeDigest != repository.providerScopeDigest || !groupOpsDigest(sourceDigest) || !groupOpsDigest(providerScopeDigest) {
		return false, groupOpsMaterialUnavailable(err)
	}
	return query.HasSufficientGroupOpsUploadLease(ctx, mediadb.HasSufficientGroupOpsUploadLeaseParams{SourceKind: sourceKind, SourceID: sourceID, SourceDigest: sourceDigest, ProviderScopeDigest: providerScopeDigest, UploadKind: uploadKind, RequiredThrough: stamp(requiredThrough.UTC())})
}

func (repository *GroupOpsMaterialRepository) NextGroupOpsUploadPreparationGeneration(ctx context.Context, sourceKind string, sourceID int64, sourceDigest, providerScopeDigest, uploadKind string) (int64, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || sourceID < 1 || providerScopeDigest != repository.providerScopeDigest || !groupOpsDigest(sourceDigest) || !groupOpsDigest(providerScopeDigest) {
		return 0, groupOpsMaterialUnavailable(err)
	}
	if err = query.LockGroupOpsUploadPreparationGeneration(ctx, groupOpsPreparationLockKey(sourceKind, sourceID, sourceDigest, providerScopeDigest, uploadKind)); err != nil {
		return 0, groupOpsMaterialUnavailable(err)
	}
	generation, err := query.NextGroupOpsUploadPreparationGeneration(ctx, mediadb.NextGroupOpsUploadPreparationGenerationParams{SourceKind: sourceKind, SourceID: sourceID, SourceDigest: sourceDigest, ProviderScopeDigest: providerScopeDigest, UploadKind: uploadKind})
	if err != nil || generation < 1 {
		return 0, groupOpsMaterialUnavailable(err)
	}
	return int64(generation), nil
}

func groupOpsPreparationLockKey(sourceKind string, sourceID int64, sourceDigest, providerScopeDigest, uploadKind string) int64 {
	sum := sha256.Sum256([]byte(sourceKind + "\x00" + strconv.FormatInt(sourceID, 10) + "\x00" + sourceDigest + "\x00" + providerScopeDigest + "\x00" + uploadKind))
	return int64(binary.BigEndian.Uint64(sum[:8]))
}

func (repository *GroupOpsMaterialRepository) LoadGroupOpsMaterialUpload(ctx context.Context, effectID string) (mediaapp.GroupOpsMaterialUploadInput, error) {
	query, err := queries(ctx)
	parsedEffectID, parseErr := parseGroupOpsEffectID(effectID)
	if repository == nil || err != nil || parseErr != nil {
		return mediaapp.GroupOpsMaterialUploadInput{}, groupOpsMaterialUnavailable(errors.Join(err, parseErr))
	}
	preparation, err := query.ReadGroupOpsUploadPreparationAttempt(ctx, parsedEffectID)
	if err != nil || preparation.State != "preparing" || preparation.ProviderScopeDigest != repository.providerScopeDigest {
		return mediaapp.GroupOpsMaterialUploadInput{}, groupOpsMaterialUnavailable(err)
	}
	var filename, mimeType string
	var content, sourceChecksum, blobChecksum []byte
	if preparation.SourceKind == "image" {
		row, readErr := query.LockGroupOpsImageSource(ctx, preparation.SourceID)
		if readErr != nil {
			return mediaapp.GroupOpsMaterialUploadInput{}, groupOpsMaterialUnavailable(readErr)
		}
		filename, mimeType, content, sourceChecksum, blobChecksum = row.FileName, row.MimeType, row.Content, row.SourceChecksum, row.BlobChecksum
	} else if preparation.SourceKind == "attachment" {
		row, readErr := query.LockGroupOpsAttachmentSource(ctx, preparation.SourceID)
		if readErr != nil {
			return mediaapp.GroupOpsMaterialUploadInput{}, groupOpsMaterialUnavailable(readErr)
		}
		filename, mimeType, content, sourceChecksum, blobChecksum = row.FileName, row.MimeType, row.Content, row.SourceChecksum, row.BlobChecksum
	} else {
		return mediaapp.GroupOpsMaterialUploadInput{}, groupopsmaterial.ErrUnavailable
	}
	digest, digestErr := groupOpsBlobDigest(preparation.SourceKind, preparation.SourceID, filename, mimeType, sourceChecksum, blobChecksum, content)
	if digestErr != nil || digest != preparation.SourceDigest {
		return mediaapp.GroupOpsMaterialUploadInput{}, groupOpsMaterialUnavailable(digestErr)
	}
	return mediaapp.GroupOpsMaterialUploadInput{EffectID: effectID, SourceDigest: digest, Filename: filename, MIME: mimeType, Checksum: "sha256:" + hex.EncodeToString(sourceChecksum), Kind: preparation.UploadKind, Bytes: append([]byte(nil), content...)}, nil
}

func (repository *GroupOpsMaterialRepository) RecordGroupOpsMaterialUploadReady(ctx context.Context, effectID string, result mediaapp.GroupOpsMaterialUploadResult, receiptDigest eer.Digest) error {
	query, err := queries(ctx)
	parsedEffectID, parseErr := parseGroupOpsEffectID(effectID)
	if repository == nil || err != nil || parseErr != nil || !groupOpsDigest(string(receiptDigest)) || result.MediaID == "" || result.CreatedAt.IsZero() || !result.ExpiresAt.After(result.CreatedAt) {
		return groupOpsMaterialUnavailable(errors.Join(err, parseErr))
	}
	preparation, err := query.ReadGroupOpsUploadPreparationAttempt(ctx, parsedEffectID)
	if err != nil || preparation.State != "preparing" {
		return groupOpsMaterialUnavailable(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err = query.InsertGroupOpsUploadReceipt(ctx, mediadb.InsertGroupOpsUploadReceiptParams{ExternalEffectID: parsedEffectID, PreparationID: preparation.ID, ProviderMediaID: result.MediaID, ProviderCreatedAt: stamp(result.CreatedAt.UTC()), ExpiresAt: stamp(result.ExpiresAt.UTC()), ReceiptDigest: string(receiptDigest), CreatedAt: stamp(now)}); err != nil {
		return groupOpsMaterialUnavailable(err)
	}
	return query.MarkGroupOpsUploadPreparationReady(ctx, mediadb.MarkGroupOpsUploadPreparationReadyParams{ProviderMediaID: result.MediaID, ProviderCreatedAt: stamp(result.CreatedAt.UTC()), ExpiresAt: stamp(result.ExpiresAt.UTC()), ReceiptDigest: string(receiptDigest), UpdatedAt: stamp(now), PreparationID: preparation.ID})
}

func (repository *GroupOpsMaterialRepository) MarkGroupOpsMaterialUploadOutcomeUnknown(ctx context.Context, effectID string, now time.Time) error {
	return repository.markGroupOpsMaterialUploadTerminal(ctx, effectID, now, true)
}

func (repository *GroupOpsMaterialRepository) MarkGroupOpsMaterialUploadFinalFailed(ctx context.Context, effectID string, now time.Time) error {
	return repository.markGroupOpsMaterialUploadTerminal(ctx, effectID, now, false)
}

func (repository *GroupOpsMaterialRepository) markGroupOpsMaterialUploadTerminal(ctx context.Context, effectID string, now time.Time, unknown bool) error {
	query, err := queries(ctx)
	parsedEffectID, parseErr := parseGroupOpsEffectID(effectID)
	if repository == nil || err != nil || parseErr != nil || now.IsZero() {
		return groupOpsMaterialUnavailable(errors.Join(err, parseErr))
	}
	preparation, err := query.ReadGroupOpsUploadPreparationAttempt(ctx, parsedEffectID)
	if err != nil || preparation.State != "preparing" {
		return groupOpsMaterialUnavailable(err)
	}
	if unknown {
		return query.MarkGroupOpsUploadPreparationOutcomeUnknown(ctx, mediadb.MarkGroupOpsUploadPreparationOutcomeUnknownParams{PreparationID: preparation.ID, UpdatedAt: stamp(now.UTC())})
	}
	return query.MarkGroupOpsUploadPreparationFinalFailed(ctx, mediadb.MarkGroupOpsUploadPreparationFinalFailedParams{PreparationID: preparation.ID, UpdatedAt: stamp(now.UTC())})
}

func (repository *GroupOpsMaterialRepository) bindGroupOpsUploadPreparation(ctx context.Context, sourceKind string, sourceID int64, sourceDigest, providerScopeDigest, uploadKind, effectID string, now time.Time) (GroupOpsUploadPreparation, bool, error) {
	query, err := queries(ctx)
	parsedEffectID, parseErr := parseGroupOpsEffectID(effectID)
	if repository == nil || err != nil || parseErr != nil || !groupOpsDigest(sourceDigest) || providerScopeDigest != repository.providerScopeDigest || !groupOpsDigest(providerScopeDigest) || sourceID < 1 || (sourceKind != "image" && sourceKind != "attachment") || (uploadKind != "image" && uploadKind != "file") || now.IsZero() {
		return GroupOpsUploadPreparation{}, false, groupOpsMaterialUnavailable(errors.Join(err, parseErr))
	}
	row, insertErr := query.InsertGroupOpsUploadPreparation(ctx, mediadb.InsertGroupOpsUploadPreparationParams{SourceKind: sourceKind, SourceID: sourceID, SourceDigest: sourceDigest, ProviderScopeDigest: providerScopeDigest, UploadKind: uploadKind, ExternalEffectID: parsedEffectID, CreatedAt: stamp(now.UTC())})
	if insertErr == nil {
		return groupOpsPreparation(row.ID, row.SourceKind, row.SourceID, row.SourceDigest, row.ProviderScopeDigest, row.UploadKind, row.ExternalEffectID, row.State), true, nil
	}
	if !errors.Is(insertErr, pgx.ErrNoRows) {
		return GroupOpsUploadPreparation{}, false, insertErr
	}
	existing, readErr := query.GetGroupOpsUploadPreparation(ctx, parsedEffectID)
	if readErr != nil {
		return GroupOpsUploadPreparation{}, false, groupOpsMaterialUnavailable(readErr)
	}
	value := groupOpsPreparation(existing.ID, existing.SourceKind, existing.SourceID, existing.SourceDigest, existing.ProviderScopeDigest, existing.UploadKind, existing.ExternalEffectID, existing.State)
	if value.SourceKind != sourceKind || value.SourceID != sourceID || value.SourceDigest != sourceDigest || value.ProviderScopeDigest != providerScopeDigest || value.UploadKind != uploadKind {
		return GroupOpsUploadPreparation{}, false, groupopsmaterial.ErrUnavailable
	}
	return value, false, nil
}

func groupOpsPreparation(id int64, sourceKind string, sourceID int64, sourceDigest, scope, uploadKind string, effectID int64, state string) GroupOpsUploadPreparation {
	return GroupOpsUploadPreparation{ID: id, SourceKind: sourceKind, SourceID: sourceID, SourceDigest: sourceDigest, ProviderScopeDigest: scope, UploadKind: uploadKind, ExternalEffectID: effectID, State: state}
}

func parseGroupOpsEffectID(value string) (int64, error) {
	if len(value) < 6 || value[:4] != "eer_" {
		return 0, groupopsmaterial.ErrUnavailable
	}
	parsed, err := strconv.ParseInt(value[4:], 10, 64)
	if err != nil || parsed < 1 || "eer_"+strconv.FormatInt(parsed, 10) != value {
		return 0, groupopsmaterial.ErrUnavailable
	}
	return parsed, nil
}

func (repository *GroupOpsMaterialRepository) readPreparedSource(ctx context.Context, query *mediadb.Queries, source mediaport.GroupOpsMaterialSourceReference, requiredThrough time.Time) (groupopsmaterial.PreparedMaterial, error) {
	if source.Reference.Kind == "group_invite" {
		return groupopsmaterial.PreparedMaterial{Reference: source.Reference, SourceDigest: source.SourceDigest, Attachment: source.ProviderFields}, nil
	}
	kind, id, digest := source.Reference.Kind, source.Reference.ID, source.SourceDigest
	if source.Reference.Kind == "attachment" {
		kind = "attachment"
	} else if source.Reference.Kind == "miniprogram" {
		kind, id, digest = "image", source.ThumbnailImageID, source.ThumbnailSourceDigest
	}
	row, err := query.ReadGroupOpsPreparedUpload(ctx, mediadb.ReadGroupOpsPreparedUploadParams{SourceKind: kind, SourceID: id, SourceDigest: digest, ProviderScopeDigest: repository.providerScopeDigest, RequiredThrough: stamp(requiredThrough.UTC())})
	if err != nil || !row.ExpiresAt.Valid || !groupOpsDigest(row.ReceiptDigest) {
		return groupopsmaterial.PreparedMaterial{}, err
	}
	attachment := mediaport.GroupOpsProviderReadyAttachment{MediaID: row.ProviderMediaID}
	switch source.Reference.Kind {
	case "image":
		attachment.MsgType = "image"
	case "attachment":
		attachment.MsgType = "file"
	case "miniprogram":
		attachment = source.ProviderFields
		attachment.MediaID = row.ProviderMediaID
	default:
		return groupopsmaterial.PreparedMaterial{}, groupopsmaterial.ErrUnavailable
	}
	return groupopsmaterial.PreparedMaterial{Reference: source.Reference, SourceDigest: source.SourceDigest, ReceiptDigest: row.ReceiptDigest, ReadyUntil: row.ExpiresAt.Time.UTC(), Attachment: attachment}, nil
}

func captureGroupOpsSource(ctx context.Context, query *mediadb.Queries, reference mediaport.GroupOpsMaterialReference) (mediaport.GroupOpsMaterialSourceReference, error) {
	switch reference.Kind {
	case "image":
		row, err := query.LockGroupOpsImageSource(ctx, reference.ID)
		if err != nil {
			return mediaport.GroupOpsMaterialSourceReference{}, groupOpsMaterialUnavailable(err)
		}
		digest, err := groupOpsBlobDigest("image", row.ID, row.FileName, row.MimeType, row.SourceChecksum, row.BlobChecksum, row.Content)
		return mediaport.GroupOpsMaterialSourceReference{Reference: reference, SourceDigest: digest}, err
	case "attachment":
		row, err := query.LockGroupOpsAttachmentSource(ctx, reference.ID)
		if err != nil {
			return mediaport.GroupOpsMaterialSourceReference{}, groupOpsMaterialUnavailable(err)
		}
		digest, err := groupOpsBlobDigest("attachment", row.ID, row.FileName, row.MimeType, row.SourceChecksum, row.BlobChecksum, row.Content)
		return mediaport.GroupOpsMaterialSourceReference{Reference: reference, SourceDigest: digest}, err
	case "miniprogram":
		row, err := query.LockGroupOpsMiniProgramSource(ctx, reference.ID)
		if err != nil || !row.ThumbnailImageID.Valid || row.ThumbnailImageID.Int64 < 1 {
			return mediaport.GroupOpsMaterialSourceReference{}, groupOpsMaterialUnavailable(err)
		}
		thumbnail, err := query.LockGroupOpsImageSource(ctx, row.ThumbnailImageID.Int64)
		if err != nil {
			return mediaport.GroupOpsMaterialSourceReference{}, groupOpsMaterialUnavailable(err)
		}
		thumbnailDigest, err := groupOpsBlobDigest("image", thumbnail.ID, thumbnail.FileName, thumbnail.MimeType, thumbnail.SourceChecksum, thumbnail.BlobChecksum, thumbnail.Content)
		if err != nil {
			return mediaport.GroupOpsMaterialSourceReference{}, err
		}
		digest, err := groupOpsSourceDigest("miniprogram", row.ID, row.AppID, row.PagePath, row.Title, fmt.Sprint(row.ThumbnailImageID.Int64), thumbnailDigest)
		return mediaport.GroupOpsMaterialSourceReference{Reference: reference, SourceDigest: digest, ThumbnailImageID: row.ThumbnailImageID.Int64, ThumbnailSourceDigest: thumbnailDigest, ProviderFields: mediaport.GroupOpsProviderReadyAttachment{MsgType: "miniprogram", AppID: row.AppID, PagePath: row.PagePath, Title: row.Title}}, err
	case "group_invite":
		row, err := query.LockGroupOpsGroupInviteSource(ctx, reference.ID)
		if err != nil {
			return mediaport.GroupOpsMaterialSourceReference{}, groupOpsMaterialUnavailable(err)
		}
		digest, err := groupOpsSourceDigest("group_invite", row.ID, row.Title, row.Description, row.JoinUrl)
		return mediaport.GroupOpsMaterialSourceReference{Reference: reference, SourceDigest: digest, ProviderFields: mediaport.GroupOpsProviderReadyAttachment{MsgType: "link", Title: row.Title, Description: row.Description, URL: row.JoinUrl}}, err
	default:
		return mediaport.GroupOpsMaterialSourceReference{}, groupopsmaterial.ErrUnavailable
	}
}

func groupOpsBlobDigest(kind string, id int64, filename, mimeType string, sourceChecksum, blobChecksum, content []byte) (string, error) {
	if len(sourceChecksum) != sha256.Size || len(blobChecksum) != sha256.Size || string(sourceChecksum) != string(blobChecksum) || sha256.Sum256(content) != bytesToDigest(blobChecksum) {
		return "", groupopsmaterial.ErrUnavailable
	}
	return groupOpsSourceDigest(kind, id, filename, mimeType, hex.EncodeToString(sourceChecksum))
}

func bytesToDigest(value []byte) [sha256.Size]byte {
	var digest [sha256.Size]byte
	copy(digest[:], value)
	return digest
}

func groupOpsSourceDigest(kind string, id int64, fields ...string) (string, error) {
	payload, err := json.Marshal(struct {
		Kind   string   `json:"kind"`
		ID     int64    `json:"id"`
		Fields []string `json:"fields"`
	}{Kind: kind, ID: id, Fields: fields})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func groupOpsDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || value[:7] != "sha256:" {
		return false
	}
	_, err := hex.DecodeString(value[7:])
	return err == nil && value[7:] == strings.ToLower(value[7:])
}

func groupOpsMaterialUnavailable(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || err == nil {
		return groupopsmaterial.ErrUnavailable
	}
	return err
}
