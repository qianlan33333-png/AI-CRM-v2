package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type authStaffResolver struct{}

func (authStaffResolver) ResolveStaffID(ctx context.Context, adminUserID int64) (*int64, error) {
	if ctx == nil || adminUserID < 1 {
		return nil, errors.New("invalid admin user")
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var staffID int64
	err = tx.QueryRow(ctx, `
SELECT COALESCE(linked.id, inferred.id)
FROM public.admin_users AS account
LEFT JOIN public.staff AS linked
  ON linked.id = account.staff_id
 AND linked.is_active
LEFT JOIN public.staff AS inferred
  ON inferred.wecom_userid = account.provider_subject_id
 AND inferred.is_active
WHERE account.id = $1
  AND account.is_active
  AND account.login_enabled
  AND COALESCE(linked.id, inferred.id) IS NOT NULL`, adminUserID).Scan(&staffID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &staffID, nil
}
