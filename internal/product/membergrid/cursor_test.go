package membergrid

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTripIsOpaqueAndBindsEveryQueryDimension(t *testing.T) {
	codec, err := newCursorCodec(bytes.Repeat([]byte("s"), 32), &incrementingReader{})
	if err != nil {
		t.Fatal(err)
	}
	position := Position{UpdatedAt: time.Date(2026, 8, 24, 9, 10, 11, 123000000, time.UTC), MemberRef: "spm_0000000000000000000001"}
	token, err := codec.Encode(42, StateActive, SourceManual, 17, position)
	if err != nil {
		t.Fatal(err)
	}
	if token == position.MemberRef || token == base64.RawURLEncoding.EncodeToString([]byte(position.MemberRef)) {
		t.Fatalf("cursor exposed position: %q", token)
	}
	decoded, err := codec.Decode(token, 42, StateActive, SourceManual, 17)
	if err != nil || decoded.MemberRef != position.MemberRef || !decoded.UpdatedAt.Equal(position.UpdatedAt) {
		t.Fatalf("decoded=%+v error=%v", decoded, err)
	}
	for _, mismatch := range []struct {
		product int64
		state   StateFilter
		source  SourceFilter
		limit   int
	}{
		{43, StateActive, SourceManual, 17}, {42, StateExpired, SourceManual, 17}, {42, StateActive, SourcePaidOrder, 17}, {42, StateActive, SourceManual, 18},
	} {
		if _, err = codec.Decode(token, mismatch.product, mismatch.state, mismatch.source, mismatch.limit); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("mismatch=%+v error=%v", mismatch, err)
		}
	}
}

func TestCursorRejectsTamperingAndAmbiguousInputs(t *testing.T) {
	codec, err := newCursorCodec(bytes.Repeat([]byte("k"), 32), &incrementingReader{})
	if err != nil {
		t.Fatal(err)
	}
	token, err := codec.Encode(7, StateAll, SourceAny, 50, Position{UpdatedAt: time.Now().UTC(), MemberRef: "spm_0000000000000000000002"})
	if err != nil {
		t.Fatal(err)
	}
	tampered := token[:len(token)-1] + replacementByte(token[len(token)-1])
	for _, candidate := range []string{"", "spm_0000000000000000000002", "mg1.11", "mg0." + token[len(cursorPrefix):], token + "=", tampered, "mg2." + base64.RawURLEncoding.EncodeToString([]byte("too-short"))} {
		if _, err = codec.Decode(candidate, 7, StateAll, SourceAny, 50); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("Decode(%q) error=%v", candidate, err)
		}
	}
}

func TestCursorUsesFreshNonceAndRequiresManagedSecret(t *testing.T) {
	if _, err := NewCursorCodec(bytes.Repeat([]byte("x"), 31)); err == nil {
		t.Fatal("short secret unexpectedly accepted")
	}
	codec, err := newCursorCodec(bytes.Repeat([]byte("x"), 32), &incrementingReader{})
	if err != nil {
		t.Fatal(err)
	}
	position := Position{UpdatedAt: time.Now().UTC(), MemberRef: "spm_0000000000000000000003"}
	first, err := codec.Encode(9, StateAll, SourceAny, 50, position)
	if err != nil {
		t.Fatal(err)
	}
	second, err := codec.Encode(9, StateAll, SourceAny, 50, position)
	if err != nil || first == second {
		t.Fatalf("second=%q error=%v", second, err)
	}
}

func replacementByte(value byte) string {
	if value == 'A' {
		return "B"
	}
	return "A"
}

type incrementingReader struct{ next byte }

func (reader *incrementingReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		reader.next++
		buffer[index] = reader.next
	}
	return len(buffer), nil
}
