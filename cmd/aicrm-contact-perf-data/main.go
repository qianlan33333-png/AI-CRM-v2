package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	performanceDatabase       = "aicrm_perf"
	resetToken                = "AICRM_PERF_RESET_V1"
	seedText                  = "20260812"
	datasetSeed         int64 = 20260812

	stageCount        = 8
	staffCount        = 64
	channelCount      = 12
	tagGroupCount     = 5
	tagCount          = 50
	customerCount     = 200000
	tagsPerCustomer   = 3
	deletedCount      = 10000
	hotCohortPerState = 500

	seedTimeout = 15 * time.Minute
)

var (
	errInvalidArguments     = errors.New("invalid arguments")
	errDatabaseUnavailable  = errors.New("database unavailable")
	errDatabaseSchema       = errors.New("database schema rejected")
	errSeedFailed           = errors.New("seed transaction failed")
	errAnalyzeIncomplete    = errors.New("seed committed but ANALYZE incomplete")
	errValidationIncomplete = errors.New("seed committed but validation incomplete")
)

var requiredTables = []string{
	"customer_events",
	"customer_tags",
	"tags",
	"tag_groups",
	"customers",
	"channels",
	"staff",
	"stages",
}

var forbiddenCustomerColumns = map[string]struct{}{
	"external_user_id": {},
	"external_userid":  {},
	"mobile":           {},
	"mobile_phone":     {},
	"open_id":          {},
	"openid":           {},
	"phone":            {},
	"phone_number":     {},
	"telephone":        {},
	"unionid":          {},
	"wecom_user_id":    {},
	"wecom_userid":     {},
	"wechat_user_id":   {},
	"wechat_userid":    {},
}

var (
	addedWindowStart    = time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	addedWindowEnd      = time.Date(2026, time.July, 31, 23, 59, 59, 0, time.UTC)
	interactWindowStart = time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	interactWindowEnd   = time.Date(2026, time.August, 11, 23, 59, 59, 0, time.UTC)
	queryWatermark      = time.Date(2026, time.August, 13, 0, 0, 0, 0, time.UTC)
)

type commandConfig struct {
	databaseURL string
	seed        int64
}

type seedSummary struct {
	Database            string `json:"database"`
	Seed                int64  `json:"seed"`
	Stages              int64  `json:"stages"`
	Staff               int64  `json:"staff"`
	Channels            int64  `json:"channels"`
	TagGroups           int64  `json:"tag_groups"`
	Tags                int64  `json:"tags"`
	Customers           int64  `json:"customers"`
	CustomerTags        int64  `json:"customer_tags"`
	ActiveCustomers     int64  `json:"active_customers"`
	DeletedCustomers    int64  `json:"deleted_customers"`
	HotActiveCustomers  int64  `json:"hot_active_customers"`
	HotDeletedCustomers int64  `json:"hot_deleted_customers"`
}

type customerRecord struct {
	name           string
	gender         int16
	stageID        int64
	ownerStaffID   int64
	channelID      int64
	addedAt        time.Time
	lastInteractAt time.Time
	isDeleted      bool
	extra          string
	createdAt      time.Time
	updatedAt      time.Time
	tagIDs         [tagsPerCustomer]int64
}

type seedRunner func(context.Context, string, int64) (seedSummary, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, seedPerformanceDatabase))
}

