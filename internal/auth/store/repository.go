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

type OAuthStateClaim struct {
	NextPath string
}

type AdminAccessMember struct {
	AdminUserID      int64
	DisplayName      string
	Role             string
	StaffID          *int64
	StaffWeComUserID string
	StaffName        string
	IsActive         bool
	LoginEnabled     bool
}

type AdminAccessSaveResult struct {
	AdminUserID  int64
	LoginEnabled bool
}

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) FindVerifiedLogin(ctx context.Context, login authport.VerifiedLogin) (LoginUser, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return LoginUser{}, err
	}
	row, err := queries.FindAdminUserForVerifiedLogin(ctx, authdb.FindAdminUserForVerifiedLoginParams{
		AuthProvider: string(login.Provider), WecomCorpID: login.CorpID,
		ProviderSubjectID: login.SubjectID,
	})
	if err != nil {
		return LoginUser{}, err
	}
	return LoginUser{
		Principal:      authport.Principal{AdminUserID: row.ID, Role: authport.Role(row.Role), StaffID: positiveInt64Pointer(row.StaffID)},
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
	return authport.Principal{AdminUserID: row.ID, Role: authport.Role(row.Role), StaffID: positiveInt64Pointer(row.StaffID)}, nil
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

func (repository *Repository) InsertOAuthState(ctx context.Context, stateHash []byte, provider authport.Provider, nextPath string, createdAt, expiresAt time.Time) error {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return err
	}
	return queries.InsertAdminOAuthState(ctx, authdb.InsertAdminOAuthStateParams{
		StateHash: stateHash, AuthProvider: string(provider), NextPath: nextPath,
		CreatedAt: timestamp(createdAt), ExpiresAt: timestamp(expiresAt),
	})
}

func (repository *Repository) ClaimOAuthState(ctx context.Context, stateHash []byte, provider authport.Provider, claimedAt time.Time) (OAuthStateClaim, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return OAuthStateClaim{}, err
	}
	row, err := queries.ClaimAdminOAuthState(ctx, authdb.ClaimAdminOAuthStateParams{
		StateHash: stateHash, AuthProvider: string(provider), ClaimedAt: timestamp(claimedAt),
	})
	if err != nil {
		return OAuthStateClaim{}, err
	}
	return OAuthStateClaim{NextPath: row}, nil
}

func (repository *Repository) ListAdminAccessMembers(ctx context.Context) ([]AdminAccessMember, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAdminAccessMembers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]AdminAccessMember, len(rows))
	for index, row := range rows {
		result[index] = AdminAccessMember{
			AdminUserID: row.ID, DisplayName: row.DisplayName, Role: row.Role, StaffID: int64Pointer(row.StaffID),
			StaffWeComUserID: row.StaffWecomUserid, StaffName: row.StaffName, IsActive: row.IsActive, LoginEnabled: row.LoginEnabled,
		}
	}
	return result, nil
}

func (repository *Repository) SaveAdminAccessMember(ctx context.Context, adminUserID int64, loginEnabled bool, updatedAt time.Time) (AdminAccessSaveResult, error) {
	queries, err := queriesFromContext(ctx)
	if err != nil {
		return AdminAccessSaveResult{}, err
	}
	row, err := queries.SaveAdminAccessMember(ctx, authdb.SaveAdminAccessMemberParams{
		ID: adminUserID, LoginEnabled: loginEnabled, UpdatedAt: timestamp(updatedAt),
	})
	if err != nil {
		return AdminAccessSaveResult{}, err
	}
	return AdminAccessSaveResult{AdminUserID: row.ID, LoginEnabled: row.LoginEnabled}, nil
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

func positiveInt64Pointer(value int64) *int64 {
	if value < 1 {
		return nil
	}
	result := value
	return &result
}
