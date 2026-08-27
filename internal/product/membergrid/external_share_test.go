package membergrid

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
)

func TestExternalShareTokenUsesDomainSeparatedConstantTimeVerification(t *testing.T) {
	secret := bytes.Repeat([]byte("s"), minimumCursorKey)
	codec, err := NewExternalShareTokenCodec(secret)
	if err != nil {
		t.Fatal(err)
	}
	shareID := "share_abcdefghijklmnopqrstuv"
	token, err := codec.Issue(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := codec.Verify(token); err != nil || got != shareID {
		t.Fatalf("Verify()=%q err=%v", got, err)
	}
	parts := []string{externalShareTokenPrefix, shareID, ""}
	rawMAC := hmac.New(sha256.New, secret)
	_, _ = rawMAC.Write([]byte(externalShareTokenPrefix + "\x00" + shareID))
	parts[2] = base64.RawURLEncoding.EncodeToString(rawMAC.Sum(nil))
	if _, err := codec.Verify(parts[0] + "." + parts[1] + "." + parts[2]); !errors.Is(err, ErrInvalidExternalShareToken) {
		t.Fatalf("raw-root token err=%v, want domain-separated rejection", err)
	}
	if _, err := codec.Verify(token[:len(token)-1] + "x"); !errors.Is(err, ErrInvalidExternalShareToken) {
		t.Fatalf("tampered token err=%v", err)
	}
}

func TestExternalShareServiceDisablesAndReenablesWithNewShareID(t *testing.T) {
	codec, err := NewExternalShareTokenCodec(bytes.Repeat([]byte("s"), minimumCursorKey))
	if err != nil {
		t.Fatal(err)
	}
	store := &externalShareStub{current: ExternalShare{ServiceProductID: 8, Version: 0}}
	service, err := NewExternalShareService(store, &externalShareIDStub{ids: []string{
		"share_abcdefghijklmnopqrstuv", "share_zyxwvutsrqponmlkjihgfedc",
	}}, codec)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	enabled, err := service.SetExternalShare(ctx, SetExternalShareCommand{ServiceProductID: 8, Enabled: true, ExpectedVersion: 0, ActorID: 3, IdempotencyKey: "external-share-enable-0001"})
	if err != nil || !enabled.Share.Enabled || !enabled.TokenIssued || enabled.PublicToken == "" || enabled.Share.Version != 1 {
		t.Fatalf("enable result=%+v err=%v", enabled, err)
	}
	if _, err = service.ResolvePublicExternalShare(ctx, enabled.PublicToken); err != nil {
		t.Fatalf("new token did not resolve: %v", err)
	}
	disabled, err := service.SetExternalShare(ctx, SetExternalShareCommand{ServiceProductID: 8, Enabled: false, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-disable-001"})
	if err != nil || disabled.Share.Enabled || disabled.Share.ShareID != "" || disabled.TokenIssued || disabled.PublicToken != "" || disabled.Share.Version != 2 {
		t.Fatalf("disable result=%+v err=%v", disabled, err)
	}
	if _, err = service.ResolvePublicExternalShare(ctx, enabled.PublicToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled old token err=%v, want not found", err)
	}
	reenabled, err := service.SetExternalShare(ctx, SetExternalShareCommand{ServiceProductID: 8, Enabled: true, ExpectedVersion: 2, ActorID: 3, IdempotencyKey: "external-share-enable-0002"})
	if err != nil || !reenabled.Share.Enabled || !reenabled.TokenIssued || reenabled.Share.ShareID == enabled.Share.ShareID || reenabled.Share.Version != 3 {
		t.Fatalf("re-enable result=%+v err=%v", reenabled, err)
	}
	if _, err = service.ResolvePublicExternalShare(ctx, enabled.PublicToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old token after re-enable err=%v, want not found", err)
	}
	if share, err := service.ResolvePublicExternalShare(ctx, reenabled.PublicToken); err != nil || share.ShareID != reenabled.Share.ShareID {
		t.Fatalf("new token share=%+v err=%v", share, err)
	}
}

func TestExternalShareServiceRejectsCASConflictAndDoesNotIssueToken(t *testing.T) {
	codec, err := NewExternalShareTokenCodec(bytes.Repeat([]byte("s"), minimumCursorKey))
	if err != nil {
		t.Fatal(err)
	}
	store := &externalShareStub{current: ExternalShare{ServiceProductID: 8, Version: 2}}
	ids := &externalShareIDStub{ids: []string{"share_abcdefghijklmnopqrstuv"}}
	service, err := NewExternalShareService(store, ids, codec)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SetExternalShare(context.Background(), SetExternalShareCommand{ServiceProductID: 8, Enabled: true, ExpectedVersion: 1, ActorID: 3, IdempotencyKey: "external-share-enable-0001"})
	if !errors.Is(err, ErrConflict) || ids.calls != 0 || store.setCalls != 0 {
		t.Fatalf("err=%v id_calls=%d store_calls=%d", err, ids.calls, store.setCalls)
	}
}

type externalShareIDStub struct {
	ids   []string
	calls int
}

func (stub *externalShareIDStub) NewExternalShareID(context.Context) (string, error) {
	if stub.calls >= len(stub.ids) {
		return "", errors.New("no share id")
	}
	value := stub.ids[stub.calls]
	stub.calls++
	return value, nil
}

type externalShareStub struct {
	current  ExternalShare
	setCalls int
}

func (stub *externalShareStub) CurrentExternalShare(_ context.Context, productID int64) (ExternalShare, error) {
	if stub.current.ServiceProductID != productID {
		return ExternalShare{}, ErrNotFound
	}
	return cloneExternalShare(stub.current), nil
}

func (stub *externalShareStub) SetExternalShare(_ context.Context, record SetExternalShareRecord) (ExternalShare, error) {
	stub.setCalls++
	if record.ServiceProductID != stub.current.ServiceProductID || record.ExpectedVersion != stub.current.Version {
		return ExternalShare{}, ErrConflict
	}
	stub.current = ExternalShare{ServiceProductID: record.ServiceProductID, ShareID: record.ShareID, Enabled: record.Enabled, Version: record.ExpectedVersion + 1}
	return cloneExternalShare(stub.current), nil
}

func (stub *externalShareStub) LookupEnabledExternalShare(_ context.Context, shareID string) (ExternalShare, error) {
	if !stub.current.Enabled || stub.current.ShareID != shareID {
		return ExternalShare{}, ErrNotFound
	}
	return cloneExternalShare(stub.current), nil
}
