package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// AttachmentRepository keeps attachment metadata, its private bytea blob, and
// the mutation receipt in the transaction supplied by the Media application
// service. It intentionally does not introduce a second blob abstraction.
type AttachmentRepository struct{}

var _ mediaapp.AttachmentStore = (*AttachmentRepository)(nil)
var _ mediaport.AttachmentMetadataReader = (*AttachmentRepository)(nil)
var _ mediaport.ChannelAttachmentReferenceReader = (*AttachmentRepository)(nil)

func NewAttachmentRepository() *AttachmentRepository { return &AttachmentRepository{} }

func (repository *AttachmentRepository) ReserveAttachmentMutation(ctx context.Context, reservation mediaapp.AttachmentMutationReservation) (mediaapp.AttachmentMutationReceipt, bool, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaapp.AttachmentMutationReceipt{}, false, attachmentStoreUnavailable(err)
	}
	row, err := query.ReserveMediaAttachmentMutation(ctx, mediadb.ReserveMediaAttachmentMutationParams{
		Operation: reservation.Operation, ActorScope: reservation.ActorScope, BusinessKey: reservation.BusinessKey,
		KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], CreatedAt: stamp(reservation.CreatedAt),
	})
	if err == nil {
		receipt, mapErr := attachmentReceipt(row.ID, row.Operation, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot)
		return receipt, true, mapErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.AttachmentMutationReceipt{}, false, err
	}
	existing, err := query.GetMediaAttachmentMutation(ctx, mediadb.GetMediaAttachmentMutationParams{
		Operation: reservation.Operation, ActorScope: reservation.ActorScope, BusinessKey: reservation.BusinessKey, KeyDigest: reservation.KeyDigest[:],
	})
	if err != nil {
		return mediaapp.AttachmentMutationReceipt{}, false, err
	}
	receipt, mapErr := attachmentReceipt(existing.ID, existing.Operation, existing.ActorScope, existing.BusinessKey, existing.KeyDigest, existing.PayloadDigest, existing.State, existing.ResultSnapshot)
	return receipt, false, mapErr
}

func (repository *AttachmentRepository) CreateAttachment(ctx context.Context, input mediaapp.AttachmentCreateInput) (mediaport.Attachment, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.Attachment{}, attachmentStoreUnavailable(err)
	}
	tags, err := json.Marshal(input.Command.Tags)
	if err != nil {
		return mediaport.Attachment{}, mediaapp.ErrAttachmentUnavailable
	}
	if len(input.Command.Content) < 1 || len(input.Command.Content) > int(^uint32(0)>>1) {
		return mediaport.Attachment{}, mediaapp.ErrAttachmentUnavailable
	}
	now := stamp(input.Now)
	row, err := query.InsertMediaAttachment(ctx, mediadb.InsertMediaAttachmentParams{
		Name: input.Command.Name, FileName: input.Command.FileName, MimeType: input.MediaType,
		FileSize: int32(len(input.Command.Content)), Checksum: input.Checksum[:], Description: input.Command.Description,
		Tags: tags, Enabled: input.Command.Enabled == nil || *input.Command.Enabled, CreatedBy: input.Command.Actor,
		UpdatedBy: input.Command.Actor, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return mediaport.Attachment{}, err
	}
	if err = query.InsertMediaAttachmentBlob(ctx, mediadb.InsertMediaAttachmentBlobParams{
		AttachmentID: row.ID, Content: input.Command.Content, Checksum: input.Checksum[:], CreatedAt: now,
	}); err != nil {
		return mediaport.Attachment{}, err
	}
	return attachmentFromFields(row.ID, row.Name, row.FileName, row.MimeType, row.FileSize, row.Description, row.Tags, row.Enabled, row.Version, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt)
}

func (repository *AttachmentRepository) GetAttachment(ctx context.Context, attachmentID int64) (mediaport.Attachment, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || attachmentID < 1 {
		return mediaport.Attachment{}, attachmentStoreUnavailable(err)
	}
	row, err := query.GetMediaAttachment(ctx, attachmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaport.Attachment{}, mediaapp.ErrAttachmentNotFound
	}
	if err != nil {
		return mediaport.Attachment{}, err
	}
	return attachmentFromFields(row.ID, row.Name, row.FileName, row.MimeType, row.FileSize, row.Description, row.Tags, row.Enabled, row.Version, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt)
}

func (repository *AttachmentRepository) GetAttachmentForUpdate(ctx context.Context, attachmentID int64) (mediaport.Attachment, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || attachmentID < 1 {
		return mediaport.Attachment{}, attachmentStoreUnavailable(err)
	}
	row, err := query.LockMediaAttachmentForUpdate(ctx, attachmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaport.Attachment{}, mediaapp.ErrAttachmentNotFound
	}
	if err != nil {
		return mediaport.Attachment{}, err
	}
	return attachmentFromFields(row.ID, row.Name, row.FileName, row.MimeType, row.FileSize, row.Description, row.Tags, row.Enabled, row.Version, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt)
}

