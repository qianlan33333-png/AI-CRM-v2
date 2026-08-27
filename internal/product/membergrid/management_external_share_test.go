package membergrid

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestManagementExternalShareEnableReplayDisableAndReenable(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[8] = true
	service, codec := newManagementExternalShareService(t, store, []string{
		"share_abcdefghijklmnopqrstuv", "share_zyxwvutsrqponmlkjihgfedc",
	})
	ctx := context.Background()

	settings, err := service.ShareSettings(ctx, 8)
	if err != nil || !settings.ExternalShareSupported || settings.ExternalShareEnabled || settings.ExternalShareVersion != 0 {
		t.Fatalf("initial settings=%+v err=%v", settings, err)
	}

	enable := SetExternalShareCommand{ServiceProductID: 8, Enabled: true, ExpectedVersion: 0, ActorID: 3, IdempotencyKey: "external-share-enable-0001"}
	first, err := service.SetExternalShare(ctx, enable)
	if err != nil || !first.Share.Enabled || first.Share.Version != 1 || !first.TokenIssued || first.PublicToken == "" {
		t.Fatalf("first enable=%+v err=%v", first, err)
	}
	replayed, err := service.SetExternalShare(ctx, enable)
	if err != nil || replayed != first {
		t.Fatalf("enable replay=%+v err=%v want=%+v", replayed, err, first)
	}
	receiptKey := receiptMemoryKey(mutationOperationUpdate, "membergrid:"+snapshotExternalShareSet+":actor:3", sha256.Sum256([]byte(enable.IdempotencyKey)))
	receipt := store.receipts[receiptKey]
	if strings.Contains(string(receipt.ResultSnapshot), first.PublicToken) || strings.Contains(string(receipt.ResultSnapshot), externalShareTokenPrefix) {
		t.Fatalf("receipt stored bearer token: %s", receipt.ResultSnapshot)
	}

	sameState, err := service.SetExternalShare(ctx, SetExternalShareCommand{ServiceProductID: 8, Enabled: true, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-enable-0002"})
	if err != nil || sameState.Share != first.Share || sameState.TokenIssued || sameState.PublicToken != "" {
		t.Fatalf("same-state enable=%+v err=%v", sameState, err)
	}
	if _, err = service.SetExternalShare(ctx, SetExternalShareCommand{ServiceProductID: 8, Enabled: false, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-enable-0002"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same key different payload err=%v", err)
	}

	publicReader, err := NewExternalShareService(store, &managementShareIDFactory{}, codec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = publicReader.ResolvePublicExternalShare(ctx, first.PublicToken); err != nil {
		t.Fatalf("enabled public token err=%v", err)
	}

	disabled, err := service.SetExternalShare(ctx, SetExternalShareCommand{ServiceProductID: 8, Enabled: false, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-disable-001"})
	if err != nil || disabled.Share.Enabled || disabled.Share.ShareID != "" || disabled.Share.Version != 2 || disabled.TokenIssued || disabled.PublicToken != "" {
		t.Fatalf("disable=%+v err=%v", disabled, err)
	}
	if _, err = publicReader.ResolvePublicExternalShare(ctx, first.PublicToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled old token err=%v", err)
	}

	reenabled, err := service.SetExternalShare(ctx, SetExternalShareCommand{ServiceProductID: 8, Enabled: true, ExpectedVersion: 2, ActorID: 3, IdempotencyKey: "external-share-enable-0003"})
	if err != nil || !reenabled.Share.Enabled || reenabled.Share.Version != 3 || reenabled.Share.ShareID == first.Share.ShareID || !reenabled.TokenIssued || reenabled.PublicToken == "" {
		t.Fatalf("reenable=%+v err=%v", reenabled, err)
	}
	if _, err = publicReader.ResolvePublicExternalShare(ctx, first.PublicToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old token after re-enable err=%v", err)
	}
	if share, err := publicReader.ResolvePublicExternalShare(ctx, reenabled.PublicToken); err != nil || share != reenabled.Share {
		t.Fatalf("new token share=%+v err=%v", share, err)
	}
	settings, err = service.ShareSettings(ctx, 8)
	if err != nil || !settings.ExternalShareSupported || !settings.ExternalShareEnabled || settings.ExternalShareVersion != 3 {
		t.Fatalf("enabled settings=%+v err=%v", settings, err)
	}
}

func TestManagementExternalShareReceiptFailureRollsBackState(t *testing.T) {
	store := newManagementMemoryStore()
	store.products[8] = true
	store.failComplete = true
	service, _ := newManagementExternalShareService(t, store, []string{"share_abcdefghijklmnopqrstuv"})
	_, err := service.SetExternalShare(context.Background(), SetExternalShareCommand{
		ServiceProductID: 8, Enabled: true, ExpectedVersion: 0, ActorID: 3, IdempotencyKey: "external-share-rollback-01",
	})
	if !errors.Is(err, ErrUnavailable) || len(store.externalShares) != 0 || len(store.receipts) != 0 {
		t.Fatalf("err/shares/receipts=%v/%v/%v", err, store.externalShares, store.receipts)
	}
}

func newManagementExternalShareService(t *testing.T, store *managementMemoryStore, ids []string) (*ManagementService, *ExternalShareTokenCodec) {
	t.Helper()
	codec, err := NewExternalShareTokenCodec(bytes.Repeat([]byte("s"), minimumCursorKey))
	if err != nil {
		t.Fatal(err)
	}
	unit := &managementRollbackUOW{store: store}
	service, err := NewManagementServiceWithExternalShares(unit, store, &managementEventAppender{}, store, &managementShareIDFactory{ids: ids}, codec)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	var tick int64
	service.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Minute)
	}
	return service, codec
}

type managementShareIDFactory struct {
	ids  []string
	next int
}

func (factory *managementShareIDFactory) NewExternalShareID(context.Context) (string, error) {
	if factory.next >= len(factory.ids) {
		return "", errors.New("share id unavailable")
	}
	value := factory.ids[factory.next]
	factory.next++
	return value, nil
}
