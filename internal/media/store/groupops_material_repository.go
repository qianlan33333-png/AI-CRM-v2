package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	groupopsmaterial "github.com/qianlan33333-png/AI-CRM-v2/internal/media/groupopsmaterial"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

// GroupOpsMaterialRepository owns the local source/lease join. Both methods
// require the caller's UoW: source rows are locked while acceptance persists
// the resulting snapshot, and receipt rows are locked while freezing it.
type GroupOpsMaterialRepository struct{ providerScopeDigest string }

var _ mediaport.GroupOpsMaterialSourceCapturer = (*GroupOpsMaterialRepository)(nil)
var _ groupopsmaterial.PreparedPlanReader = (*GroupOpsMaterialRepository)(nil)

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
		item, readErr := repository.readPreparedSource(ctx, query, source)
		if readErr != nil || (source.Reference.Kind != "group_invite" && !item.ReadyUntil.After(requiredThrough)) {
			return groupopsmaterial.PreparedPlan{}, groupOpsMaterialUnavailable(readErr)
		}
		plan.Items = append(plan.Items, item)
	}
	return plan, nil
}

func (repository *GroupOpsMaterialRepository) readPreparedSource(ctx context.Context, query *mediadb.Queries, source mediaport.GroupOpsMaterialSourceReference) (groupopsmaterial.PreparedMaterial, error) {
	if source.Reference.Kind == "group_invite" {
		return groupopsmaterial.PreparedMaterial{Reference: source.Reference, SourceDigest: source.SourceDigest, Attachment: source.ProviderFields}, nil
	}
	kind, id, digest := source.Reference.Kind, source.Reference.ID, source.SourceDigest
	if source.Reference.Kind == "attachment" {
		kind = "attachment"
	} else if source.Reference.Kind == "miniprogram" {
		kind, id, digest = "image", source.ThumbnailImageID, source.ThumbnailSourceDigest
	}
	row, err := query.ReadGroupOpsPreparedUpload(ctx, mediadb.ReadGroupOpsPreparedUploadParams{SourceKind: kind, SourceID: id, SourceDigest: digest, ProviderScopeDigest: repository.providerScopeDigest})
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
	return len(value) == len("sha256:")+64 && len(value) > 7 && value[:7] == "sha256:"
}

func groupOpsMaterialUnavailable(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || err == nil {
		return groupopsmaterial.ErrUnavailable
	}
	return err
}
