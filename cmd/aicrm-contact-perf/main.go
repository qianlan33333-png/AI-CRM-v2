package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const (
	requiredDatabase  = "aicrm_perf"
	requiredCustomers = 200_000
	requiredTags      = 600_000
	requiredSamples   = 20
	requiredWarmups   = 3
	selectorGroups    = 5
	latencyLimit      = 200 * time.Millisecond
)

var (
	benchmarkWatermark = time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	addedAfter         = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	addedBefore        = time.Date(2026, 7, 31, 23, 59, 59, 0, time.UTC)
	interactAfter      = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	interactBefore     = time.Date(2026, 8, 11, 23, 59, 59, 0, time.UTC)
)

type options struct {
	databaseURL     string
	databaseURLFile string
	sourceSHA       string
	receiptPath     string
	samples         int
	warmups         int
}

type datasetEvidence struct {
	Customers    int64 `json:"customers"`
	CustomerTags int64 `json:"customer_tags"`
	Staff        int64 `json:"staff"`
	Stages       int64 `json:"stages"`
	Channels     int64 `json:"channels"`
	Tags         int64 `json:"tags"`
	Deleted      int64 `json:"deleted_customers"`
	HotActive    int64 `json:"hot_cohort_active"`
	HotDeleted   int64 `json:"hot_cohort_deleted"`
}

type environmentEvidence struct {
	SourceSHA          string `json:"source_sha"`
	BinaryVCSRevision  string `json:"binary_vcs_revision"`
	BinaryVCSModified  bool   `json:"binary_vcs_modified"`
	Database           string `json:"database"`
	PostgreSQLVersion  string `json:"postgresql_server_version_num"`
	CPUs               int    `json:"cpus"`
	MemoryKiB          int64  `json:"memory_kib"`
	SwapKiB            int64  `json:"swap_kib"`
	GoMemoryLimitBytes int64  `json:"go_memory_limit_bytes"`
	SharedBuffers      string `json:"shared_buffers"`
	EffectiveCacheSize string `json:"effective_cache_size"`
	WorkMem            string `json:"work_mem"`
	MaxConnections     string `json:"max_connections"`
}

type planEvidence struct {
	Query          string   `json:"query"`
	ExecutionMS    float64  `json:"execution_ms"`
	PlanningMS     float64  `json:"planning_ms"`
	SharedHit      int64    `json:"shared_hit_blocks"`
	SharedRead     int64    `json:"shared_read_blocks"`
	NodeTypes      []string `json:"node_types"`
	ForbiddenScans []string `json:"forbidden_seq_scans"`
}

type caseEvidence struct {
	ID           string         `json:"id"`
	SelectorMask int            `json:"selector_mask"`
	Deleted      bool           `json:"is_deleted"`
	Samples      int            `json:"samples"`
	AddedMode    string         `json:"added_mode"`
	InteractMode string         `json:"interact_mode"`
	Page         string         `json:"page"`
	Limit        int32          `json:"limit"`
	P50MS        float64        `json:"p50_ms"`
	P95MS        float64        `json:"p95_ms"`
	MaxMS        float64        `json:"max_ms"`
	Matched      int            `json:"items"`
	HasMore      bool           `json:"has_more"`
	Plans        []planEvidence `json:"plans"`
}

type timeMode uint8

const (
	timeNone timeMode = iota
	timeAfter
	timeBefore
	timeClosed
)

type scenario struct {
	selectorMask int
	deleted      bool
	addedMode    timeMode
	interactMode timeMode
	nextPage     bool
	limit        int32
}

type report struct {
	Kind             string              `json:"kind"`
	EvidenceClass    string              `json:"evidence_class"`
	GeneratedAt      string              `json:"generated_at"`
	Environment      environmentEvidence `json:"environment"`
	Dataset          datasetEvidence     `json:"dataset"`
	Cases            []caseEvidence      `json:"cases"`
	CombinationCount int                 `json:"combination_count"`
	SampleCount      int                 `json:"sample_count"`
	GlobalP50MS      float64             `json:"global_p50_ms"`
	GlobalP95MS      float64             `json:"global_p95_ms"`
	GlobalMaxMS      float64             `json:"global_max_ms"`
	SlowestCase      string              `json:"slowest_case"`
	ThresholdMS      int64               `json:"threshold_ms"`
	Passed           bool                `json:"passed"`
}

