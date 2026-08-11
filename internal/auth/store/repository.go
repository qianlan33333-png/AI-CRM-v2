package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authdb "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

type LoginUser struct {
	Principal      authport.Principal
	SessionVersion int64
}

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) FindVerifiedLogin(ctx context.Context, login authport.VerifiedLogin) (LoginUser, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return LoginUser{}, err
	}
	row, err := queries.FindAdminUserForVerifiedLogin(ctx, authdb.FindAdminUserForVerifiedLoginParams{
		AuthProvider: string(login.Provider), ProviderTenantID: login.TenantID,
		ProviderSubjectID: login.SubjectID,
	})
	if err != nil {
		return LoginUser{}, err
	}
	return LoginUser{
		Principal:      authport.Principal{AdminUserID: row.ID, Role: authport.Role(row.Role), StaffID: int64Pointer(row.StaffID)},
		SessionVersion: row.SessionVersion,
	}, nil
}

func (repository *Repository) InsertSession(ctx context.Context, tokenHash, csrfHash []byte, user LoginUser, authTime, expiresAt time.Time) error {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return err
	}
	return queries.InsertAdminSession(ctx, authdb.InsertAdminSessionParams{
		SessionTokenHash: tokenHash, CsrfTokenHash: csrfHash,
		AdminUserID: user.Principal.AdminUserID, SessionVersion: user.SessionVersion,
		AuthTime: timestamp(authTime), ExpiresAt: timestamp(expiresAt),
	})
}

func (repository *Repository) GetActive(ctx context.Context, tokenHash []byte, now time.Time) (authport.Principal, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return authport.Principal{}, err
	}
	row, err := queries.GetActiveSession(ctx, authdb.GetActiveSessionParams{SessionTokenHash: tokenHash, Now: timestamp(now)})
	if err != nil {
		return authport.Principal{}, err
	}
	return authport.Principal{AdminUserID: row.ID, Role: authport.Role(row.Role), StaffID: int64Pointer(row.StaffID)}, nil
}

func (repository *Repository) ValidateCSRF(ctx context.Context, tokenHash, csrfHash []byte, now time.Time) (bool, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return false, err
	}
	return queries.ValidateSessionCSRF(ctx, authdb.ValidateSessionCSRFParams{
		SessionTokenHash: tokenHash, CsrfTokenHash: csrfHash, Now: timestamp(now),
	})
}

func (repository *Repository) Revoke(ctx context.Context, tokenHash, csrfHash []byte, revokedAt time.Time) error {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return err
	}
	_, err = queries.RevokeSession(ctx, authdb.RevokeSessionParams{
		SessionTokenHash: tokenHash, CsrfTokenHash: csrfHash, RevokedAt: timestamp(revokedAt),
	})
	return err
}

func queriesFromContext(ctx context.Context) (*authdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return authdb.New(tx), nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func int64Pointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
