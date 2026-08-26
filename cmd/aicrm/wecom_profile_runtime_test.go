package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	wecomprofile "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/profile"
)

type profileTargetStub struct {
	staff, external string
	resolved        bool
	err             error
	customerID      int64
}

func (stub *profileTargetStub) Resolve(_ context.Context, customerID int64) (string, string, bool, error) {
	stub.customerID = customerID
	return stub.staff, stub.external, stub.resolved, stub.err
}

type profileEffectQueuerStub struct {
	command wecomprofile.QueueCommand
	calls   int
}

func (stub *profileEffectQueuerStub) QueueInTransaction(_ context.Context, command wecomprofile.QueueCommand) (wecomprofile.Acceptance, error) {
	stub.calls++
	stub.command = command
	return wecomprofile.Acceptance{EffectID: "eer_41", QueueReceiptID: "eerop_43", RiverJobID: 44, State: eer.StateQueued}, nil
}

func TestSidebarWeComProfileEffectMapsCanonicalProfileAndVerifiedTarget(t *testing.T) {
	targets := &profileTargetStub{staff: "owner-1", external: "external-1", resolved: true}
	effects := &profileEffectQueuerStub{}
	adapter := &sidebarWeComProfileEffect{targets: targets, effects: effects}
	got, err := adapter.QueueInTransaction(context.Background(), contactapp.SidebarProfileEffectCommand{
		ReceiptID: 12, ActorID: 9, CustomerID: contactport.CustomerID(41), IdempotencyKey: "sidebar-profile-queued-0001",
		Profile: contactport.SidebarProfile{Name: "customer", Description: "follow-up"},
	})
	if err != nil || !got.Queued || !got.ProviderExecutionEligible || targets.customerID != 41 || effects.calls != 1 {
		t.Fatalf("got=%+v customer=%d calls=%d err=%v", got, targets.customerID, effects.calls, err)
	}
	want := wecomprofile.QueueCommand{LegacyReceiptID: 12, Actor: 9, IdempotencyKey: "sidebar-profile-queued-0001", StaffUserID: "owner-1", ExternalUserID: "external-1", Remark: "customer", Description: "follow-up"}
	if effects.command != want {
		t.Fatalf("command=%+v want=%+v", effects.command, want)
	}

	targets.resolved = false
	if _, err = adapter.QueueInTransaction(context.Background(), contactapp.SidebarProfileEffectCommand{ReceiptID: 13, ActorID: 9, CustomerID: 41}); err == nil || effects.calls != 1 {
		t.Fatalf("unresolved err=%v calls=%d", err, effects.calls)
	}
}

func TestWeComContactProfileWriterCompositionIsExplicitAndNetworkInert(t *testing.T) {
	writer, err := newWeComContactProfileWriter(appconfig.WeComOutbound{}, nil, nil)
	if err != nil || writer != nil {
		t.Fatalf("disabled writer=%v err=%v", writer, err)
	}
	for key, value := range map[string]string{
		"AICRM_DATABASE_URL":                        "postgres://db/aicrm",
		"AICRM_WORKER_PGX_MAX_CONNS":                "9",
		"AICRM_RIVER_CRITICAL_MAX_WORKERS":          "2",
		"AICRM_RIVER_EVENT_MAX_WORKERS":             "1",
		"AICRM_RIVER_OUTBOUND_MAX_WORKERS":          "1",
		"AICRM_RIVER_SYNC_MAX_WORKERS":              "1",
		"AICRM_RIVER_HEAVY_MAX_WORKERS":             "1",
		"AICRM_RIVER_AI_MAX_WORKERS":                "1",
		"AICRM_WECOM_OUTBOUND_ENABLED":              "true",
		"AICRM_WECOM_OUTBOUND_CORP_ID":              "outbound-corp",
		"AICRM_WECOM_OUTBOUND_SECRET":               "outbound-secret-must-not-leak",
		"AICRM_WECOM_OUTBOUND_PERMISSION_CONFIRMED": "true",
	} {
		t.Setenv(key, value)
	}
	config, err := appconfig.Load(appruntime.RoleWorker)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	writer, err = newWeComContactProfileWriter(config.WeCom.Outbound, &http.Client{Transport: roundTripCounter{calls: &calls}}, time.Now)
	if err != nil || writer == nil || calls.Load() != 0 {
		t.Fatalf("writer=%v calls=%d err=%v", writer, calls.Load(), err)
	}
}