type capturedQuery struct {
	Name string
	SQL  string
	Args []any
}

type queryCapture struct {
	mu      sync.Mutex
	active  bool
	queries []capturedQuery
}

func (capture *queryCapture) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	name := ""
	switch {
	case strings.HasPrefix(data.SQL, "-- name: ListCustomerIDsBounded :many"):
		name = "ListCustomerIDsBounded"
	case strings.HasPrefix(data.SQL, "-- name: ListCustomers :many"):
		name = "ListCustomers"
	}
	if name == "" {
		return ctx
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.active {
		capture.queries = append(capture.queries, capturedQuery{
			Name: name,
			SQL:  data.SQL,
			Args: append([]any(nil), data.Args...),
		})
	}
	return ctx
}

func (*queryCapture) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (capture *queryCapture) begin() {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.active = true
	capture.queries = nil
}

func (capture *queryCapture) finish() ([]capturedQuery, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.active = false
	queries := append([]capturedQuery(nil), capture.queries...)
	capture.queries = nil
	if len(queries) != 2 || queries[0].Name != "ListCustomerIDsBounded" || queries[1].Name != "ListCustomers" {
		return nil, errors.New("production customer repository did not execute the two frozen queries exactly once")
	}
	return queries, nil
}

func main() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "contact-perf: invalid arguments")
		os.Exit(2)
	}
	if opts.receiptPath != "" {
		if err := verifyReceiptFile(opts.receiptPath); err != nil {
			fmt.Fprintln(os.Stderr, "contact-perf-receipt: invalid")
			os.Exit(1)
		}
		fmt.Println("contact-perf-receipt: PASS")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	result, err := execute(ctx, opts)
	if result != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if encodeErr := encoder.Encode(result); encodeErr != nil {
			fmt.Fprintln(os.Stderr, "contact-perf: encode evidence failed")
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "contact-perf: hard gate failed")
		os.Exit(1)
	}
}

func parseOptions(arguments []string) (options, error) {
	set := flag.NewFlagSet("aicrm-contact-perf", flag.ContinueOnError)
	set.SetOutput(new(strings.Builder))
	var result options
	set.StringVar(&result.databaseURL, "database-url", "", "isolated performance database URL")
	set.StringVar(&result.databaseURLFile, "database-url-file", "", "root-only isolated performance database URL file")
	set.StringVar(&result.sourceSHA, "source-sha", "", "exact main source SHA")
	set.StringVar(&result.receiptPath, "verify-receipt", "", "verify a saved S-tier receipt")
	set.IntVar(&result.samples, "samples", requiredSamples, "measured calls per combination")
	set.IntVar(&result.warmups, "warmups", requiredWarmups, "warmup calls per combination")
	if err := set.Parse(arguments); err != nil || len(set.Args()) != 0 {
		return options{}, errors.New("invalid arguments")
	}
	if result.receiptPath != "" {
		if result.databaseURL != "" || result.databaseURLFile != "" || result.sourceSHA != "" || result.samples != requiredSamples || result.warmups != requiredWarmups {
			return options{}, errors.New("invalid arguments")
		}
		return result, nil
	}
	if (result.databaseURL == "") == (result.databaseURLFile == "") {
		return options{}, errors.New("invalid arguments")
	}
	if result.databaseURLFile != "" {
		var err error
		result.databaseURL, err = databaseURLFromFile(result.databaseURLFile)
		if err != nil {
			return options{}, errors.New("invalid arguments")
		}
	}
	if err := validateDatabaseURL(result.databaseURL); err != nil || !isExactSHA(result.sourceSHA) ||
		result.samples < requiredSamples || result.warmups < requiredWarmups {
		return options{}, errors.New("invalid arguments")
	}
	return result, nil
}

func databaseURLFromFile(path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", errors.New("database URL file must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() < 1 || info.Size() > 4096 {
		return "", errors.New("database URL file must be a private bounded regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("read database URL file")
	}
	value := strings.TrimSpace(string(contents))
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", errors.New("database URL file must contain one URL")
	}
	return value, nil
}

func verifyReceiptFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 2 || info.Size() > 64<<20 {
		return errors.New("receipt must be a bounded regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return errors.New("read receipt")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var value report
	if err := decoder.Decode(&value); err != nil {
		return errors.New("decode receipt")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	return validateReceipt(value)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("receipt contains trailing JSON")
		}
		return errors.New("receipt contains invalid trailing data")
	}
	return nil
}