func run(args []string, stdout, stderr io.Writer, seedDatabase seedRunner) int {
	if stdout == nil || stderr == nil || seedDatabase == nil {
		return 2
	}
	config, err := parseArguments(args)
	if err != nil {
		fmt.Fprintln(stderr, "aicrm-contact-perf-data: invalid arguments")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()
	summary, err := seedDatabase(ctx, config.databaseURL, config.seed)
	if err != nil {
		switch {
		case errors.Is(err, errAnalyzeIncomplete):
			fmt.Fprintln(stderr, "aicrm-contact-perf-data: seed committed but ANALYZE incomplete")
		case errors.Is(err, errValidationIncomplete):
			fmt.Fprintln(stderr, "aicrm-contact-perf-data: seed committed but validation incomplete")
		default:
			fmt.Fprintln(stderr, "aicrm-contact-perf-data: database reset failed")
		}
		return 1
	}

	encoded, err := json.Marshal(summary)
	if err != nil {
		fmt.Fprintln(stderr, "aicrm-contact-perf-data: summary encoding failed")
		return 1
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}

func parseArguments(args []string) (commandConfig, error) {
	if len(args) != 3 {
		return commandConfig{}, errInvalidArguments
	}

	var databaseURL, databaseURLFile, suppliedToken, suppliedSeed string
	var databaseURLFileSet bool
	for _, argument := range args {
		switch {
		case strings.HasPrefix(argument, "--database-url-file=") && !databaseURLFileSet:
			databaseURLFileSet = true
			databaseURLFile = strings.TrimPrefix(argument, "--database-url-file=")
		case strings.HasPrefix(argument, "--reset-token=") && suppliedToken == "":
			suppliedToken = strings.TrimPrefix(argument, "--reset-token=")
		case strings.HasPrefix(argument, "--seed=") && suppliedSeed == "":
			suppliedSeed = strings.TrimPrefix(argument, "--seed=")
		default:
			return commandConfig{}, errInvalidArguments
		}
	}
	if !databaseURLFileSet || suppliedToken != resetToken || suppliedSeed != seedText {
		return commandConfig{}, errInvalidArguments
	}
	var err error
	databaseURL, err = readDatabaseURLFile(databaseURLFile)
	if err != nil {
		return commandConfig{}, errInvalidArguments
	}
	if databaseURL == "" {
		return commandConfig{}, errInvalidArguments
	}
	if _, err := validateDatabaseURL(databaseURL); err != nil {
		return commandConfig{}, errInvalidArguments
	}
	return commandConfig{databaseURL: databaseURL, seed: datasetSeed}, nil
}

func readDatabaseURLFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errInvalidArguments
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 4096 {
		return "", errInvalidArguments
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errInvalidArguments
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return "", errInvalidArguments
	}
	contents, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(contents) > 4096 {
		return "", errInvalidArguments
	}
	databaseURL := strings.TrimSpace(string(contents))
	if databaseURL == "" || strings.ContainsAny(databaseURL, "\r\n") {
		return "", errInvalidArguments
	}
	return databaseURL, nil
}

func validateDatabaseURL(databaseURL string) (*pgxpool.Config, error) {
	parsed, err := url.ParseRequestURI(databaseURL)
	if err != nil || parsed == nil {
		return nil, errInvalidArguments
	}
	if !strings.EqualFold(parsed.Scheme, "postgres") && !strings.EqualFold(parsed.Scheme, "postgresql") {
		return nil, errInvalidArguments
	}
	if parsed.Opaque != "" || parsed.Host == "" || parsed.Path != "/"+performanceDatabase ||
		parsed.RawPath != "" || parsed.Fragment != "" || parsed.RawFragment != "" || parsed.ForceQuery ||
		parsed.RawQuery != "sslmode=disable" || !safeDatabaseHost(parsed.Hostname()) || !safePort(parsed.Port()) {
		return nil, errInvalidArguments
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil || config.ConnConfig.Database != performanceDatabase {
		return nil, errInvalidArguments
	}
	return config, nil
}

func safeDatabaseHost(host string) bool {
	parsedIP := net.ParseIP(host)
	return parsedIP != nil && parsedIP.IsLoopback() && (host == "127.0.0.1" || host == "::1")
}

func safePort(port string) bool {
	if port == "" {
		return true
	}
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value <= 65535
}

func seedPerformanceDatabase(ctx context.Context, databaseURL string, seed int64) (seedSummary, error) {
	config, err := validateDatabaseURL(databaseURL)
	if err != nil || seed != datasetSeed {
		return seedSummary{}, errDatabaseSchema
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return seedSummary{}, errDatabaseUnavailable
	}
	defer pool.Close()

	connection, err := pool.Acquire(ctx)
	if err != nil {
		return seedSummary{}, errDatabaseUnavailable
	}
	defer connection.Release()

	if err = verifyDatabaseAndSchema(ctx, connection); err != nil {
		return seedSummary{}, errDatabaseSchema
	}

	tx, err := connection.Begin(ctx)
	if err != nil {
		return seedSummary{}, errSeedFailed
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err = resetAndSeed(ctx, tx, seed); err != nil {
		return seedSummary{}, errSeedFailed
	}
	if err = tx.Commit(ctx); err != nil {
		return seedSummary{}, errSeedFailed
	}

	if _, err = connection.Exec(ctx, "ANALYZE public.stages, public.staff, public.channels, public.tag_groups, public.tags, public.customers, public.customer_tags"); err != nil {
		return seedSummary{}, errAnalyzeIncomplete
	}

	summary, err := validateSeededDatabase(ctx, connection, seed)
	if err != nil {
		return seedSummary{}, errValidationIncomplete
	}
	return summary, nil
}

func verifyDatabaseAndSchema(ctx context.Context, connection *pgxpool.Conn) error {
	var currentDatabase string
	if err := connection.QueryRow(ctx, "SELECT current_database()").Scan(&currentDatabase); err != nil || currentDatabase != performanceDatabase {
		return errDatabaseSchema
	}
	var presentTableCount int64
	if err := connection.QueryRow(ctx, `
SELECT count(*)
FROM pg_catalog.pg_class AS relation
JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
WHERE namespace.nspname = 'public'
  AND relation.relkind IN ('r', 'p')
  AND relation.relname = ANY($1)`, requiredTables).Scan(&presentTableCount); err != nil || presentTableCount != int64(len(requiredTables)) {
		return errDatabaseSchema
	}

	rows, err := connection.Query(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'customers'`)
	if err != nil {
		return errDatabaseSchema
	}
	defer rows.Close()
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err != nil || isForbiddenCustomerColumn(columnName) {
			return errDatabaseSchema
		}
	}
	if rows.Err() != nil {
		return errDatabaseSchema
	}
	return nil
}

func isForbiddenCustomerColumn(columnName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(columnName))
	if _, forbidden := forbiddenCustomerColumns[normalized]; forbidden {
		return true
	}
	canonical := strings.ReplaceAll(normalized, "_", "")
	return strings.Contains(canonical, "phone") || strings.Contains(canonical, "mobile") ||
		strings.Contains(canonical, "telephone") || strings.Contains(canonical, "unionid") ||
		strings.Contains(canonical, "openid") || strings.Contains(canonical, "external")
}

func resetAndSeed(ctx context.Context, tx pgx.Tx, seed int64) error {
	if _, err := tx.Exec(ctx, "TRUNCATE TABLE public.customer_tags, public.customer_events, public.tags, public.tag_groups, public.customers, public.channels, public.staff, public.stages RESTART IDENTITY"); err != nil {
		return errSeedFailed
	}
	if err := copyStages(ctx, tx); err != nil {
		return err
	}
	if err := copyStaff(ctx, tx); err != nil {
		return err
	}
	if err := copyChannels(ctx, tx); err != nil {
		return err
	}
	if err := copyTagGroups(ctx, tx); err != nil {
		return err
	}
	if err := copyTags(ctx, tx); err != nil {
		return err
	}
	if err := copyCustomers(ctx, tx, seed); err != nil {
		return err
	}
	return copyCustomerTags(ctx, tx, seed)
}

func copyStages(ctx context.Context, tx pgx.Tx) error {
	return copyExpected(ctx, tx, pgx.Identifier{"public", "stages"}, []string{"name", "sort_order", "config"}, stageCount, func(index int) ([]any, error) {
		return []any{fmt.Sprintf("synthetic-stage-%02d", index+1), index + 1, "{}"}, nil
	})
}

func copyStaff(ctx context.Context, tx pgx.Tx) error {
	return copyExpected(ctx, tx, pgx.Identifier{"public", "staff"}, []string{"wecom_userid", "name", "department", "is_active", "created_at", "updated_at"}, staffCount, func(index int) ([]any, error) {
		timestamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * time.Minute)
		return []any{
			fmt.Sprintf("synthetic-staff-%02d", index+1),
			fmt.Sprintf("Synthetic Staff %02d", index+1),
			fmt.Sprintf("synthetic-department-%02d", index%8+1),
			true,
			timestamp,
			timestamp,
		}, nil
	})
}

func copyChannels(ctx context.Context, tx pgx.Tx) error {
	return copyExpected(ctx, tx, pgx.Identifier{"public", "channels"}, []string{"name", "code", "config", "created_at"}, channelCount, func(index int) ([]any, error) {
		return []any{
			fmt.Sprintf("synthetic-channel-%02d", index+1),
			fmt.Sprintf("synthetic-channel-%02d", index+1),
			"{}",
			time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * time.Minute),
		}, nil
	})
}

func copyTagGroups(ctx context.Context, tx pgx.Tx) error {
	return copyExpected(ctx, tx, pgx.Identifier{"public", "tag_groups"}, []string{"name", "sort_order"}, tagGroupCount, func(index int) ([]any, error) {
		return []any{fmt.Sprintf("synthetic-tag-group-%02d", index+1), index + 1}, nil
	})
}

func copyTags(ctx context.Context, tx pgx.Tx) error {
	return copyExpected(ctx, tx, pgx.Identifier{"public", "tags"}, []string{"group_id", "name", "sort_order"}, tagCount, func(index int) ([]any, error) {
		return []any{int64(index/10 + 1), fmt.Sprintf("synthetic-tag-%02d", index+1), index%10 + 1}, nil
	})
}

func copyCustomers(ctx context.Context, tx pgx.Tx, seed int64) error {
	return copyExpected(ctx, tx, pgx.Identifier{"public", "customers"}, []string{
		"name", "gender", "stage_id", "owner_staff_id", "channel_id", "added_at", "last_interact_at", "is_deleted", "extra", "created_at", "updated_at",
	}, customerCount, func(index int) ([]any, error) {
		record := deterministicCustomer(seed, index)
		return []any{
			record.name,
			record.gender,
			record.stageID,
			record.ownerStaffID,
			record.channelID,
			record.addedAt,
			record.lastInteractAt,
			record.isDeleted,
			record.extra,
			record.createdAt,
			record.updatedAt,
		}, nil
	})
}

func copyCustomerTags(ctx context.Context, tx pgx.Tx, seed int64) error {
	return copyExpected(ctx, tx, pgx.Identifier{"public", "customer_tags"}, []string{"customer_id", "tag_id", "tagged_at"}, customerCount*tagsPerCustomer, func(index int) ([]any, error) {
		customerIndex := index / tagsPerCustomer
		tagIndex := index % tagsPerCustomer
		record := deterministicCustomer(seed, customerIndex)
		return []any{int64(customerIndex + 1), record.tagIDs[tagIndex], record.addedAt}, nil
	})
}

func copyExpected(ctx context.Context, tx pgx.Tx, table pgx.Identifier, columns []string, expected int, next func(int) ([]any, error)) error {
	copied, err := tx.CopyFrom(ctx, table, columns, pgx.CopyFromSlice(expected, next))
	if err != nil || copied != int64(expected) {
		return errSeedFailed
	}
	return nil
}

func deterministicCustomer(seed int64, index int) customerRecord {
	if index < 0 || index >= customerCount {
		return customerRecord{}
	}
	value := seededValue(seed, index, 1)
	addedAt := deterministicTime(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), int(value%244), seededValue(seed, index, 2))
	lastInteractAt := deterministicTime(time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), int(seededValue(seed, index, 3)%210), seededValue(seed, index, 4))
	updatedAt := deterministicTime(time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), int(seededValue(seed, index, 5)%192), seededValue(seed, index, 6))
	record := customerRecord{
		name:           fmt.Sprintf("kw%03d", index%128),
		gender:         int16(value % 3),
		stageID:        int64(seededValue(seed, index, 7)%stageCount) + 1,
		ownerStaffID:   int64(seededValue(seed, index, 8)%staffCount) + 1,
		channelID:      int64(seededValue(seed, index, 9)%channelCount) + 1,
		addedAt:        addedAt,
		lastInteractAt: lastInteractAt,
		isDeleted:      ordinaryCustomerDeleted(seed, index),
		extra:          neutralExtra(fmt.Sprintf("bucket-%02d", seededValue(seed, index, 10)%32)),
		createdAt:      addedAt.Add(-24 * time.Hour),
		updatedAt:      updatedAt,
		tagIDs:         deterministicTags(seed, index),
	}
	if cohort, deleted, cohortIndex := hotCohort(index); cohort {
		record.name = "kw017"
		record.stageID = 3
		record.ownerStaffID = 7
		record.channelID = 5
		record.addedAt = deterministicTime(addedWindowStart, cohortIndex%122, seededValue(seed, index, 11))
		record.lastInteractAt = deterministicTime(interactWindowStart, cohortIndex%103, seededValue(seed, index, 12))
		record.isDeleted = deleted
		record.extra = neutralExtra(map[bool]string{false: "hot-active", true: "hot-deleted"}[deleted])
		record.createdAt = record.addedAt.Add(-24 * time.Hour)
		record.updatedAt = deterministicTime(time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC), cohortIndex%1, seededValue(seed, index, 13))
		record.tagIDs = [tagsPerCustomer]int64{11, 12, 13}
	}
	return record
}

func deterministicTags(seed int64, index int) [tagsPerCustomer]int64 {
	first := int(seededValue(seed, index, 14) % tagCount)
	return [tagsPerCustomer]int64{
		int64(first + 1),
		int64((first+17)%tagCount + 1),
		int64((first+34)%tagCount + 1),
	}
}

func ordinaryCustomerDeleted(seed int64, index int) bool {
	if index < hotCohortPerState*2 || index >= customerCount {
		return false
	}
	regularIndex := index - hotCohortPerState*2
	regularCount := customerCount - hotCohortPerState*2
	regularDeletedCount := deletedCount - hotCohortPerState
	permuted := (uint64(regularIndex)*7919 + uint64(seed)) % uint64(regularCount)
	return permuted < uint64(regularDeletedCount)
}

func hotCohort(index int) (matched bool, deleted bool, cohortIndex int) {
	switch {
	case index >= 0 && index < hotCohortPerState:
		return true, false, index
	case index >= hotCohortPerState && index < hotCohortPerState*2:
		return true, true, index - hotCohortPerState
	default:
		return false, false, 0
	}
}

func deterministicTime(start time.Time, days int, entropy uint64) time.Time {
	return start.AddDate(0, 0, days).Add(time.Duration(entropy%86400) * time.Second)
}

func neutralExtra(bucket string) string {
	return fmt.Sprintf(`{"synthetic":true,"bucket":"%s"}`, bucket)
}

func seededValue(seed int64, index int, salt uint64) uint64 {
	return mix64(uint64(seed) ^ uint64(index+1)*0x9e3779b97f4a7c15 ^ salt*0xbf58476d1ce4e5b9)
}

func mix64(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	return value ^ value>>31
}

func validateSeededDatabase(ctx context.Context, connection *pgxpool.Conn, seed int64) (seedSummary, error) {
	if err := verifyDatabaseAndSchema(ctx, connection); err != nil {
		return seedSummary{}, errValidationIncomplete
	}
	summary := seedSummary{Database: performanceDatabase, Seed: seed}
	if err := connection.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM public.stages),
  (SELECT count(*) FROM public.staff),
  (SELECT count(*) FROM public.channels),
  (SELECT count(*) FROM public.tag_groups),
  (SELECT count(*) FROM public.tags),
  (SELECT count(*) FROM public.customers),
  (SELECT count(*) FROM public.customer_tags)`).Scan(
		&summary.Stages,
		&summary.Staff,
		&summary.Channels,
		&summary.TagGroups,
		&summary.Tags,
		&summary.Customers,
		&summary.CustomerTags,
	); err != nil {
		return seedSummary{}, errValidationIncomplete
	}
	if summary.Stages != stageCount || summary.Staff != staffCount || summary.Channels != channelCount ||
		summary.TagGroups != tagGroupCount || summary.Tags != tagCount || summary.Customers != customerCount ||
		summary.CustomerTags != customerCount*tagsPerCustomer {
		return seedSummary{}, errValidationIncomplete
	}

	if err := validateDistribution(ctx, connection, &summary); err != nil {
		return seedSummary{}, errValidationIncomplete
	}
	return summary, nil
}

func validateDistribution(ctx context.Context, connection *pgxpool.Conn, summary *seedSummary) error {
	var stages, staff, channels int64
	var addedBefore, addedWithin, addedAfter int64
	var interactedBefore, interactedWithin, interactedAfter int64
	if err := connection.QueryRow(ctx, `
SELECT
  count(*) FILTER (WHERE is_deleted),
  count(*) FILTER (WHERE NOT is_deleted),
  count(DISTINCT stage_id),
  count(DISTINCT owner_staff_id),
  count(DISTINCT channel_id),
  count(*) FILTER (WHERE added_at < $1),
  count(*) FILTER (WHERE added_at >= $1 AND added_at <= $2),
  count(*) FILTER (WHERE added_at > $2),
  count(*) FILTER (WHERE last_interact_at < $3),
  count(*) FILTER (WHERE last_interact_at >= $3 AND last_interact_at <= $4),
  count(*) FILTER (WHERE last_interact_at > $4)
FROM public.customers`, addedWindowStart, addedWindowEnd, interactWindowStart, interactWindowEnd).Scan(
		&summary.DeletedCustomers,
		&summary.ActiveCustomers,
		&stages,
		&staff,
		&channels,
		&addedBefore,
		&addedWithin,
		&addedAfter,
		&interactedBefore,
		&interactedWithin,
		&interactedAfter,
	); err != nil {
		return errValidationIncomplete
	}
	if summary.DeletedCustomers != deletedCount || summary.ActiveCustomers != customerCount-deletedCount ||
		stages != stageCount || staff != staffCount || channels != channelCount ||
		addedBefore == 0 || addedWithin == 0 || addedAfter == 0 ||
		interactedBefore == 0 || interactedWithin == 0 || interactedAfter == 0 {
		return errValidationIncomplete
	}

	var invalidTagCounts, distinctTags, duplicateTags int64
	if err := connection.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM (
    SELECT customer_id
    FROM public.customer_tags
    GROUP BY customer_id
    HAVING count(*) <> 3
  ) AS malformed),
  (SELECT count(DISTINCT tag_id) FROM public.customer_tags),
  (SELECT count(*) FROM (
    SELECT customer_id, tag_id
    FROM public.customer_tags
    GROUP BY customer_id, tag_id
    HAVING count(*) > 1
  ) AS duplicate_tags)`).Scan(&invalidTagCounts, &distinctTags, &duplicateTags); err != nil {
		return errValidationIncomplete
	}
	if invalidTagCounts != 0 || distinctTags != tagCount || duplicateTags != 0 {
		return errValidationIncomplete
	}

	if err := connection.QueryRow(ctx, `
SELECT
  count(*) FILTER (WHERE NOT c.is_deleted),
  count(*) FILTER (WHERE c.is_deleted)
FROM public.customers AS c
WHERE c.name = 'kw017'
  AND c.owner_staff_id = 7
  AND c.stage_id = 3
  AND c.channel_id = 5
  AND c.added_at >= $1
  AND c.added_at <= $2
  AND c.last_interact_at >= $3
  AND c.last_interact_at <= $4
  AND c.updated_at <= $5
  AND EXISTS (
    SELECT 1
    FROM public.customer_tags AS customer_tag
    WHERE customer_tag.customer_id = c.id AND customer_tag.tag_id = 11
  )`, addedWindowStart, addedWindowEnd, interactWindowStart, interactWindowEnd, queryWatermark).Scan(
		&summary.HotActiveCustomers,
		&summary.HotDeletedCustomers,
	); err != nil {
		return errValidationIncomplete
	}
	if summary.HotActiveCustomers < hotCohortPerState || summary.HotDeletedCustomers < hotCohortPerState {
		return errValidationIncomplete
	}
	return nil
}