func (repository *AttachmentRepository) ListAttachments(ctx context.Context, input mediaport.AttachmentListQuery) (mediaapp.AttachmentListRead, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || input.Limit < 1 || input.Offset < 0 {
		return mediaapp.AttachmentListRead{}, attachmentStoreUnavailable(err)
	}
	rows, err := query.ListMediaAttachments(ctx, mediadb.ListMediaAttachmentsParams{
		Search: input.Search, EnabledOnly: input.EnabledOnly, RowOffset: input.Offset, RowLimit: input.Limit,
	})
	if err != nil {
		return mediaapp.AttachmentListRead{}, err
	}
	read := mediaapp.AttachmentListRead{Items: []mediaport.Attachment{}}
	for _, row := range rows {
		if read.Total != 0 && read.Total != row.Total {
			return mediaapp.AttachmentListRead{}, mediaapp.ErrAttachmentUnavailable
		}
		read.Total = row.Total
		if !row.ID.Valid {
			if len(rows) != 1 || row.Total != 0 || !attachmentListNullRow(row) {
				return mediaapp.AttachmentListRead{}, mediaapp.ErrAttachmentUnavailable
			}
			continue
		}
		if !row.Name.Valid || !row.FileName.Valid || !row.MimeType.Valid || !row.FileSize.Valid || !row.Description.Valid || !row.Enabled.Valid || !row.Version.Valid || !row.CreatedBy.Valid || !row.UpdatedBy.Valid || !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
			return mediaapp.AttachmentListRead{}, mediaapp.ErrAttachmentUnavailable
		}
		attachment, mapErr := attachmentFromFields(row.ID.Int64, row.Name.String, row.FileName.String, row.MimeType.String, row.FileSize.Int32, row.Description.String, row.Tags, row.Enabled.Bool, row.Version.Int64, row.CreatedBy.Int64, row.UpdatedBy.Int64, row.CreatedAt, row.UpdatedAt)
		if mapErr != nil {
			return mediaapp.AttachmentListRead{}, mapErr
		}
		read.Items = append(read.Items, attachment)
	}
	if len(rows) == 0 {
		return mediaapp.AttachmentListRead{}, mediaapp.ErrAttachmentUnavailable
	}
	return read, nil
}

func (repository *AttachmentRepository) ReadAttachment(ctx context.Context, attachmentID int64) (mediaapp.AttachmentBlob, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || attachmentID < 1 {
		return mediaapp.AttachmentBlob{}, attachmentStoreUnavailable(err)
	}
	row, err := query.ReadMediaAttachment(ctx, attachmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.AttachmentBlob{}, mediaapp.ErrAttachmentNotFound
	}
	if err != nil {
		return mediaapp.AttachmentBlob{}, err
	}
	attachment, err := attachmentFromFields(row.ID, row.Name, row.FileName, row.MimeType, row.FileSize, row.Description, row.Tags, row.Enabled, row.Version, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt)
	if err != nil || len(row.AttachmentChecksum) != 32 || len(row.BlobChecksum) != 32 || !bytes.Equal(row.AttachmentChecksum, row.BlobChecksum) || len(row.Content) != int(row.FileSize) {
		return mediaapp.AttachmentBlob{}, mediaapp.ErrAttachmentUnavailable
	}
	var checksum [32]byte
	copy(checksum[:], row.AttachmentChecksum)
	return mediaapp.AttachmentBlob{Attachment: attachment, Content: append([]byte(nil), row.Content...), Checksum: checksum}, nil
}

