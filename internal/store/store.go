// Package store owns the Package A PostgreSQL boundaries. Protected operations
// require an authenticated internal User ID; this package does not authenticate it.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound        = errors.New("workspace not found") // Identical for inaccessible and nonexistent objects.
	ErrConflict        = errors.New("workspace version conflict")
	ErrInvalid         = errors.New("invalid command")
	ErrUnavailable     = errors.New("persistence unavailable")
	ErrUnsafeRole      = errors.New("unsafe database runtime role")
	ErrLeaseLost       = errors.New("outbox lease lost")
	ErrInvalidEvent    = errors.New("invalid event")
	ErrCommitUncertain = errors.New("consumer commit outcome uncertain")
)

// Fault intentionally omits PostgreSQL error text, which may contain row data.
// SQLState and Operation provide bounded diagnostic context without secrets.
type Fault struct {
	Operation, SQLState string
	cause               error
}

func (e *Fault) Error() string        { return "persistence " + e.Operation + " failed (" + e.SQLState + ")" }
func (e *Fault) Unwrap() error        { return e.cause }
func (e *Fault) Is(target error) bool { return target == ErrUnavailable }
func fault(op string, err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	state := "unavailable"
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		state = pgErr.Code
	}
	return &Fault{Operation: op, SQLState: state, cause: err}
}

type Store struct {
	pool *pgxpool.Pool
	role string
}

// Open validates the actual runtime login; migrations must use a separate login.
// A login must inherit its process capability, never both API and worker.
func Open(ctx context.Context, dsn, role string) (*Store, error) {
	if role != "api" && role != "worker" {
		return nil, ErrUnsafeRole
	}
	if strings.TrimSpace(dsn) == "" {
		return nil, ErrInvalid
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, ErrInvalid
	}
	cfg.MaxConns = 8
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "3000"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "15000"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fault("connect", err)
	}
	s := &Store{pool: pool, role: role}
	if err = s.validateRole(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err = s.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) validateRole(ctx context.Context) error {
	var unsafe, api, worker, owns bool
	// Check the actual requested login as well as current/session roles: a
	// privileged startup session_authorization can otherwise disguise both.
	err := s.pool.QueryRow(ctx, `SELECT current_user <> session_user OR session_user::text <> $1 OR EXISTS(SELECT FROM pg_roles privileged
 WHERE pg_has_role(current_user,privileged.oid,'MEMBER') AND
 (privileged.rolsuper OR privileged.rolbypassrls OR privileged.rolcreaterole OR privileged.rolcreatedb OR
 privileged.rolname IN ('pg_read_all_data','pg_write_all_data','pg_read_server_files','pg_write_server_files','pg_execute_server_program'))),
pg_has_role(current_user,'foundation_api','MEMBER'), pg_has_role(current_user,'foundation_worker','MEMBER'),
EXISTS(SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname IN ('foundation_schema','identity','organizations','audit','eventing') AND pg_has_role(current_user,c.relowner,'MEMBER'))
FROM pg_roles r WHERE r.rolname=current_user`, s.pool.Config().ConnConfig.User).Scan(&unsafe, &api, &worker, &owns)
	if err != nil {
		return fault("role_check", err)
	}
	if unsafe || owns || (s.role == "api" && (!api || worker)) || (s.role == "worker" && (!worker || api)) {
		return ErrUnsafeRole
	}
	return nil
}

func (s *Store) Close() { s.pool.Close() }
func (s *Store) Ping(ctx context.Context) error {
	// Validate a relation the process actually depends on, not merely the socket.
	query := `SELECT count(*) FROM organizations.workspaces WHERE false`
	if s.role == "worker" {
		query = `SELECT count(*) FROM eventing.outbox WHERE false`
	}
	var count int64
	if err := s.pool.QueryRow(ctx, query).Scan(&count); err != nil {
		return fault("readiness", err)
	}
	return nil
}

func validID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil
}
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate identifier: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

func rollback(tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := tx.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	if err != nil {
		return fault("rollback", err)
	}
	return nil
}

// protectedTX binds principal and scope transaction-locally. SET LOCAL values
// disappear on commit/rollback before the connection returns to the pool.
func (s *Store) protectedTX(ctx context.Context, actor, workspace, permission string) (pgx.Tx, error) {
	if s.role != "api" {
		return nil, ErrUnsafeRole
	}
	if !validID(actor) || !validID(workspace) {
		return nil, ErrNotFound
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fault("begin", err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.actor_id',$1,true),set_config('app.workspace_id',$2,true)`, actor, workspace); err != nil {
		return nil, errors.Join(fault("tenant_context", err), rollback(tx))
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT organizations.authorize_workspace($1)`, permission).Scan(&allowed); err != nil {
		return nil, errors.Join(fault("authorize", err), rollback(tx))
	}
	if !allowed {
		return nil, errors.Join(ErrNotFound, rollback(tx))
	}
	return tx, nil
}
