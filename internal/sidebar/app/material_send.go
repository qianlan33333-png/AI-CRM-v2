package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"time"
)

type SidebarImageSource struct {
	ImageID  int64
	Enabled  bool
	Filename string
	MIME     string
	Content  []byte
}

type SidebarImageReader interface {
	ReadSidebarImage(context.Context, int64) (SidebarImageSource, error)
}

type SidebarTemporaryMediaUpload struct {
	Filename string
	MIME     string
	Bytes    []byte
	Checksum string
}

type SidebarTemporaryMediaUploadResult struct {
	MediaID                string
	ExpiresAt              time.Time
	ProviderCallDispatched bool
	OutcomeUnknown         bool
	FinalFailed            bool
}

type SidebarTemporaryMediaUploader interface {
	UploadSidebarImageTemporaryMedia(context.Context, SidebarTemporaryMediaUpload) (SidebarTemporaryMediaUploadResult, error)
}

type SidebarImageSendReceipt struct {
	ID                     int64
	ImageID                int64
	State                  string
	MediaID                string
	MediaExpiresAt         time.Time
	ProviderCallDispatched bool
}

type SidebarImageSendReceiptStore interface {
	ReserveSidebarImageSend(context.Context, int64, int64, int64, [32]byte) (SidebarImageSendReceipt, bool, error)
	CompleteSidebarImageSend(context.Context, int64, string, string, time.Time, bool) (SidebarImageSendReceipt, error)
}

type TemporaryMediaService struct {
	receipts SidebarImageSendReceiptStore
	images   SidebarImageReader
	uploader SidebarTemporaryMediaUploader
}

func NewTemporaryMediaService(receipts SidebarImageSendReceiptStore, images SidebarImageReader, uploader SidebarTemporaryMediaUploader) (*TemporaryMediaService, error) {
	if nilSidebarDependency(receipts) || nilSidebarDependency(images) || nilSidebarDependency(uploader) {
		return nil, ErrUnavailable
	}
	return &TemporaryMediaService{receipts: receipts, images: images, uploader: uploader}, nil
}

func (service *TemporaryMediaService) PrepareTemporaryImageMedia(ctx context.Context, imageID, actorID, customerID int64, idempotencyKey string) (TemporaryMediaResult, error) {
	if service == nil || ctx == nil || imageID < 1 || actorID < 1 || customerID < 1 || idempotencyKey == "" {
		return TemporaryMediaResult{}, ErrUnavailable
	}
	keyDigest := sha256.Sum256([]byte(idempotencyKey))
	receipt, owned, err := service.receipts.ReserveSidebarImageSend(ctx, actorID, customerID, imageID, keyDigest)
	if err != nil {
		return TemporaryMediaResult{}, err
	}
	if receipt.ImageID != imageID {
		return TemporaryMediaResult{}, ErrConflict
	}
	if !owned {
		return temporaryMediaResultFromReceipt(receipt), nil
	}

	image, err := service.images.ReadSidebarImage(ctx, imageID)
	if err != nil || image.ImageID != imageID || !image.Enabled || image.Filename == "" || image.MIME == "" || len(image.Content) == 0 {
		receipt, completeErr := service.receipts.CompleteSidebarImageSend(ctx, receipt.ID, "final_failed", "", time.Time{}, false)
		if completeErr != nil {
			return TemporaryMediaResult{}, completeErr
		}
		return temporaryMediaResultFromReceipt(receipt), nil
	}
	sum := sha256.Sum256(image.Content)
	upload, uploadErr := service.uploader.UploadSidebarImageTemporaryMedia(ctx, SidebarTemporaryMediaUpload{
		Filename: image.Filename, MIME: image.MIME, Bytes: image.Content, Checksum: "sha256:" + hex.EncodeToString(sum[:]),
	})
	state := "ready"
	if upload.OutcomeUnknown || (uploadErr != nil && upload.ProviderCallDispatched) {
		state = "outcome_unknown"
	} else if uploadErr != nil || upload.FinalFailed || upload.MediaID == "" || upload.ExpiresAt.IsZero() || !upload.ExpiresAt.After(time.Now().UTC()) {
		state = "final_failed"
	}
	mediaID, expiresAt := upload.MediaID, upload.ExpiresAt
	if state != "ready" {
		mediaID, expiresAt = "", time.Time{}
	}
	receipt, err = service.receipts.CompleteSidebarImageSend(ctx, receipt.ID, state, mediaID, expiresAt, upload.ProviderCallDispatched)
	if err != nil {
		return TemporaryMediaResult{}, err
	}
	return temporaryMediaResultFromReceipt(receipt), nil
}

func temporaryMediaResultFromReceipt(receipt SidebarImageSendReceipt) TemporaryMediaResult {
	state := receipt.State
	if state == "pending" {
		state = "outcome_unknown"
	}
	if state == "ready" && !receipt.MediaExpiresAt.After(time.Now().UTC()) {
		return TemporaryMediaResult{ImageID: receipt.ImageID, UploadState: "final_failed", ProviderCallDispatched: receipt.ProviderCallDispatched}
	}
	return TemporaryMediaResult{
		ImageID: receipt.ImageID, MediaID: receipt.MediaID, MediaExpiresAt: receipt.MediaExpiresAt,
		UploadState: state, ProviderCallDispatched: receipt.ProviderCallDispatched,
	}
}

func nilSidebarDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Ptr && reflected.IsNil()
}
