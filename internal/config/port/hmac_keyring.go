package port

import (
	"context"
	"errors"
)

type HMACPurpose string
type HMACKeyVersion uint16
type HMACDigest [32]byte

const (
	HMACPurposeIdentityFingerprint    HMACPurpose = "identity_fingerprint"
	HMACPurposeIdentityReceiptPayload HMACPurpose = "identity_receipt_payload"
)

var (
	ErrSecretUnavailable        = errors.New("typed secret unavailable")
	ErrSecretVersionUnavailable = errors.New("typed secret version unavailable")
)

// HMACKeyring is the only Identity access to key material. Implementations may
// load keys from deployment environment or read-only mounted secret files, but
// must never expose key bytes, persist them in settings, or include them in an
// error. Historical versions remain verify-only while persistent rows refer to
// them.
type HMACKeyring interface {
	CurrentVersion(context.Context, HMACPurpose) (HMACKeyVersion, error)
	Sign(context.Context, HMACPurpose, HMACKeyVersion, []byte) (HMACDigest, error)
	Verify(context.Context, HMACPurpose, HMACKeyVersion, []byte, HMACDigest) (bool, error)
}