func validateReceipt(value report) error {
	if value.Kind != "contact_customer_list_s_tier_hard_gate" || value.EvidenceClass != "authorized_test_server_synthetic" ||
		!value.Passed || value.ThresholdMS != latencyLimit.Milliseconds() || value.CombinationCount != 4096 ||
		len(value.Cases) != 4096 || value.SampleCount < 4096*requiredSamples || value.GlobalP50MS < 0 ||
		value.GlobalP50MS > value.GlobalP95MS || value.GlobalP95MS >= float64(latencyLimit)/float64(time.Millisecond) ||
		value.GlobalP95MS > value.GlobalMaxMS || value.SlowestCase == "" {
		return errors.New("receipt summary is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, value.GeneratedAt); err != nil {
		return errors.New("receipt timestamp is invalid")
	}
	environment := value.Environment
	if !isExactSHA(environment.SourceSHA) || environment.BinaryVCSRevision != environment.SourceSHA || environment.BinaryVCSModified ||
		environment.Database != requiredDatabase || environment.PostgreSQLVersion != "160014" || environment.CPUs != 2 ||
		environment.MemoryKiB < 3_500_000 || environment.MemoryKiB > 4_800_000 || environment.SwapKiB < 4_000_000 ||
		environment.GoMemoryLimitBytes != 768*1024*1024 || environment.SharedBuffers != "1GB" ||
		environment.EffectiveCacheSize != "2GB" || environment.WorkMem != "8MB" || environment.MaxConnections != "40" {
		return errors.New("receipt environment is invalid")
	}
	dataset := value.Dataset
	if dataset.Customers != requiredCustomers || dataset.CustomerTags != requiredTags || dataset.Staff != 64 ||
		dataset.Stages != 8 || dataset.Channels != 12 || dataset.Tags != 50 || dataset.Deleted != requiredCustomers/20 ||
		dataset.HotActive < 500 || dataset.HotDeleted < 500 {
		return errors.New("receipt dataset is invalid")
	}
	expected := make(map[string]scenario, 4096)
	for _, item := range scenarios() {
		expected[scenarioID(item)] = item
	}
	seen := make(map[string]bool, 4096)
	samples := 0
	for _, item := range value.Cases {
		scenarioValue, exists := expected[item.ID]
		if !exists || seen[item.ID] || item.SelectorMask != scenarioValue.selectorMask || item.Deleted != scenarioValue.deleted ||
			item.AddedMode != scenarioValue.addedMode.String() || item.InteractMode != scenarioValue.interactMode.String() ||
			item.Page != pageName(scenarioValue.nextPage) || item.Limit != scenarioValue.limit || item.Samples < requiredSamples ||
			item.P50MS < 0 || item.P50MS > item.P95MS || item.P95MS >= float64(latencyLimit)/float64(time.Millisecond) ||
			item.P95MS > item.MaxMS || item.Matched != int(item.Limit) || !item.HasMore || len(item.Plans) != 2 {
			return errors.New("receipt case matrix is invalid")
		}
		seen[item.ID] = true
		samples += item.Samples
		planNames := map[string]bool{}
		for _, plan := range item.Plans {
			if (plan.Query != "ListCustomerIDsBounded" && plan.Query != "ListCustomers") || planNames[plan.Query] ||
				plan.ExecutionMS < 0 || plan.PlanningMS < 0 || len(plan.NodeTypes) == 0 || len(plan.ForbiddenScans) != 0 {
				return errors.New("receipt query plan is invalid")
			}
			planNames[plan.Query] = true
		}
	}
	if len(seen) != 4096 || samples != value.SampleCount || !seen[value.SlowestCase] {
		return errors.New("receipt coverage is invalid")
	}
	return nil
}

func validateDatabaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "postgres" || parsed.Path != "/"+requiredDatabase ||
		parsed.RawQuery != "sslmode=disable" || parsed.Fragment != "" || parsed.User == nil ||
		parsed.User.Username() == "" || parsed.Hostname() == "" {
		return errors.New("database URL must target the isolated performance database")
	}
	if password, ok := parsed.User.Password(); !ok || password == "" {
		return errors.New("database URL must target the isolated performance database")
	}
	return nil
}

func isExactSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func execute(ctx context.Context, opts options) (*report, error) {
	capture := &queryCapture{}
	config, err := pgxpool.ParseConfig(opts.databaseURL)
	if err != nil {
		return nil, errors.New("parse isolated performance database configuration")
	}
	config.MaxConns = 2
	config.MinConns = 0
	config.ConnConfig.Tracer = capture
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("open isolated performance database")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return nil, errors.New("connect isolated performance database")
	}

	environment, err := inspectEnvironment(ctx, pool, opts.sourceSHA)
	if err != nil {
		return nil, err
	}
	dataset, err := inspectDataset(ctx, pool)
	if err != nil {
		return nil, err
	}

	repository := contactstore.NewCustomerQueryRepository()
	uow := platformstore.NewUnitOfWork(pool)
	result := &report{
		Kind: "contact_customer_list_s_tier_hard_gate", EvidenceClass: "authorized_test_server_synthetic",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Environment: environment,
		Dataset: dataset, ThresholdMS: latencyLimit.Milliseconds(),
	}
	var allDurations []time.Duration
	for _, item := range scenarios() {
		caseResult, durations, caseErr := executeCase(ctx, pool, uow, repository, capture, item, opts)
		if caseErr != nil {
			return result, caseErr
		}
		result.Cases = append(result.Cases, caseResult)
		allDurations = append(allDurations, durations...)
	}
	result.CombinationCount = len(result.Cases)
	result.SampleCount = len(allDurations)
	result.GlobalP50MS = durationMS(percentile(allDurations, 50))
	result.GlobalP95MS = durationMS(percentile95(allDurations))
	result.GlobalMaxMS = durationMS(maxDuration(allDurations))
	for _, item := range result.Cases {
		if item.MaxMS >= result.GlobalMaxMS {
			result.SlowestCase = item.ID
		}
	}
	result.Passed = result.CombinationCount == 4096 && result.GlobalP95MS < float64(latencyLimit)/float64(time.Millisecond)
	for _, item := range result.Cases {
		if item.P95MS >= float64(latencyLimit)/float64(time.Millisecond) {
			result.Passed = false
		}
		for _, plan := range item.Plans {
			if len(plan.ForbiddenScans) != 0 {
				result.Passed = false
			}
		}
	}
	if !result.Passed {
		return result, errors.New("performance threshold or plan contract failed")
	}
	return result, nil
}

func inspectEnvironment(ctx context.Context, pool *pgxpool.Pool, sourceSHA string) (environmentEvidence, error) {
	result := environmentEvidence{
		SourceSHA: sourceSHA, CPUs: runtime.NumCPU(), GoMemoryLimitBytes: debug.SetMemoryLimit(-1),
	}
	revision, modified, err := binaryVCS()
	if err != nil || revision != sourceSHA || modified {
		return environmentEvidence{}, errors.New("performance binary is not the clean exact source SHA")
	}
	result.BinaryVCSRevision, result.BinaryVCSModified = revision, modified
	if err := pool.QueryRow(ctx, `SELECT current_database(), current_setting('server_version_num'), current_setting('shared_buffers'), current_setting('effective_cache_size'), current_setting('work_mem'), current_setting('max_connections')`).Scan(
		&result.Database, &result.PostgreSQLVersion, &result.SharedBuffers, &result.EffectiveCacheSize, &result.WorkMem, &result.MaxConnections,
	); err != nil {
		return environmentEvidence{}, errors.New("inspect performance database environment")
	}
	memory, swap, err := linuxMemoryEvidence("/proc/meminfo")
	if err != nil {
		return environmentEvidence{}, err
	}
	result.MemoryKiB, result.SwapKiB = memory, swap
	if result.Database != requiredDatabase || result.PostgreSQLVersion != "160014" || result.CPUs != 2 ||
		result.MemoryKiB < 3_500_000 || result.MemoryKiB > 4_800_000 ||
		result.SwapKiB < 4_000_000 || result.GoMemoryLimitBytes != 768*1024*1024 ||
		result.SharedBuffers != "1GB" || result.EffectiveCacheSize != "2GB" || result.WorkMem != "8MB" || result.MaxConnections != "40" {
		return environmentEvidence{}, errors.New("test target is not the frozen PostgreSQL 16.14 S tier")
	}
	return result, nil
}

