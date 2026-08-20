package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	event "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	platform "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidCommand = errors.New("invalid hxc sender configuration command")
	ErrConfigConflict = errors.New("hxc sender configuration conflict")
)

type ManageStore interface {
	ListSenderConfigs(context.Context) ([]hxc.SenderConfig, error)
	SaveSenderConfig(context.Context, hxc.SenderConfig) (hxc.SenderConfig, error)
	DeleteSenderConfig(context.Context, string) error
	ReorderSenderConfigs(context.Context, []string) ([]hxc.SenderConfig, error)
	ReserveSenderReceipt(context.Context, string, string, [32]byte, [32]byte, time.Time) (json.RawMessage, bool, error)
	CompleteSenderReceipt(context.Context, string, string, [32]byte, json.RawMessage, time.Time) error
}
type ManageCommand struct {
	ID, SenderUserID, DisplayName, Actor, IdempotencyKey string
	Priority                                             int
	Active                                               bool
}
type Manager struct {
	uow    platform.UnitOfWork
	store  ManageStore
	staff  contact.StaffDirectoryReader
	events event.Appender
	now    func() time.Time
}

func NewManager(uow platform.UnitOfWork, store ManageStore, staff contact.StaffDirectoryReader, events event.Appender) *Manager {
	return &Manager{uow: uow, store: store, staff: staff, events: events, now: time.Now}
}
func (m *Manager) List(ctx context.Context) ([]hxc.SenderConfig, error) {
	if !m.ready() {
		return nil, errUnavailable
	}
	var out []hxc.SenderConfig
	err := m.uow.Within(ctx, func(tx context.Context) error { var e error; out, e = m.store.ListSenderConfigs(tx); return e })
	return out, err
}
func (m *Manager) Save(ctx context.Context, c ManageCommand) (hxc.SenderConfig, error) {
	if !m.ready() || !valid(c) {
		return hxc.SenderConfig{}, ErrInvalidCommand
	}
	var out hxc.SenderConfig
	err := m.mutate(ctx, "save", c, func(tx context.Context) (any, error) {
		if err := m.eligible(tx, c.SenderUserID); err != nil {
			return nil, err
		}
		x, e := m.store.SaveSenderConfig(tx, hxc.SenderConfig{ID: c.ID, SenderUserID: c.SenderUserID, DisplayName: c.DisplayName, Priority: c.Priority, IsActive: c.Active})
		if e != nil {
			return nil, e
		}
		out = x
		return x, nil
	})
	return out, err
}
func (m *Manager) Archive(ctx context.Context, c ManageCommand) error {
	if !m.ready() || !validKey(c.Actor, c.IdempotencyKey) || text(c.ID, 200) == "" {
		return ErrInvalidCommand
	}
	return m.mutate(ctx, "archive", c, func(tx context.Context) (any, error) {
		if e := m.store.DeleteSenderConfig(tx, c.ID); e != nil {
			return nil, e
		}
		return map[string]string{"id": c.ID}, nil
	})
}
func (m *Manager) Reorder(ctx context.Context, actor, key string, ids []string) ([]hxc.SenderConfig, error) {
	c := ManageCommand{Actor: actor, IdempotencyKey: key}
	if !m.ready() || !validKey(actor, key) || len(ids) == 0 {
		return nil, ErrInvalidCommand
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if text(id, 200) == "" || seen[id] {
			return nil, ErrInvalidCommand
		}
		seen[id] = true
	}
	var out []hxc.SenderConfig
	err := m.mutate(ctx, "reorder", c, func(tx context.Context) (any, error) {
		x, e := m.store.ReorderSenderConfigs(tx, ids)
		out = x
		return x, e
	})
	return out, err
}
func (m *Manager) mutate(ctx context.Context, op string, c ManageCommand, fn func(context.Context) (any, error)) error {
	payload, _ := json.Marshal(c)
	kd := sha256.Sum256([]byte(op + "\x00" + c.IdempotencyKey))
	pd := sha256.Sum256(payload)
	return m.uow.Within(ctx, func(tx context.Context) error {
		replay, found, e := m.store.ReserveSenderReceipt(tx, op, c.Actor, kd, pd, m.now().UTC())
		if e != nil {
			return e
		}
		if found {
			if len(replay) == 0 {
				return ErrConfigConflict
			}
			return json.Unmarshal(replay, &struct{}{})
		}
		result, e := fn(tx)
		if e != nil {
			return e
		}
		raw, e := json.Marshal(result)
		if e != nil {
			return e
		}
		if m.events != nil {
			if _, e = m.events.Append(tx, event.Event{Type: "hxc.sender_config.changed", Payload: raw, OccurredAt: m.now().UTC(), IdempotencyKey: "hxc.sender:" + op + ":" + c.IdempotencyKey}); e != nil {
				return e
			}
		}
		return m.store.CompleteSenderReceipt(tx, op, c.Actor, kd, raw, m.now().UTC())
	})
}
func (m *Manager) eligible(ctx context.Context, id string) error {
	entries, e := m.staff.ListEligibleStaff(ctx)
	if e != nil {
		return e
	}
	for _, x := range entries {
		if x.WeComUserID == id {
			return nil
		}
	}
	return ErrConfigConflict
}
func (m *Manager) ready() bool { return m != nil && m.uow != nil && m.store != nil && m.staff != nil }
func valid(c ManageCommand) bool {
	return text(c.ID, 200) != "" && text(c.SenderUserID, 200) != "" && len(c.DisplayName) <= 200 && c.DisplayName == strings.TrimSpace(c.DisplayName) && c.Priority >= 0 && c.Priority <= 100000 && validKey(c.Actor, c.IdempotencyKey)
}
func validKey(actor, key string) bool {
	return text(actor, 200) != "" && len(key) >= 16 && len(key) <= 128 && key == strings.TrimSpace(key)
}
func text(s string, max int) string {
	if s != strings.TrimSpace(s) || s == "" || len(s) > max {
		return ""
	}
	return s
}

var _ = sort.Strings
