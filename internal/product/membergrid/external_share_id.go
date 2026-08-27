package membergrid

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

type RandomExternalShareIDFactory struct{ reader io.Reader }

func NewRandomExternalShareIDFactory() *RandomExternalShareIDFactory {
	return &RandomExternalShareIDFactory{reader: rand.Reader}
}

func (factory *RandomExternalShareIDFactory) NewExternalShareID(ctx context.Context) (string, error) {
	if factory == nil || factory.reader == nil || ctx == nil {
		return "", ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", errors.Join(ErrUnavailable, err)
	}
	raw := make([]byte, 24)
	if _, err := io.ReadFull(factory.reader, raw); err != nil {
		return "", errors.Join(ErrUnavailable, err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