func binaryVCS() (string, bool, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false, errors.New("Go build information is unavailable")
	}
	var revision, modified string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	if !isExactSHA(revision) || (modified != "true" && modified != "false") {
		return "", false, errors.New("Go VCS build information is incomplete")
	}
	return revision, modified == "true", nil
}

func linuxMemoryEvidence(path string) (int64, int64, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, errors.New("read Linux memory evidence")
	}
	var memory, swap int64
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && (fields[0] == "MemTotal:" || fields[0] == "SwapTotal:") && fields[2] == "kB" {
			value, parseErr := strconv.ParseInt(fields[1], 10, 64)
			if parseErr == nil && value > 0 {
				if fields[0] == "MemTotal:" {
					memory = value
				} else {
					swap = value
				}
			}
		}
	}
	if memory == 0 || swap == 0 {
		return 0, 0, errors.New("Linux memory evidence is invalid")
	}
	return memory, swap, nil
}

func inspectDataset(ctx context.Context, pool *pgxpool.Pool) (datasetEvidence, error) {
	var result datasetEvidence
	err := pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM customers),
  (SELECT count(*) FROM customer_tags),
  (SELECT count(*) FROM staff),
  (SELECT count(*) FROM stages),
  (SELECT count(*) FROM channels),
  (SELECT count(*) FROM tags),
  (SELECT count(*) FROM customers WHERE is_deleted),
  (SELECT count(*) FROM customers AS c WHERE NOT c.is_deleted AND lower(c.name) % lower('kw017') AND c.owner_staff_id = 7 AND c.stage_id = 3 AND c.channel_id = 5 AND c.added_at BETWEEN '2026-04-01T00:00:00Z' AND '2026-07-31T23:59:59Z' AND c.last_interact_at BETWEEN '2026-05-01T00:00:00Z' AND '2026-08-11T23:59:59Z' AND EXISTS (SELECT 1 FROM customer_tags AS ct WHERE ct.customer_id = c.id AND ct.tag_id = 11)),
  (SELECT count(*) FROM customers AS c WHERE c.is_deleted AND lower(c.name) % lower('kw017') AND c.owner_staff_id = 7 AND c.stage_id = 3 AND c.channel_id = 5 AND c.added_at BETWEEN '2026-04-01T00:00:00Z' AND '2026-07-31T23:59:59Z' AND c.last_interact_at BETWEEN '2026-05-01T00:00:00Z' AND '2026-08-11T23:59:59Z' AND EXISTS (SELECT 1 FROM customer_tags AS ct WHERE ct.customer_id = c.id AND ct.tag_id = 11))
