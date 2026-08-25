package client

import (
	"context"
	"errors"
)

// ErrExternalContactReadDisabled is returned by the explicit disabled adapter.
// It performs no token or HTTP request.
var ErrExternalContactReadDisabled = errors.New("WeCom external contact read is disabled")

// DisabledExternalContactReader is a production-safe composition option when
// credentials or the controlled sync schedule are unavailable.
type DisabledExternalContactReader struct{}

func NewDisabledExternalContactReader() *DisabledExternalContactReader {
	return &DisabledExternalContactReader{}
}

func (*DisabledExternalContactReader) ListExternalContacts(context.Context, string, string) (ExternalContactPage, error) {
	return ExternalContactPage{}, ErrExternalContactReadDisabled
}
