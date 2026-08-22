package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestChannelEntrantsPG16Integration(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("P4CHANNELENTRANTS_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("P4CHANNELENTRANTS_TEST_DATABASE_URL is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(databaseURL); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	var versionText string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&versionText); err != nil {
		t.Fatal(err)
	}
	version, err := strconv.Atoi(versionText)
	if err != nil || version < 160000 || version >= 170000 {
		t.Fatalf("PostgreSQL 16 is required, server_version_num=%q err=%v", versionText, err)
	}

	marker := fmt.Sprintf("channel-entrants-%d", time.Now().UnixNano())
	channelIDs := make([]int64, 0, 5)
	insertChannel := func(name, suffix, status string) int64 {
		t.Helper()
		var channelID int64
		err := pool.QueryRow(ctx, `INSERT INTO public.channels
      (name, code, config, status, created_by, updated_by, created_at, updated_at)
      VALUES ($1, $2, jsonb_build_object('schema_version', 1), $3, 9101, 9101, now(), now())
      RETURNING id`, name, marker+"-"+suffix, status).Scan(&channelID)
		if err != nil {
			t.Fatal(err)
		}
		channelIDs = append(channelIDs, channelID)
		return channelID
	}
	channelA := insertChannel("近期进入 A", "a", "active")
	channelB := insertChannel("近期进入 B", "b", "inactive")
	archivedChannel := insertChannel("已归档", "archived", "archived")
	emptyChannel := insertChannel("空渠道", "empty", "active")
	malformedChannel := insertChannel("畸形投影", "malformed", "active")

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if len(channelIDs) == 0 {
			return
		}
		if _, cleanupErr := pool.Exec(cleanupCtx, `DELETE FROM public.customers WHERE channel_id = ANY($1::bigint[])`, channelIDs); cleanupErr != nil {
			t.Errorf("cleanup customers: %v", cleanupErr)
		}
		if _, cleanupErr := pool.Exec(cleanupCtx, `DELETE FROM public.channels WHERE id = ANY($1::bigint[])`, channelIDs); cleanupErr != nil {
			t.Errorf("cleanup channels: %v", cleanupErr)
		}
	})

	tied := time.Date(2026, 8, 22, 12, 0, 0, 987654000, time.UTC)
	insertCustomer := func(
		name string,
		channelID int64,
		addedAt *time.Time,
		lastInteractAt *time.Time,
		deleted bool,
		extra string,
	) int64 {
		t.Helper()
		var customerID int64
		err := pool.QueryRow(ctx, `INSERT INTO public.customers
      (name, channel_id, added_at, last_interact_at, is_deleted, extra, created_at, updated_at)
      VALUES ($1, $2, $3, $4, $5, $6::jsonb, now(), now())
      RETURNING id`, name, channelID, addedAt, lastInteractAt, deleted, extra).Scan(&customerID)
		if err != nil {
			t.Fatal(err)
		}
		return customerID
	}

	deletedTime := tied.Add(time.Minute)
	deletedID := insertCustomer("已删除", channelA, &deletedTime, nil, true, `{}`)
	otherChannelID := insertCustomer("其他渠道", channelB, &tied, nil, false, `{}`)
	firstID := insertCustomer("同刻一", channelA, &tied, nil, false, `{}`)
	lastInteract := tied.Add(10 * time.Minute)
	secondID := insertCustomer("同刻二", channelA, &tied, &lastInteract, false, `{}`)
	thirdID := insertCustomer("同刻三", channelA, &tied, nil, false, `{
      "mobile":"13800000000",
      "unionid":"raw-union-value",
      "external_userid":"raw-external-value",
      "owner_token":"raw-owner-token",
      "provider_payload":{"secret":"raw-provider-secret"}
    }`)
	older := tied.Add(-time.Second)
	olderID := insertCustomer("更早", channelA, &older, nil, false, `{}`)
	_ = insertCustomer("空进入时间", malformedChannel, nil, nil, false, `{}`)

	codec, err := contactapp.NewChannelEntrantsCursorCodec(bytes.Repeat([]byte("pg16-channel-entrants-test-key-"), 2))
	if err != nil {
		t.Fatal(err)
	}
	service, err := contactapp.NewChannelEntrantsService(
		platformstore.NewUnitOfWork(pool),
		contactstore.NewChannelEntrantsRepository(),
		codec,
	)
	if err != nil {
		t.Fatal(err)
	}

	firstPage, err := service.List(ctx, contactapp.ChannelEntrantsInput{ChannelID: channelA, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !firstPage.HasMore || firstPage.NextCursor == "" || len(firstPage.Items) != 2 ||
		!firstPage.LocalProjection || firstPage.RealExternalCallExecuted {
		t.Fatalf("first page=%#v", firstPage)
	}
	wantFirst := []int64{thirdID, secondID}
	if got := []int64{firstPage.Items[0].CustomerID, firstPage.Items[1].CustomerID}; !equalChannelEntrantIDs(got, wantFirst) {
		t.Fatalf("first IDs=%v want=%v", got, wantFirst)
	}

	secondPage, err := service.List(ctx, contactapp.ChannelEntrantsInput{
		ChannelID: channelA,
		Limit:     2,
		Cursor:    firstPage.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondPage.HasMore || secondPage.NextCursor != "" || len(secondPage.Items) != 2 {
		t.Fatalf("second page=%#v", secondPage)
	}
	wantSecond := []int64{firstID, olderID}
	if got := []int64{secondPage.Items[0].CustomerID, secondPage.Items[1].CustomerID}; !equalChannelEntrantIDs(got, wantSecond) {
		t.Fatalf("second IDs=%v want=%v", got, wantSecond)
	}

	allIDs := []int64{
		firstPage.Items[0].CustomerID,
		firstPage.Items[1].CustomerID,
		secondPage.Items[0].CustomerID,
		secondPage.Items[1].CustomerID,
	}
	if containsChannelEntrantID(allIDs, deletedID) || containsChannelEntrantID(allIDs, otherChannelID) {
		t.Fatalf("cross-channel/deleted row leaked: ids=%v deleted=%d other=%d", allIDs, deletedID, otherChannelID)
	}
	sorted := append([]int64(nil), []int64{firstID, secondID, thirdID}...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })
	if !equalChannelEntrantIDs(allIDs[:3], sorted) {
		t.Fatalf("same-timestamp tie-break=%v want=%v", allIDs[:3], sorted)
	}

	if _, err = service.List(ctx, contactapp.ChannelEntrantsInput{
		ChannelID: channelB, Limit: 2, Cursor: firstPage.NextCursor,
	}); !errors.Is(err, contactapp.ErrInvalidChannelEntrantsCursor) {
		t.Fatalf("cross-channel cursor error=%v", err)
	}
	channelBPage, err := service.List(ctx, contactapp.ChannelEntrantsInput{ChannelID: channelB, Limit: 20})
	if err != nil || len(channelBPage.Items) != 1 || channelBPage.Items[0].CustomerID != otherChannelID {
		t.Fatalf("channel B page=%#v err=%v", channelBPage, err)
	}
	if _, err = service.List(ctx, contactapp.ChannelEntrantsInput{ChannelID: archivedChannel}); !errors.Is(err, contactapp.ErrChannelEntrantsNotFound) {
		t.Fatalf("archived error=%v", err)
	}
	if _, err = service.List(ctx, contactapp.ChannelEntrantsInput{ChannelID: 1<<62 - 1}); !errors.Is(err, contactapp.ErrChannelEntrantsNotFound) {
		t.Fatalf("missing error=%v", err)
	}
	emptyPage, err := service.List(ctx, contactapp.ChannelEntrantsInput{ChannelID: emptyChannel})
	if err != nil || emptyPage.Items == nil || len(emptyPage.Items) != 0 || emptyPage.HasMore {
		t.Fatalf("empty page=%#v err=%v", emptyPage, err)
	}
	if _, err = service.List(ctx, contactapp.ChannelEntrantsInput{ChannelID: malformedChannel}); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) {
		t.Fatalf("malformed projection error=%v", err)
	}

	encoded, err := json.Marshal([]contactapp.ChannelEntrantsResponse{firstPage, secondPage})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"13800000000", "raw-union-value", "raw-external-value", "raw-owner-token",
		"raw-provider-secret", "mobile", "unionid", "external_userid", "owner_staff_id",
		"owner_token", "provider_payload", "\"extra\"",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("sensitive local column leaked %q: %s", forbidden, encoded)
		}
	}
}

func equalChannelEntrantIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func containsChannelEntrantID(values []int64, expected int64) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