`).Scan(&result.Customers, &result.CustomerTags, &result.Staff, &result.Stages, &result.Channels, &result.Tags, &result.Deleted, &result.HotActive, &result.HotDeleted)
	if err != nil {
		return datasetEvidence{}, errors.New("inspect performance dataset")
	}
	if result.Customers != requiredCustomers || result.CustomerTags != requiredTags || result.Staff != 64 ||
		result.Stages != 8 || result.Channels != 12 || result.Tags != 50 || result.Deleted != requiredCustomers/20 ||
		result.HotActive < 500 || result.HotDeleted < 500 {
		return datasetEvidence{}, errors.New("performance dataset does not match the frozen deterministic distribution")
	}
	return result, nil
}

func scenarios() []scenario {
	result := make([]scenario, 0, 4096)
	for selectorMask := 0; selectorMask < 1<<selectorGroups; selectorMask++ {
		for deletedIndex := 0; deletedIndex < 2; deletedIndex++ {
			for added := timeNone; added <= timeClosed; added++ {
				for interact := timeNone; interact <= timeClosed; interact++ {
					for pageIndex := 0; pageIndex < 2; pageIndex++ {
						for _, limit := range []int32{contactapp.CustomerListDefaultLimit, contactapp.CustomerListMaximumLimit} {
							result = append(result, scenario{
								selectorMask: selectorMask, deleted: deletedIndex == 1,
								addedMode: added, interactMode: interact, nextPage: pageIndex == 1, limit: limit,
							})
						}
					}
				}
			}
		}
	}
	return result
}

func queryForScenario(item scenario) contactapp.CustomerListQuery {
	query := contactapp.CustomerListQuery{Watermark: benchmarkWatermark, IsDeleted: item.deleted, Limit: item.limit}
	if item.selectorMask&1 != 0 {
		query.Keyword = "kw017"
	}
	if item.selectorMask&2 != 0 {
		query.OwnerStaffID = int64Pointer(7)
	}
	if item.selectorMask&4 != 0 {
		query.StageID = int64Pointer(3)
	}
	if item.selectorMask&8 != 0 {
		query.ChannelID = int64Pointer(5)
	}
	if item.selectorMask&16 != 0 {
		query.TagID = int64Pointer(11)
	}
	query.AddedAfter, query.AddedBefore = timeRange(item.addedMode, addedAfter, addedBefore)
	query.LastInteractAfter, query.LastInteractBefore = timeRange(item.interactMode, interactAfter, interactBefore)
	return query
}

func timeRange(mode timeMode, after, before time.Time) (*time.Time, *time.Time) {
	switch mode {
	case timeNone:
		return nil, nil
	case timeAfter:
		return timePointer(after), nil
	case timeBefore:
		return nil, timePointer(before)
	case timeClosed:
		return timePointer(after), timePointer(before)
	default:
		return nil, nil
	}
}

func (mode timeMode) String() string {
	switch mode {
	case timeNone:
		return "none"
	case timeAfter:
		return "after"
	case timeBefore:
		return "before"
	case timeClosed:
		return "closed"
	default:
		return "invalid"
	}
}

func executeCase(
	ctx context.Context,
	pool *pgxpool.Pool,
	uow *platformstore.UnitOfWork,
	repository *contactstore.CustomerQueryRepository,
	capture *queryCapture,
	item scenario,
	opts options,
) (caseEvidence, []time.Duration, error) {
	query := queryForScenario(item)
	id := scenarioID(item)
	var anchor contactapp.CustomerListStoreResult
	if item.nextPage {
		var err error
		anchor, err = callRepository(ctx, uow, repository, query)
		if err != nil || !anchor.HasMore || len(anchor.Items) != int(item.limit) {
			return caseEvidence{}, nil, fmt.Errorf("prepare real keyset %s failed", id)
		}
		query, err = continuationQuery(query, anchor)
		if err != nil {
			return caseEvidence{}, nil, fmt.Errorf("prepare real keyset %s: %w", id, err)
		}
	}
	for range opts.warmups {
		if _, err := callRepository(ctx, uow, repository, query); err != nil {
			return caseEvidence{}, nil, fmt.Errorf("warm up %s: %w", id, err)
		}
	}
	capture.begin()
	first, err := callRepository(ctx, uow, repository, query)
	if err != nil {
		return caseEvidence{}, nil, fmt.Errorf("capture %s: %w", id, err)
	}
	captured, err := capture.finish()
	if err != nil {
		return caseEvidence{}, nil, fmt.Errorf("capture %s: %w", id, err)
	}
	plans := make([]planEvidence, 0, len(captured))
	for _, statement := range captured {
		plan, explainErr := explainQuery(ctx, pool, statement)
		if explainErr != nil {
			return caseEvidence{}, nil, fmt.Errorf("explain %s/%s: %w", id, statement.Name, explainErr)
		}
		plans = append(plans, plan)
	}

	durations := make([]time.Duration, 0, opts.samples)
	for range opts.samples {
		started := time.Now()
		page, callErr := callRepository(ctx, uow, repository, query)
		durations = append(durations, time.Since(started))
		if callErr != nil {
			return caseEvidence{}, nil, fmt.Errorf("measure %s: %w", id, callErr)
		}
		if item.nextPage && overlaps(anchor, page) {
			return caseEvidence{}, nil, fmt.Errorf("keyset continuation %s overlaps first page", id)
		}
	}
	return caseEvidence{
		ID: id, SelectorMask: item.selectorMask, Deleted: item.deleted, AddedMode: item.addedMode.String(),
		InteractMode: item.interactMode.String(), Page: pageName(item.nextPage), Limit: item.limit,
		Samples: opts.samples, P50MS: durationMS(percentile(durations, 50)),
		P95MS: durationMS(percentile95(durations)), MaxMS: durationMS(maxDuration(durations)),
		Matched: len(first.Items), HasMore: first.HasMore, Plans: plans,
	}, durations, nil
}

func pageName(next bool) string {
	if next {
		return "next"
	}
	return "first"
}

func scenarioID(item scenario) string {
	return fmt.Sprintf("selectors-%02d-deleted-%t-added-%s-interact-%s-page-%s-limit-%d",
		item.selectorMask, item.deleted, item.addedMode, item.interactMode, pageName(item.nextPage), item.limit)
}

func callRepository(ctx context.Context, uow *platformstore.UnitOfWork, repository *contactstore.CustomerQueryRepository, query contactapp.CustomerListQuery) (contactapp.CustomerListStoreResult, error) {
	var result contactapp.CustomerListStoreResult
	err := uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		result, storeErr = repository.ListCustomers(txCtx, query)
		return storeErr
	})
	return result, err
}

func continuationQuery(query contactapp.CustomerListQuery, page contactapp.CustomerListStoreResult) (contactapp.CustomerListQuery, error) {
	if !page.HasMore || len(page.Items) == 0 {
		return contactapp.CustomerListQuery{}, errors.New("continuation requires a non-empty page with has_more")
	}
	last := page.Items[len(page.Items)-1]
	updatedAt := last.UpdatedAt
	id := last.ID
	query.AfterUpdatedAt, query.AfterID = &updatedAt, &id
	return query, nil
}

func overlaps(first, second contactapp.CustomerListStoreResult) bool {
	seen := make(map[contactport.CustomerID]struct{}, len(first.Items))
	for _, item := range first.Items {
		seen[item.ID] = struct{}{}
	}
	for _, item := range second.Items {
		if _, exists := seen[item.ID]; exists {
			return true
		}
	}
	return false
}

func explainQuery(ctx context.Context, pool *pgxpool.Pool, statement capturedQuery) (planEvidence, error) {
	var raw []byte
	if err := pool.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+statement.SQL, statement.Args...).Scan(&raw); err != nil {
		return planEvidence{}, errors.New("execute exact captured query plan")
	}
	var roots []struct {
		Plan          map[string]any `json:"Plan"`
		PlanningTime  float64        `json:"Planning Time"`
		ExecutionTime float64        `json:"Execution Time"`
	}
	if err := json.Unmarshal(raw, &roots); err != nil || len(roots) != 1 || roots[0].Plan == nil {
		return planEvidence{}, errors.New("decode exact captured query plan")
	}
	result := planEvidence{Query: statement.Name, PlanningMS: roots[0].PlanningTime, ExecutionMS: roots[0].ExecutionTime}
	walkPlan(roots[0].Plan, &result)
	sort.Strings(result.NodeTypes)
	sort.Strings(result.ForbiddenScans)
	result.NodeTypes = uniqueStrings(result.NodeTypes)
	result.ForbiddenScans = uniqueStrings(result.ForbiddenScans)
	return result, nil
}

func walkPlan(node map[string]any, evidence *planEvidence) {
	nodeType, _ := node["Node Type"].(string)
	relation, _ := node["Relation Name"].(string)
	if nodeType != "" {
		evidence.NodeTypes = append(evidence.NodeTypes, nodeType)
	}
	if nodeType == "Seq Scan" && (relation == "customers" || relation == "customer_tags") {
		evidence.ForbiddenScans = append(evidence.ForbiddenScans, relation)
	}
	evidence.SharedHit += jsonInt64(node["Shared Hit Blocks"])
	evidence.SharedRead += jsonInt64(node["Shared Read Blocks"])
	children, _ := node["Plans"].([]any)
	for _, child := range children {
		if typed, ok := child.(map[string]any); ok {
			walkPlan(typed, evidence)
		}
	}
}

func jsonInt64(value any) int64 {
	number, ok := value.(float64)
	if !ok || number < 0 {
		return 0
	}
	return int64(number)
}

func percentile95(values []time.Duration) time.Duration {
	return percentile(values, 95)
}

func percentile(values []time.Duration, rank int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	if rank < 1 || rank > 100 {
		return 0
	}
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := (rank*len(ordered) + 99) / 100
	return ordered[index-1]
}

func maxDuration(values []time.Duration) time.Duration {
	var maximum time.Duration
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func durationMS(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func int64Pointer(value int64) *int64        { return &value }
func timePointer(value time.Time) *time.Time { return &value }
