package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	platform "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func (r *SenderConfigRepository) SaveSenderConfig(ctx context.Context, value hxc.SenderConfig) (hxc.SenderConfig, error) {
	tx, e := platform.TxFromContext(ctx)
	if e != nil {
		return hxc.SenderConfig{}, e
	}
	row := tx.QueryRow(ctx, `INSERT INTO hxc_sender_configs (id,sender_userid,display_name,priority,is_active) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO UPDATE SET sender_userid=EXCLUDED.sender_userid,display_name=EXCLUDED.display_name,priority=EXCLUDED.priority,is_active=EXCLUDED.is_active,updated_at=now() RETURNING id,sender_userid,display_name,priority,is_active,created_at,updated_at`, value.ID, value.SenderUserID, value.DisplayName, value.Priority, value.IsActive)
	if e = row.Scan(&value.ID, &value.SenderUserID, &value.DisplayName, &value.Priority, &value.IsActive, &value.CreatedAt, &value.UpdatedAt); e != nil {
		return hxc.SenderConfig{}, e
	}
	return value, nil
}
func (r *SenderConfigRepository) DeleteSenderConfig(ctx context.Context, senderUserID string) error {
	tx, e := platform.TxFromContext(ctx)
	if e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `DELETE FROM hxc_sender_configs WHERE sender_userid=$1`, senderUserID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return hxcapp.ErrConfigConflict
	}
	return nil
}
func (r *SenderConfigRepository) ReorderSenderConfigs(ctx context.Context, ids []string) ([]hxc.SenderConfig, error) {
	tx, e := platform.TxFromContext(ctx)
	if e != nil {
		return nil, e
	}
	for priority, id := range ids {
		tag, e := tx.Exec(ctx, `UPDATE hxc_sender_configs SET priority=$1,updated_at=now() WHERE id=$2`, priority, id)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, hxcapp.ErrConfigConflict
		}
	}
	return r.ListSenderConfigs(ctx)
}
func (r *SenderConfigRepository) ReserveSenderReceipt(ctx context.Context, op, actor string, key, payload [32]byte, now time.Time) (json.RawMessage, bool, error) {
	tx, e := platform.TxFromContext(ctx)
	if e != nil {
		return nil, false, e
	}
	var foundPayload []byte
	var state string
	var result []byte
	e = tx.QueryRow(ctx, `SELECT payload_digest,state,result FROM hxc_sender_config_receipts WHERE operation=$1 AND actor=$2 AND key_digest=$3 FOR UPDATE`, op, actor, key[:]).Scan(&foundPayload, &state, &result)
	if e == nil {
		if string(foundPayload) != string(payload[:]) || state != "completed" {
			return nil, true, hxcapp.ErrConfigConflict
		}
		return result, true, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return nil, false, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO hxc_sender_config_receipts(operation,actor,key_digest,payload_digest,state,created_at) VALUES($1,$2,$3,$4,'in_progress',$5)`, op, actor, key[:], payload[:], now)
	return nil, false, e
}
func (r *SenderConfigRepository) CompleteSenderReceipt(ctx context.Context, op, actor string, key [32]byte, result json.RawMessage, now time.Time) error {
	tx, e := platform.TxFromContext(ctx)
	if e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE hxc_sender_config_receipts SET state='completed',result=$1,completed_at=$2 WHERE operation=$3 AND actor=$4 AND key_digest=$5 AND state='in_progress'`, result, now, op, actor, key[:])
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return hxcapp.ErrConfigConflict
	}
	return nil
}
