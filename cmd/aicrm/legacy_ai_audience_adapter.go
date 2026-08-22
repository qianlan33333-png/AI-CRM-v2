package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
)

type legacyAIAudienceSecurity struct{}

func (legacyAIAudienceSecurity) Authorize(request *http.Request, requirement legacyaudience.AccessRequirement) (legacyaudience.Actor, error) {
	if request == nil {
		return legacyaudience.Actor{}, legacyaudience.ErrUnauthenticated
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !principalOK || principal.AdminUserID < 1 {
		return legacyaudience.Actor{}, legacyaudience.ErrUnauthenticated
	}
	if !authorizationOK || authorization.Scope != authport.ScopeGlobal || string(authorization.Capability) != requirement.Capability {
		return legacyaudience.Actor{}, legacyaudience.ErrForbidden
	}
	if requirement.Capability != legacyaudience.CapabilitySegmentsRead && requirement.Capability != legacyaudience.CapabilitySegmentsWrite {
		return legacyaudience.Actor{}, legacyaudience.ErrForbidden
	}
	if requirement.RequireCSRF && authorization.Capability != authport.CapabilitySegmentsWrite {
		return legacyaudience.Actor{}, errors.Join(legacyaudience.ErrCSRFInvalid, legacyaudience.ErrForbidden)
	}
	return legacyaudience.Actor{AdminUserID: principal.AdminUserID}, nil
}

type legacyAIAudienceSQLProvider struct{ pool *pgxpool.Pool }

func (provider legacyAIAudienceSQLProvider) Reader(context.Context) (legacyaudience.SQLExecutor, error) {
	if provider.pool == nil {
		return nil, legacyaudience.ErrUnavailable
	}
	return legacyAIAudienceExecutor{queryer: provider.pool, execer: provider.pool}, nil
}

func (legacyAIAudienceSQLProvider) Transaction(ctx context.Context) (legacyaudience.SQLExecutor, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return legacyAIAudienceExecutor{queryer: tx, execer: tx}, nil
}

func (legacyAIAudienceSQLProvider) IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

type legacyAIAudienceQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type legacyAIAudienceExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type legacyAIAudienceExecutor struct {
	queryer legacyAIAudienceQueryer
	execer  legacyAIAudienceExecer
}

func (executor legacyAIAudienceExecutor) Exec(ctx context.Context, sql string, arguments ...any) (int64, error) {
	tag, err := executor.execer.Exec(ctx, sql, arguments...)
	return tag.RowsAffected(), err
}

func (executor legacyAIAudienceExecutor) Query(ctx context.Context, sql string, arguments ...any) (legacyaudience.SQLRows, error) {
	return executor.queryer.Query(ctx, sql, arguments...)
}

func (executor legacyAIAudienceExecutor) QueryRow(ctx context.Context, sql string, arguments ...any) legacyaudience.SQLRow {
	return executor.queryer.QueryRow(ctx, sql, arguments...)
}

type legacyAIAudienceEventAppender struct{ appender eventport.Appender }

func (adapter legacyAIAudienceEventAppender) Append(ctx context.Context, event legacyaudience.LocalEvent) error {
	if adapter.appender == nil {
		return legacyaudience.ErrUnavailable
	}
	_, err := adapter.appender.Append(ctx, eventport.Event{
		Type: event.Type, Payload: event.Payload, OccurredAt: event.OccurredAt, IdempotencyKey: event.IdempotencyKey,
	})
	return err
}
