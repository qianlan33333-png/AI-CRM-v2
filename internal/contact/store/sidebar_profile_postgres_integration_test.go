package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestSidebarProfilePostgreSQL16ReceiptCASAndConcurrentWriters(t *testing.T) {
	databaseURL := os.Getenv("P4SIDEBAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("P4SIDEBAR_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	prefix := fmt.Sprintf("sidebar-profile-%d", time.Now().UnixNano())
	var staffID int64
	if err = pool.QueryRow(ctx, `INSERT INTO public.staff(wecom_userid,name) VALUES($1,$2) RETURNING id`, prefix+"-staff", prefix+" staff").Scan(&staffID); err != nil {
		t.Fatal(err)
	}
	var customerID int64
	var expected time.Time
	if err = pool.QueryRow(ctx, `INSERT INTO public.customers(name,owner_staff_id,extra) VALUES($1,$2,'{"kept":{"flag":true}}'::jsonb) RETURNING id,updated_at`, prefix, staffID).Scan(&customerID, &expected); err != nil {
		t.Fatal(err)
	}

	service := contactapp.NewSidebarProfileService(platformstore.NewUnitOfWork(pool), NewSidebarProfileRepository(), eventstore.NewAppender())
	type outcome struct {
		key, value string
		profile    contactport.SidebarProfile
		err        error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	var wait sync.WaitGroup
	for index, value := range []string{"first", "second"} {
		wait.Add(1)
		go func(index int, value string) {
			defer wait.Done()
			<-start
			key := fmt.Sprintf("%s-writer-%d-000000000000", prefix, index)
			profile, updateErr := service.UpdateSidebarProfile(ctx, contactport.SidebarProfileUpdateCommand{CustomerID: contactport.CustomerID(customerID), OwnerStaffID: staffID, ExpectedUpdatedAt: expected, Patch: contactport.SidebarProfilePatch{Needs: &value}, Actor: "admin:701", IdempotencyKey: key})
			results <- outcome{key: key, value: value, profile: profile, err: updateErr}
		}(index, value)
	}
	close(start)
	wait.Wait()
	close(results)
	var winner outcome
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			winner = result
		case errors.Is(result.err, contactport.ErrSidebarProfileConflict):
			conflicts++
		default:
			t.Fatalf("unexpected writer error=%v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 || winner.profile.Needs != winner.value {
		t.Fatalf("success/conflict=%d/%d winner=%+v", successes, conflicts, winner)
	}

	var receipts, events int
	var storedNeeds string
	var keptExtra bool
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.sidebar_customer_profile_operation_receipts WHERE actor_scope='sidebar_customer_profile:actor:701' AND state='completed'`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.event_log WHERE event_type='customer.updated' AND customer_id=$1`, customerID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT extra->'sidebar_profile'->>'needs' FROM public.customers WHERE id=$1`, customerID).Scan(&storedNeeds); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT extra @> '{"kept":{"flag":true}}'::jsonb FROM public.customers WHERE id=$1`, customerID).Scan(&keptExtra); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || events != 1 || storedNeeds != winner.value || !keptExtra {
		t.Fatalf("receipts/events/needs/kept=%d/%d/%q/%t", receipts, events, storedNeeds, keptExtra)
	}

	winningValue := winner.value
	replay, err := service.UpdateSidebarProfile(ctx, contactport.SidebarProfileUpdateCommand{CustomerID: contactport.CustomerID(customerID), OwnerStaffID: staffID, ExpectedUpdatedAt: expected, Patch: contactport.SidebarProfilePatch{Needs: &winningValue}, Actor: "admin:701", IdempotencyKey: winner.key})
	if err != nil || replay != winner.profile {
		t.Fatalf("replay=%+v winner=%+v err=%v", replay, winner.profile, err)
	}
}
