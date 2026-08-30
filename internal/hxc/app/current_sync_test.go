package app

import (
	"context"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type syncSourceFake struct {
	rows []hxcport.SourceCurrent
	err  error
}

func (fake syncSourceFake) ReadCurrent(context.Context) ([]hxcport.SourceCurrent, error) {
	return fake.rows, fake.err
}

type syncIdentityFake map[string]identityport.ResolveResult

func (fake syncIdentityFake) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	return fake[string(ref.Kind)+":"+ref.Value], nil
}

type syncStoreFake struct {
	rows     []hxcport.Current
	summary  hxcport.SyncSummary
	failures []string
}

func (fake *syncStoreFake) Replace(_ context.Context, rows []hxcport.Current, summary hxcport.SyncSummary, _ time.Time) error {
	fake.rows, fake.summary = append([]hxcport.Current(nil), rows...), summary
	return nil
}
func (fake *syncStoreFake) RecordFailure(_ context.Context, _ time.Time, code string) error {
	fake.failures = append(fake.failures, code)
	return nil
}

type syncUOWFake struct{}

func (syncUOWFake) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

func TestCurrentSyncMatchesUnionIDThenPhoneAndFailsClosedOnConflict(t *testing.T) {
	now := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	rows := []hxcport.SourceCurrent{
		validSource("hxc-union", "union-1", "+8613800000001", now),
		validSource("hxc-phone", "union-missing", "+8613800000002", now),
		validSource("hxc-conflict", "union-3", "+8613800000003", now),
	}
	identities := syncIdentityFake{
		"unionid:union-1":       {Status: identityport.ResolveFound, CustomerID: 11},
		"phone:+8613800000001":  {Status: identityport.ResolveFound, CustomerID: 11},
		"unionid:union-missing": {Status: identityport.ResolveNotFound},
		"phone:+8613800000002":  {Status: identityport.ResolveFound, CustomerID: 22},
		"unionid:union-3":       {Status: identityport.ResolveFound, CustomerID: 33},
		"phone:+8613800000003":  {Status: identityport.ResolveFound, CustomerID: 44},
	}
	store := &syncStoreFake{}
	service, err := NewCurrentSyncService(syncSourceFake{rows: rows}, identities, store, syncUOWFake{}, "wechat-open-platform:hxc", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary != (hxcport.SyncSummary{SourceCount: 3, MatchedCount: 2, ConflictCount: 1}) {
		t.Fatalf("summary = %#v", summary)
	}
	if store.rows[0].CustomerID != contactport.CustomerID(11) || store.rows[1].CustomerID != contactport.CustomerID(22) || store.rows[2].CustomerID != 0 || store.rows[2].MatchState != hxcport.MatchStateConflict {
		t.Fatalf("matched rows = %#v", store.rows)
	}
}

func TestCurrentSyncSourceFailurePreservesCurrentRows(t *testing.T) {
	store := &syncStoreFake{}
	service, err := NewCurrentSyncService(syncSourceFake{err: errors.New("mysql unavailable")}, syncIdentityFake{}, store, syncUOWFake{}, "wechat-open-platform:hxc", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Sync(context.Background()); !errors.Is(err, ErrHXCCurrentSync) {
		t.Fatalf("Sync() error = %v", err)
	}
	if store.rows != nil || len(store.failures) != 1 || store.failures[0] != "source_unavailable" {
		t.Fatalf("failure state = rows %#v failures %#v", store.rows, store.failures)
	}
}

func validSource(id, unionID, phone string, now time.Time) hxcport.SourceCurrent {
	return hxcport.SourceCurrent{
		HXCUserID: id, UnionID: unionID, Phone: phone, SubscriptionTier: "pro",
		Sessions7D: 1, Sessions30D: 2, SessionsTotal: 3,
		UserMessages7D: 2, UserMessages30D: 4, UserMessagesTotal: 8,
		CapabilityUsage: map[string]hxcport.CapabilityUsage{"peer_chat": {Count7D: 2, Count30D: 4, CountTotal: 8}},
		FocusTopics:     []string{"growth"}, SourceUpdatedAt: now,
	}
}
