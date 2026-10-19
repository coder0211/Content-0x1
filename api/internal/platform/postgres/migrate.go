package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
	"github.com/pressly/goose/v3"
)

// Migrator runs goose migrations from an embedded FS.
type Migrator struct {
	provider *goose.Provider
	db       *sql.DB
}

// NewMigrator opens a database/sql connection for goose.
func NewMigrator(databaseURL string, migrations fs.FS) (*Migrator, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("goose provider: %w", err)
	}
	return &Migrator{provider: p, db: db}, nil
}

// Close releases the connection.
func (m *Migrator) Close() error { return m.db.Close() }

// Up applies all pending migrations.
func (m *Migrator) Up(ctx context.Context, log *slog.Logger) error {
	res, err := m.provider.Up(ctx)
	for _, r := range res {
		log.Info("migration applied", "version", r.Source.Version, "path", r.Source.Path, "duration_ms", r.Duration.Milliseconds())
	}
	return err
}

// Down rolls back the latest migration.
func (m *Migrator) Down(ctx context.Context, log *slog.Logger) error {
	r, err := m.provider.Down(ctx)
	if r != nil {
		log.Info("migration rolled back", "version", r.Source.Version, "path", r.Source.Path)
	}
	return err
}

// DownTo rolls back to version (0 = empty database).
func (m *Migrator) DownTo(ctx context.Context, version int64, log *slog.Logger) error {
	res, err := m.provider.DownTo(ctx, version)
	for _, r := range res {
		log.Info("migration rolled back", "version", r.Source.Version, "path", r.Source.Path)
	}
	return err
}

// Status prints the state of every migration.
func (m *Migrator) Status(ctx context.Context, log *slog.Logger) error {
	res, err := m.provider.Status(ctx)
	if err != nil {
		return err
	}
	for _, r := range res {
		log.Info("migration", "version", r.Source.Version, "path", r.Source.Path, "state", string(r.State))
	}
	return nil
}
