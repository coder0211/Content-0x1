// Package postgres provides the pgx pool, the context-carried transaction
// manager (ports.TxManager) and error helpers used by every sqlc repository.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/shared/kernel"
)

// DBTX is the interface sqlc-generated code runs on; both *pgxpool.Pool and
// pgx.Tx satisfy it.
type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

// Connect opens a pool and verifies connectivity. NUMERIC columns are
// registered to decode into shopspring decimal.Decimal.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.AfterConnect = func(_ context.Context, c *pgx.Conn) error {
		pgxdecimal.Register(c.TypeMap())
		return nil
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

type txKey struct{}

// TxManager implements ports.TxManager on a pgx pool.
type TxManager struct{ pool *pgxpool.Pool }

// NewTxManager returns a TxManager for pool.
func NewTxManager(pool *pgxpool.Pool) *TxManager { return &TxManager{pool: pool} }

// WithinTx runs fn in a transaction stored in ctx. If ctx already carries a
// transaction, fn joins it (no savepoint): the outermost caller owns commit.
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Conn returns the transaction carried by ctx, or the pool. Repositories call
// it on every operation: queries := New(postgres.Conn(ctx, r.pool)).
func Conn(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}

// InTx reports whether ctx carries a transaction. APIs documented as
// "must be called inside the caller's transaction" may assert it.
func InTx(ctx context.Context) bool {
	_, ok := ctx.Value(txKey{}).(pgx.Tx)
	return ok
}

// Postgres error codes used for mapping.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
	codeCheckViolation      = "23514"
	codeSerialization       = "40001"
)

// IsNoRows reports whether err is pgx.ErrNoRows.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// IsUniqueViolation reports a unique violation, optionally on constraint
// (pass "" for any constraint).
func IsUniqueViolation(err error, constraint string) bool {
	return isCode(err, codeUniqueViolation, constraint)
}

// IsForeignKeyViolation reports a FK violation, optionally on constraint.
func IsForeignKeyViolation(err error, constraint string) bool {
	return isCode(err, codeForeignKeyViolation, constraint)
}

// IsCheckViolation reports a CHECK violation, optionally on constraint.
func IsCheckViolation(err error, constraint string) bool {
	return isCode(err, codeCheckViolation, constraint)
}

func isCode(err error, code, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// MapNotFound converts pgx.ErrNoRows into kernel.ErrNotFound and leaves other
// errors untouched.
func MapNotFound(err error) error {
	if IsNoRows(err) {
		return kernel.ErrNotFound
	}
	return err
}
