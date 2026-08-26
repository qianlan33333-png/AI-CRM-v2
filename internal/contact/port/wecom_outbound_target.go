package port

import "context"

// WeComOutboundTargetResolver returns the active owner's WeCom userid and one
// verified external identity. It is intentionally a narrow read boundary: the
// caller decides whether that owner is authorised for its own workflow.
type WeComOutboundTargetResolver interface {
	Resolve(context.Context, int64) (senderUserID, externalUserID string, resolved bool, err error)
}
