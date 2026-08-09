package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const fixedDatabaseURL = "postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable"

var (
	hexSHA        = regexp.MustCompile(`^[0-9a-f]{40}$`)
	queryPath     = regexp.MustCompile(`^internal/[^/]+/store/queries/[^/]+[.]sql$`)
	migrationPath = regexp.MustCompile(`^migrations/[^/]+[.]sql$`)
	queryHeader   = regexp.MustCompile(`(?i)^--[[:space:]]+name:[[:space:]]+([A-Za-z_][A-Za-z0-9_]*)[[:space:]]+:[A-Za-z_]+[[:space:]]*$`)
	targetTable   = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(customers|customer_events)([^A-Za-z0-9_]|$)`)
	positional    = regexp.MustCompile(`[$]([1-9][0-9]*)`)
	namedArg      = regexp.MustCompile(`(?i)sqlc[.](?:n?arg)[(][[:space:]]*'?([A-Za-z_][A-Za-z0-9_]*)'?[[:space:]]*[)]`)
	atArg         = regexp.MustCompile(`@([A-Za-z_][A-Za-z0-9_]*)`)
)

type query struct{ File, Name, SQL string }

func main() {
	root := flag.String("root", ".", "repository root")
	base := flag.String("base", "", "base commit")
	head := flag.String("head", "", "head commit")
	databaseURL := flag.String("database-url", "", "fixed loopback PostgreSQL URL")
	flag.Parse()
	if err := run(context.Background(), *root, *base, *head, *databaseURL); err != nil {
		fmt.Fprintln(os.Stderr, "query-plan-gate:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, root, base, head, databaseURL string) error {
	if databaseURL != fixedDatabaseURL {
		return errors.New("QUERY_PLAN_TEST_DATABASE_URL must equal the fixed loopback aicrm_test DSN")
	}
	if !hexSHA.MatchString(base) || !hexSHA.MatchString(head) {
		return errors.New("base and head must be exact 40-character lowercase Git SHAs")
	}
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	physical, err = filepath.Abs(physical)
	if err != nil {
		return fmt.Errorf("make repository root absolute: %w", err)
	}
	paths, migrationChanged, err := changedQueryPaths(physical, base, head)
	if err != nil {
		return err
	}
	if migrationChanged {
		paths, err = filepath.Glob(filepath.Join(physical, "internal", "*", "store", "queries", "*.sql"))
		if err != nil {
			return fmt.Errorf("enumerate SQLc queries: %w", err)
		}
		for i := range paths {
			paths[i], _ = filepath.Rel(physical, paths[i])
		}
	}
	sort.Strings(paths)
	queries, err := loadRelevantQueries(physical, paths)
	if err != nil {
		return err
	}
	if len(queries) == 0 {
		fmt.Println("query-plan-gate: PASS (checked=0)")
		return nil
	}
	tempURL, cleanup, err := prepareDatabase(ctx, physical, databaseURL)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := inspectPlans(ctx, tempURL, queries); err != nil {
		return err
	}
	fmt.Printf("query-plan-gate: PASS (checked=%d)\n", len(queries))
	return nil
}

func changedQueryPaths(root, base, head string) ([]string, bool, error) {
	for _, sha := range []string{base, head} {
		if err := exec.Command("git", "-C", root, "cat-file", "-e", sha+"^{commit}").Run(); err != nil {
			return nil, false, fmt.Errorf("Git commit is unavailable: %s", sha)
		}
	}
	out, err := exec.Command("git", "-C", root, "diff", "--name-only", "--diff-filter=ACMRTD", base, head, "--").Output()
	if err != nil {
		return nil, false, fmt.Errorf("read changed paths: %w", err)
	}
	seen := map[string]bool{}
	migrationChanged := false
	for _, path := range strings.Fields(string(out)) {
		switch {
		case migrationPath.MatchString(path):
			migrationChanged = true
		case queryPath.MatchString(path):
			if info, statErr := os.Lstat(filepath.Join(root, path)); statErr == nil && info.Mode().IsRegular() {
				seen[path] = true
			}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	return paths, migrationChanged, nil
}

func loadRelevantQueries(root string, paths []string) ([]query, error) {
	var result []query
	for _, path := range paths {
		full := filepath.Join(root, path)
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("regular SQL query file required: %s", path)
		}
		fileQueries, err := parseQueryFile(path, full)
		if err != nil {
			return nil, err
		}
		result = append(result, fileQueries...)
	}
	return result, nil
}

func parseQueryFile(path, full string) ([]query, error) {
	f, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var result []query
	var name string
	var body []string
	flush := func() {
		sqlText := strings.TrimSpace(strings.Join(body, "\n"))
		if name != "" && targetTable.MatchString(sqlText) {
			result = append(result, query{File: path, Name: name, SQL: sqlText})
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if match := queryHeader.FindStringSubmatch(line); match != nil {
			flush()
			name, body = match[1], nil
			continue
		}
		body = append(body, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	flush()
	if name == "" && targetTable.MatchString(strings.Join(body, "\n")) {
		return nil, fmt.Errorf("target-table SQL is missing a sqlc name block: %s", path)
	}
	return result, nil
}

func prepareDatabase(ctx context.Context, root, databaseURL string) (string, func(), error) {
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return "", nil, fmt.Errorf("parse database URL: %w", err)
	}
	adminConfig.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		return "", nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	var version string
	if err := admin.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		admin.Close(ctx)
		return "", nil, fmt.Errorf("PostgreSQL server_version_num must equal 160014 (got %q)", version)
	}
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		admin.Close(ctx)
		return "", nil, fmt.Errorf("generate temporary database name: %w", err)
	}
	name := "aicrm_query_plan_" + hex.EncodeToString(random)
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		admin.Close(ctx)
		return "", nil, fmt.Errorf("create temporary database: %w", err)
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanupCtx, "DROP DATABASE "+identifier+" WITH (FORCE)")
		_ = admin.Close(cleanupCtx)
	}
	tempURL, err := temporaryDatabaseURL(databaseURL, name)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	db, err := sql.Open("pgx", tempURL)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("open temporary database: %w", err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		db.Close()
		cleanup()
		return "", nil, fmt.Errorf("set migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, filepath.Join(root, "migrations")); err != nil {
		db.Close()
		cleanup()
		return "", nil, fmt.Errorf("apply migrations to temporary database: %w", err)
	}
	if err := db.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close migration connection: %w", err)
	}
	return tempURL, cleanup, nil
}

func temporaryDatabaseURL(databaseURL, name string) (string, error) {
	marker := "/aicrm_test?"
	if !regexp.MustCompile(`^[a-z][a-z0-9_]+$`).MatchString(name) || strings.Count(databaseURL, marker) != 1 {
		return "", errors.New("cannot construct isolated temporary database URL")
	}
	return strings.Replace(databaseURL, marker, "/"+name+"?", 1), nil
}

func inspectPlans(ctx context.Context, databaseURL string, queries []query) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open plan connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SET plan_cache_mode = force_generic_plan"); err != nil {
		return fmt.Errorf("force generic plans: %w", err)
	}
	for i, item := range queries {
		sqlText, count, err := normalizeParams(item.SQL)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", item.File, item.Name, err)
		}
		statement := "qp_" + strconv.Itoa(i+1)
		if _, err := conn.ExecContext(ctx, "PREPARE "+statement+" AS "+sqlText); err != nil {
			return fmt.Errorf("prepare %s/%s: %w", item.File, item.Name, err)
		}
		args := strings.TrimSuffix(strings.Repeat("NULL,", count), ",")
		call := "EXPLAIN (FORMAT JSON) EXECUTE " + statement
		if count > 0 {
			call += "(" + args + ")"
		}
		var raw string
		if err := conn.QueryRowContext(ctx, call).Scan(&raw); err != nil {
			return fmt.Errorf("explain %s/%s: %w", item.File, item.Name, err)
		}
		if _, err := conn.ExecContext(ctx, "DEALLOCATE "+statement); err != nil {
			return fmt.Errorf("deallocate %s/%s: %w", item.File, item.Name, err)
		}
		bad, err := containsTargetSeqScan([]byte(raw))
		if err != nil {
			return fmt.Errorf("invalid EXPLAIN JSON for %s/%s: %w", item.File, item.Name, err)
		}
		if bad != "" {
			return fmt.Errorf("Seq Scan on %s: %s/%s", bad, item.File, item.Name)
		}
	}
	return nil
}

func normalizeParams(input string) (string, int, error) {
	sqlText := strings.TrimSpace(input)
	sqlText = strings.TrimSuffix(sqlText, ";")
	if strings.Contains(sqlText, ";") || sqlText == "" {
		return "", 0, errors.New("query must contain exactly one SQL statement")
	}
	max := 0
	for _, match := range positional.FindAllStringSubmatch(sqlText, -1) {
		n, _ := strconv.Atoi(match[1])
		if n > max {
			max = n
		}
	}
	names := map[string]int{}
	replace := func(name string) string {
		if names[name] == 0 {
			max++
			names[name] = max
		}
		return "$" + strconv.Itoa(names[name])
	}
	sqlText = namedArg.ReplaceAllStringFunc(sqlText, func(value string) string {
		return replace(namedArg.FindStringSubmatch(value)[1])
	})
	sqlText = atArg.ReplaceAllStringFunc(sqlText, func(value string) string {
		return replace(strings.TrimPrefix(value, "@"))
	})
	if strings.Contains(strings.ToLower(sqlText), "sqlc.") || atArg.MatchString(sqlText) {
		return "", 0, errors.New("unresolved SQL parameter syntax")
	}
	return sqlText, max, nil
}

func containsTargetSeqScan(raw []byte) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	var walk func(any) string
	walk = func(current any) string {
		switch typed := current.(type) {
		case []any:
			for _, child := range typed {
				if found := walk(child); found != "" {
					return found
				}
			}
		case map[string]any:
			if typed["Node Type"] == "Seq Scan" {
				if relation, ok := typed["Relation Name"].(string); ok && (relation == "customers" || relation == "customer_events" || strings.HasPrefix(relation, "customer_events_")) {
					return relation
				}
			}
			for _, child := range typed {
				if found := walk(child); found != "" {
					return found
				}
			}
		}
		return ""
	}
	found := walk(value)
	if found == "" {
		root, ok := value.([]any)
		if !ok || len(root) == 0 {
			return "", errors.New("EXPLAIN result must be a non-empty JSON array")
		}
	}
	return found, nil
}
