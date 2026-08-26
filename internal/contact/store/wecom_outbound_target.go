package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

var ErrWeComOutboundTargetUnavailable = errors.New("WeCom outbound target unavailable")

type weComOutboundTargetQuerier interface {
	ResolveVerifiedWeComOutboundTarget(context.Context, contactdb.ResolveVerifiedWeComOutboundTargetParams) (contactdb.ResolveVerifiedWeComOutboundTargetRow, error)
}

// WeComOutboundTargetResolver returns only a unique verified identity in the
// configured enterprise and that customer's active owner. Missing or
// ambiguous business facts are unresolved, while database failures remain
// retryable infrastructure errors for the caller.
type WeComOutboundTargetResolver struct {
	queries weComOutboundTargetQuerier
	corpID  string
}

func NewWeComOutboundTargetResolver(pool *pgxpool.Pool, corpID string) (*WeComOutboundTargetResolver, error) {
	if pool == nil {
		return nil, ErrWeComOutboundTargetUnavailable
	}
	return newWeComOutboundTargetResolver(contactdb.New(pool), corpID)
}

func newWeComOutboundTargetResolver(queries weComOutboundTargetQuerier, corpID string) (*WeComOutboundTargetResolver, error) {
	if queries == nil || corpID == "" || len(corpID) > 128 || strings.TrimSpace(corpID) != corpID {
		return nil, ErrWeComOutboundTargetUnavailable
	}
	return &WeComOutboundTargetResolver{queries: queries, corpID: corpID}, nil
}

func (resolver *WeComOutboundTargetResolver) Resolve(ctx context.Context, customerID int64) (string, string, bool, error) {
	if resolver == nil || resolver.queries == nil || ctx == nil || ctx.Err() != nil || customerID < 1 {
		return "", "", false, ErrWeComOutboundTargetUnavailable
	}
	row, err := resolver.queries.ResolveVerifiedWeComOutboundTarget(ctx, contactdb.ResolveVerifiedWeComOutboundTargetParams{CustomerID: customerID, CorpID: resolver.corpID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, errors.Join(ErrWeComOutboundTargetUnavailable, err)
	}
	if !validWeComOutboundTarget(row.SenderWecomUserid, 128) || !validWeComOutboundTarget(row.ExternalUserid, 1024) {
		return "", "", false, ErrWeComOutboundTargetUnavailable
	}
	return row.SenderWecomUserid, row.ExternalUserid, true, nil
}

func validWeComOutboundTarget(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}