func (repository *AttachmentRepository) UpdateAttachment(ctx context.Context, input mediaapp.AttachmentUpdateInput) (mediaport.Attachment, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || input.Attachment.ID < 1 || input.ExpectedVersion < 1 {
		return mediaport.Attachment{}, attachmentStoreUnavailable(err)
	}
	tags, err := json.Marshal(input.Attachment.Tags)
	if err != nil {
		return mediaport.Attachment{}, mediaapp.ErrAttachmentUnavailable
	}
	row, err := query.UpdateMediaAttachment(ctx, mediadb.UpdateMediaAttachmentParams{
		Name: input.Attachment.Name, Description: input.Attachment.Description, Tags: tags, Enabled: input.Attachment.Enabled,
		Version: input.Attachment.Version, UpdatedBy: input.Attachment.UpdatedBy, UpdatedAt: stamp(input.Attachment.UpdatedAt),
		AttachmentID: input.Attachment.ID, ExpectedVersion: input.ExpectedVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaport.Attachment{}, mediaapp.ErrAttachmentConflict
	}
	if err != nil {
		return mediaport.Attachment{}, err
	}
	return attachmentFromFields(row.ID, row.Name, row.FileName, row.MimeType, row.FileSize, row.Description, row.Tags, row.Enabled, row.Version, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt)
}

func (repository *AttachmentRepository) DeleteAttachment(ctx context.Context, attachmentID int64) (int64, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || attachmentID < 1 {
		return 0, attachmentStoreUnavailable(err)
	}
	return query.DeleteMediaAttachment(ctx, attachmentID)
}

func (repository *AttachmentRepository) CompleteAttachmentMutation(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (mediaapp.AttachmentMutationReceipt, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || id < 1 || !json.Valid(snapshot) {
		return mediaapp.AttachmentMutationReceipt{}, attachmentStoreUnavailable(err)
	}
	row, err := query.CompleteMediaAttachmentMutation(ctx, mediadb.CompleteMediaAttachmentMutationParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stamp(now)})
	if err != nil {
		return mediaapp.AttachmentMutationReceipt{}, err
	}
	return attachmentReceipt(row.ID, row.Operation, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot)
}

// AttachmentExists obtains FOR KEY SHARE from the metadata row in the caller's
// UoW. Writers that use it cannot commit a new reference across a concurrent
// hard delete.
func (repository *AttachmentRepository) AttachmentExists(ctx context.Context, attachmentID int64) (bool, error) {
	query, err := queries(ctx)
	if repository == nil || attachmentID < 1 {
		return false, mediaapp.ErrAttachmentUnavailable
	}
	if err != nil {
		return false, err
	}
	_, err = query.LockMediaAttachmentReference(ctx, attachmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (repository *AttachmentRepository) ChannelAttachmentEligible(ctx context.Context, attachmentID int64) (bool, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if repository == nil || attachmentID < 1 || err != nil {
		return false, mediaapp.ErrAttachmentUnavailable
	}
	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM media_attachments WHERE id = $1 AND enabled = TRUE FOR KEY SHARE`, attachmentID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && id == attachmentID, err
}

func attachmentFromFields(id int64, name, fileName, mediaType string, fileSize int32, description string, rawTags []byte, enabled bool, version, createdBy, updatedBy int64, createdAt, updatedAt pgtype.Timestamptz) (mediaport.Attachment, error) {
	if id < 1 || fileSize < 1 || !createdAt.Valid || !updatedAt.Valid || len(rawTags) == 0 {
		return mediaport.Attachment{}, mediaapp.ErrAttachmentUnavailable
	}
	var tags []string
	if err := json.Unmarshal(rawTags, &tags); err != nil || tags == nil {
		return mediaport.Attachment{}, mediaapp.ErrAttachmentUnavailable
	}
	return mediaport.Attachment{ID: id, Name: name, FileName: fileName, MimeType: mediaType, FileSize: int64(fileSize), Description: description,
		Tags: append([]string{}, tags...), Enabled: enabled, Version: version, CreatedBy: createdBy, UpdatedBy: updatedBy,
		CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC()}, nil
}

func attachmentReceipt(id int64, operation, actorScope, businessKey string, keyDigest, payloadDigest []byte, state string, snapshot []byte) (mediaapp.AttachmentMutationReceipt, error) {
	if id < 1 || len(keyDigest) != 32 || len(payloadDigest) != 32 || !json.Valid(defaultAttachmentSnapshot(snapshot)) {
		return mediaapp.AttachmentMutationReceipt{}, mediaapp.ErrAttachmentUnavailable
	}
	var receipt mediaapp.AttachmentMutationReceipt
	receipt.ID, receipt.Operation, receipt.ActorScope, receipt.BusinessKey, receipt.State = id, operation, actorScope, businessKey, state
	copy(receipt.KeyDigest[:], keyDigest)
	copy(receipt.PayloadDigest[:], payloadDigest)
	if len(snapshot) != 0 {
		receipt.ResultSnapshot = append([]byte(nil), snapshot...)
	}
	return receipt, nil
}

func defaultAttachmentSnapshot(snapshot []byte) []byte {
	if len(snapshot) == 0 {
		return []byte("null")
	}
	return snapshot
}

func attachmentListNullRow(row mediadb.ListMediaAttachmentsRow) bool {
	return !row.Name.Valid && !row.FileName.Valid && !row.MimeType.Valid && !row.FileSize.Valid && !row.Description.Valid && !row.Enabled.Valid && !row.Version.Valid && !row.CreatedBy.Valid && !row.UpdatedBy.Valid && !row.CreatedAt.Valid && !row.UpdatedAt.Valid && len(row.Tags) == 0
}

func attachmentStoreUnavailable(err error) error {
	if err != nil {
		return err
	}
	return mediaapp.ErrAttachmentUnavailable
}
