package app

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestChannelEntrantsCursorRoundTripBindingTamperAndExpiry(t *testing.T) {
	t.Parallel()

	clock := time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC)
	codec, err := newChannelEntrantsCursorCodec(
		bytes.Repeat([]byte("cursor-test-key-"), 3),
		bytes.NewReader(make([]byte, 256)),
		func() time.Time { return clock },
		2*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	position := ChannelEntrantsPosition{
		AddedAt:    time.Date(2026, 8, 21, 23, 59, 58, 123456000, time.UTC),
		CustomerID: 903,
	}
	token, err := codec.Encode(71, position)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, channelEntrantsCursorPrefix) || len(token) > channelEntrantsMaximumCursorLength {
		t.Fatalf("unexpected token shape: %q", token)
	}
	decoded, err := codec.Decode(token, 71)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.CustomerID != position.CustomerID || !decoded.AddedAt.Equal(position.AddedAt) {
		t.Fatalf("decoded=%#v want=%#v", decoded, position)
	}
	if _, err = codec.Decode(token, 72); !errors.Is(err, ErrInvalidChannelEntrantsCursor) {
		t.Fatalf("cross-channel cursor error=%v", err)
	}

	tampered := token[:len(token)-1] + alternateChannelEntrantsCursorCharacter(token[len(token)-1])
	if _, err = codec.Decode(tampered, 71); !errors.Is(err, ErrInvalidChannelEntrantsCursor) {
		t.Fatalf("tampered cursor error=%v", err)
	}
	clock = clock.Add(2 * time.Minute)
	if _, err = codec.Decode(token, 71); !errors.Is(err, ErrInvalidChannelEntrantsCursor) {
		t.Fatalf("expired cursor error=%v", err)
	}
}

func TestChannelEntrantsCursorRejectsMalformedInputsAndDependencyFailures(t *testing.T) {
	t.Parallel()

	if _, err := NewChannelEntrantsCursorCodec([]byte("short")); err == nil {
		t.Fatal("short cursor secret was accepted")
	}
	if _, err := newChannelEntrantsCursorCodec(
		bytes.Repeat([]byte("x"), channelEntrantsMinimumCursorKey),
		nil,
		time.Now,
		time.Minute,
	); err == nil {
		t.Fatal("nil random reader was accepted")
	}

	clock := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	codec, err := newChannelEntrantsCursorCodec(
		bytes.Repeat([]byte("y"), channelEntrantsMinimumCursorKey),
		bytes.NewReader(make([]byte, 128)),
		func() time.Time { return clock },
		time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	badInputs := []struct {
		name      string
		token     string
		channelID int64
	}{
		{name: "empty", token: "", channelID: 1},
		{name: "wrong prefix", token: "bad.AAAA", channelID: 1},
		{name: "padding", token: channelEntrantsCursorPrefix + "AAAA=", channelID: 1},
		{name: "too long", token: channelEntrantsCursorPrefix + strings.Repeat("A", channelEntrantsMaximumCursorLength), channelID: 1},
		{name: "zero channel", token: channelEntrantsCursorPrefix + "AAAA", channelID: 0},
	}
	for _, testCase := range badInputs {
		t.Run(testCase.name, func(t *testing.T) {
			if _, decodeErr := codec.Decode(testCase.token, testCase.channelID); !errors.Is(decodeErr, ErrInvalidChannelEntrantsCursor) {
				t.Fatalf("error=%v", decodeErr)
			}
		})
	}
	if _, err = codec.Encode(0, ChannelEntrantsPosition{AddedAt: clock, CustomerID: 1}); !errors.Is(err, ErrInvalidChannelEntrantsCursor) {
		t.Fatalf("zero channel encode error=%v", err)
	}
	if _, err = codec.Encode(1, ChannelEntrantsPosition{CustomerID: 1}); !errors.Is(err, ErrInvalidChannelEntrantsCursor) {
		t.Fatalf("zero timestamp encode error=%v", err)
	}

	failing, err := newChannelEntrantsCursorCodec(
		bytes.Repeat([]byte("z"), channelEntrantsMinimumCursorKey),
		channelEntrantsFailingReader{},
		func() time.Time { return clock },
		time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = failing.Encode(1, ChannelEntrantsPosition{AddedAt: clock, CustomerID: 2}); !errors.Is(err, ErrChannelEntrantsUnavailable) {
		t.Fatalf("random failure error=%v", err)
	}
}

type channelEntrantsFailingReader struct{}

func (channelEntrantsFailingReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func alternateChannelEntrantsCursorCharacter(value byte) string {
	if value == 'A' {
		return "B"
	}
	return "A"
}
