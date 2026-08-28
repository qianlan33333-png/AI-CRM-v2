package port

import "context"

// HistoricalScopedWeComIdentityEvidence is a read-only projection. Keeping it
// separate preserves the existing DM01 command and receipt digest format.
type HistoricalScopedWeComIdentityEvidence struct {
	IdentityID     int64
	Scope          string
	ExternalUserID string
	Assurance      Assurance
	HMACKeyVersion int16
}

type HistoricalScopedWeComIdentityEvidenceReader interface {
	LockHistoricalScopedWeComIdentityEvidence(context.Context, int64, []byte) (HistoricalScopedWeComIdentityEvidence, error)
}
