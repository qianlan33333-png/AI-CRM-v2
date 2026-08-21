package membergrid

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTripIsServerOpaqueAndBound(t *testing.T) {
	codec, err := newCursorCodec(bytes.Repeat([]byte("s"), 32), &incrementingReader{})
	if err != nil {
		t.Fatal(err)
	}
	position := Position{GrantedAt: time.Date(2026, 8, 21, 9, 10, 11, 123000000, time.UTC), EntitlementID: 987654321}
	token, err := codec.Encode(42, StateActive, position)
	if err != nil {
		t.Fatal(err)
	}
	if token == "987654321" || token == base64.RawURLEncoding.EncodeToString([]byte("987654321")) {
		t.Fatalf("cursor exposed the client-visible last id: %q", token)
	}
	decoded, err := codec.Decode(token, 42, StateActive)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.EntitlementID != position.EntitlementID || !decoded.GrantedAt.Equal(position.GrantedAt) {
		t.Fatalf("decoded=%+v want=%+v", decoded, position)
	}
	if _, err = codec.Decode(token, 43, StateActive); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cross-product decode error=%v", err)
	}
	if _, err = codec.Decode(token, 42, StateRevoked); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cross-filter decode error=%v", err)
	}
}

func TestCursorRejectsTamperingAndAmbiguousInputs(t *testing.T) {
	codec, err := newCursorCodec(bytes.Repeat([]byte("k"), 32), &incrementingReader{})
	if err != nil {
		t.Fatal(err)
	}
	token, err := codec.Encode(7, StateAll, Position{GrantedAt: time.Now().UTC(), EntitlementID: 11})
	if err != nil {
		t.Fatal(err)
	}
	tampered := token[:len(token)-1] + replacementByte(token[len(token)-1])
	cases := []string{
		"", "11", "mg1.11", "mg0." + token[len(cursorPrefix):], token + "=", tampered,
		"mg1." + base64.RawURLEncoding.EncodeToString([]byte("too-short")),
	}
	for _, candidate := range cases {
		if _, decodeErr := codec.Decode(candidate, 7, StateAll); !errors.Is(decodeErr, ErrInvalidCursor) {
			t.Errorf("Decode(%q) error=%v, want ErrInvalidCursor", candidate, decodeErr)
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
	position := Position{GrantedAt: time.Now().UTC(), EntitlementID: 5}
	first, err := codec.Encode(9, StateAll, position)
	if err != nil {
		t.Fatal(err)
	}
	second, err := codec.Encode(9, StateAll, position)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two cursors reused the same nonce")
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
