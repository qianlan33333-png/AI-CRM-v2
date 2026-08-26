package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTemporaryMediaServicePreparesExactEnabledImageOncePerIdempotencyKey(t *testing.T) {
	store := &temporaryMediaReceiptMemory{receipts: make(map[string]SidebarImageSendReceipt)}
	uploader := &temporaryMediaUploaderMemory{result: SidebarTemporaryMediaUploadResult{
		MediaID: "media-1", ExpiresAt: time.Now().UTC().Add(time.Hour), ProviderCallDispatched: true,
	}}
	service, err := NewTemporaryMediaService(store, temporaryMediaImageMemory{image: SidebarImageSource{
		ImageID: 7, Enabled: true, Filename: "real.png", MIME: "image/png", Content: []byte("real-image-binary"),
	}}, uploader)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.PrepareTemporaryImageMedia(context.Background(), 7, 9, 11, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.UploadState != "ready" || first.MediaID != "media-1" || !first.ProviderCallDispatched || uploader.calls != 1 || uploader.input.Filename != "real.png" || string(uploader.input.Bytes) != "real-image-binary" {
		t.Fatalf("first result=%+v calls=%d input=%+v", first, uploader.calls, uploader.input)
	}
	replay, err := service.PrepareTemporaryImageMedia(context.Background(), 7, 9, 11, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if replay != first || uploader.calls != 1 {
		t.Fatalf("replay=%+v first=%+v calls=%d", replay, first, uploader.calls)
	}
	if _, err = service.PrepareTemporaryImageMedia(context.Background(), 8, 9, 11, "key-1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("different image with same key error=%v", err)
	}
}

func TestTemporaryMediaServiceDoesNotRetryUnknownOrDisabledImage(t *testing.T) {
	store := &temporaryMediaReceiptMemory{receipts: make(map[string]SidebarImageSendReceipt)}
	uploader := &temporaryMediaUploaderMemory{
		result: SidebarTemporaryMediaUploadResult{ProviderCallDispatched: true, OutcomeUnknown: true},
		err:    errors.New("transport interrupted"),
	}
	service, err := NewTemporaryMediaService(store, temporaryMediaImageMemory{image: SidebarImageSource{
		ImageID: 7, Enabled: true, Filename: "real.png", MIME: "image/png", Content: []byte("real-image-binary"),
	}}, uploader)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.PrepareTemporaryImageMedia(context.Background(), 7, 9, 11, "key-unknown")
	if err != nil {
		t.Fatal(err)
	}
	if first.UploadState != "outcome_unknown" || !first.ProviderCallDispatched || uploader.calls != 1 {
		t.Fatalf("unknown result=%+v calls=%d", first, uploader.calls)
	}
	if _, err = service.PrepareTemporaryImageMedia(context.Background(), 7, 9, 11, "key-unknown"); err != nil || uploader.calls != 1 {
		t.Fatalf("unknown replay error=%v calls=%d", err, uploader.calls)
	}

	disabledStore := &temporaryMediaReceiptMemory{receipts: make(map[string]SidebarImageSendReceipt)}
	disabledUploader := &temporaryMediaUploaderMemory{}
	disabled, err := NewTemporaryMediaService(disabledStore, temporaryMediaImageMemory{image: SidebarImageSource{
		ImageID: 8, Enabled: false, Filename: "real.png", MIME: "image/png", Content: []byte("real-image-binary"),
	}}, disabledUploader)
	if err != nil {
		t.Fatal(err)
	}
	result, err := disabled.PrepareTemporaryImageMedia(context.Background(), 8, 9, 11, "key-disabled")
	if err != nil || result.UploadState != "final_failed" || result.ProviderCallDispatched || disabledUploader.calls != 0 {
		t.Fatalf("disabled result=%+v error=%v calls=%d", result, err, disabledUploader.calls)
	}

	preDispatchStore := &temporaryMediaReceiptMemory{receipts: make(map[string]SidebarImageSendReceipt)}
	preDispatchUploader := &temporaryMediaUploaderMemory{err: errors.New("token unavailable")}
	preDispatch, err := NewTemporaryMediaService(preDispatchStore, temporaryMediaImageMemory{image: SidebarImageSource{
		ImageID: 9, Enabled: true, Filename: "real.png", MIME: "image/png", Content: []byte("real-image-binary"),
	}}, preDispatchUploader)
	if err != nil {
		t.Fatal(err)
	}
	result, err = preDispatch.PrepareTemporaryImageMedia(context.Background(), 9, 9, 11, "key-pre-dispatch")
	if err != nil || result.UploadState != "final_failed" || result.ProviderCallDispatched || preDispatchUploader.calls != 1 {
		t.Fatalf("pre-dispatch result=%+v error=%v calls=%d", result, err, preDispatchUploader.calls)
	}
}

type temporaryMediaReceiptMemory struct {
	nextID   int64
	receipts map[string]SidebarImageSendReceipt
}

func (store *temporaryMediaReceiptMemory) ReserveSidebarImageSend(_ context.Context, _, _, imageID int64, digest [32]byte) (SidebarImageSendReceipt, bool, error) {
	key := string(digest[:])
	if receipt, ok := store.receipts[key]; ok {
		return receipt, false, nil
	}
	store.nextID++
	receipt := SidebarImageSendReceipt{ID: store.nextID, ImageID: imageID, State: "pending"}
	store.receipts[key] = receipt
	return receipt, true, nil
}

func (store *temporaryMediaReceiptMemory) CompleteSidebarImageSend(_ context.Context, id int64, state, mediaID string, expiresAt time.Time, dispatched bool) (SidebarImageSendReceipt, error) {
	for key, receipt := range store.receipts {
		if receipt.ID == id && receipt.State == "pending" {
			receipt.State, receipt.MediaID, receipt.MediaExpiresAt, receipt.ProviderCallDispatched = state, mediaID, expiresAt, dispatched
			store.receipts[key] = receipt
			return receipt, nil
		}
	}
	return SidebarImageSendReceipt{}, ErrUnavailable
}

type temporaryMediaImageMemory struct{ image SidebarImageSource }

func (source temporaryMediaImageMemory) ReadSidebarImage(_ context.Context, _ int64) (SidebarImageSource, error) {
	return source.image, nil
}

type temporaryMediaUploaderMemory struct {
	input  SidebarTemporaryMediaUpload
	result SidebarTemporaryMediaUploadResult
	err    error
	calls  int
}

func (uploader *temporaryMediaUploaderMemory) UploadSidebarImageTemporaryMedia(_ context.Context, input SidebarTemporaryMediaUpload) (SidebarTemporaryMediaUploadResult, error) {
	uploader.calls++
	uploader.input = input
	return uploader.result, uploader.err
}
