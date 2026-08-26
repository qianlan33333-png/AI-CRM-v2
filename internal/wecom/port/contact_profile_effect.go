package port

import (
	"context"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

// ContactProfileWriteRequest is the narrow outbound-owned provider request
// for the documented WeCom external-contact remark operation. It deliberately
// carries no credentials or arbitrary provider payload.
type ContactProfileWriteRequest struct {
	CorpID         string
	StaffUserID    string
	ExternalUserID string
	Remark         string
	Description    string
}

// ContactProfileWriter is implemented only by the outbound provider adapter.
// The EER result preserves whether network I/O may have occurred so unknown
// outcomes can be held for manual reconciliation instead of retried.
type ContactProfileWriter interface {
	WriteContactProfile(context.Context, ContactProfileWriteRequest) (eer.AdapterResult, error)
}
