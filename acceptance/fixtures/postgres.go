// Package fixtures provides PostgreSQL fixtures that are isolated from business tables.
package fixtures

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	Schema                         = "acceptance_fixtures"
	DefaultDatabaseURL             = "postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable"
	H01A1DatabaseName              = "aicrm_test_h01a1"
	H03DatabaseName                = "aicrm_test_h03"
	I01BDatabaseName               = "aicrm_test_i01b"
	F01ADatabaseName               = "aicrm_test_f01a"
	F01ABDatabaseName              = "aicrm_test_f01ab"
	F01PublicDatabaseName          = "aicrm_test_f01public"
	AutomationAgentsABDatabaseName = "aicrm_test_automation_agents_ab"
	AdminOpsABDatabaseName         = "aicrm_test_adminops_ab"
	C01ChannelDatabaseName         = "aicrm_test_c01_channel"
	J01CouponDatabaseName          = "aicrm_test_j01_coupon"
	CouponABDatabaseName           = "aicrm_test_coupon_ab"
	I03OrderDatabaseName           = "aicrm_test_i03_order"
	OrderABDatabaseName            = "aicrm_test_order_ab"
	MessageArchiveDatabaseName     = "aicrm_test_message_archive"
	PushCenterDatabaseName         = "aicrm_test_pushcenter"
	MiniProgramLibraryDatabaseName = "aicrm_test_miniprogram_library"
	ImageUpdateDatabaseName        = "aicrm_test_image_update"
	HXCSenderDatabaseName          = "aicrm_test_hxc_sender"
	advisoryLockKey                = int64(0x414943524d503230)
)

var (
	ErrUnsafeDatabaseURL = errors.New("acceptance fixtures require the loopback postgres aicrm_test database")
	ErrFixtureClosed     = errors.New("acceptance fixture is closed")
)

// PostgreSQL owns the fixed acceptance_fixtures schema for one test process.
// The advisory lock prevents concurrent suites from resetting each other's fixtures.
type PostgreSQL struct {
	pool     *pgxpool.Pool
	lockConn *pgxpool.Conn
	mu       sync.Mutex
	closed   bool
}

func OpenPostgreSQL(ctx context.Context, databaseURL string) (*PostgreSQL, error) {
	if err := ValidateDatabaseURL(databaseURL); err != nil {
		return nil, err
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse acceptance database URL: %w", err)
	}
	config.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open acceptance pool: %w", err)
	}
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("acquire acceptance lock connection: %w", err)
	}
	fixture := &PostgreSQL{pool: pool, lockConn: lockConn}
	if _, err = lockConn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		lockConn.Release()
		pool.Close()
		return nil, fmt.Errorf("lock acceptance schema: %w", err)
	}
	if err = fixture.resetSchema(ctx); err != nil {
		_ = fixture.Cleanup(ctx)
		return nil, err
	}
	return fixture, nil
}

// ValidateDatabaseURL accepts only the dedicated loopback acceptance database.
// It never includes the supplied URL in an error, so rejected credentials cannot leak.
func ValidateDatabaseURL(databaseURL string) error {
	return ValidateDatabaseURLForDatabase(databaseURL, "aicrm_test")
}

// ValidateDatabaseURLForDatabase accepts only named, locally-created
// acceptance databases on the dedicated loopback PostgreSQL service.
func ValidateDatabaseURLForDatabase(databaseURL, databaseName string) error {
	if databaseName != "aicrm_test" && databaseName != H01A1DatabaseName && databaseName != H03DatabaseName && databaseName != I01BDatabaseName && databaseName != F01ADatabaseName && databaseName != F01ABDatabaseName && databaseName != F01PublicDatabaseName && databaseName != AutomationAgentsABDatabaseName && databaseName != AdminOpsABDatabaseName && databaseName != C01ChannelDatabaseName && databaseName != J01CouponDatabaseName && databaseName != CouponABDatabaseName && databaseName != I03OrderDatabaseName && databaseName != OrderABDatabaseName && databaseName != MessageArchiveDatabaseName && databaseName != PushCenterDatabaseName && databaseName != MiniProgramLibraryDatabaseName && databaseName != ImageUpdateDatabaseName && databaseName != HXCSenderDatabaseName {
		return ErrUnsafeDatabaseURL
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || parsed.Scheme != "postgres" || parsed.Path != "/"+databaseName || parsed.RawPath != "" || parsed.RawQuery != "sslmode=disable" || parsed.Fragment != "" {
		return ErrUnsafeDatabaseURL
	}
	if parsed.User == nil {
		return ErrUnsafeDatabaseURL
	}
	password, hasPassword := parsed.User.Password()
	if parsed.User.Username() != "postgres" || !hasPassword || password != "postgres" {
		return ErrUnsafeDatabaseURL
	}
	host, portText := parsed.Hostname(), parsed.Port()
	if (host != "127.0.0.1" && host != "::1") || portText == "" {
		return ErrUnsafeDatabaseURL
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return ErrUnsafeDatabaseURL
	}
	return nil
}

func (f *PostgreSQL) Pool() *pgxpool.Pool {
	return f.pool
}

func (f *PostgreSQL) Begin(ctx context.Context) (pgx.Tx, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, ErrFixtureClosed
	}
	return f.pool.Begin(ctx)
}

func (f *PostgreSQL) Rollback(ctx context.Context, tx pgx.Tx) error {
	if tx == nil {
		return errors.New("acceptance fixture rollback requires a transaction")
	}
	err := tx.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return err
}

func (f *PostgreSQL) Cleanup(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	var cleanupErr error
	if _, err := f.lockConn.Exec(ctx, `DROP SCHEMA IF EXISTS acceptance_fixtures CASCADE`); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("drop acceptance schema: %w", err))
	}
	var schema *string
	if err := f.lockConn.QueryRow(ctx, `SELECT to_regnamespace('acceptance_fixtures')::text`).Scan(&schema); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("verify acceptance schema cleanup: %w", err))
	} else if schema != nil {
		cleanupErr = errors.Join(cleanupErr, errors.New("acceptance fixture schema still exists after cleanup"))
	}
	if _, err := f.lockConn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("unlock acceptance schema: %w", err))
	}
	f.lockConn.Release()
	f.pool.Close()
	return cleanupErr
}

func (f *PostgreSQL) resetSchema(ctx context.Context) error {
	if _, err := f.lockConn.Exec(ctx, `DROP SCHEMA IF EXISTS acceptance_fixtures CASCADE`); err != nil {
		return fmt.Errorf("reset acceptance schema: %w", err)
	}
	if _, err := f.lockConn.Exec(ctx, `CREATE SCHEMA acceptance_fixtures`); err != nil {
		return fmt.Errorf("create acceptance schema: %w", err)
	}
	return nil
}
