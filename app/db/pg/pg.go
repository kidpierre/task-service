package pg

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"task-service/config"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func New(ctx context.Context, cfg config.Postgres) (*sql.DB, error) {
	connCfg, err := pgx.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}

	db := stdlib.OpenDB(*connCfg)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := runMigrations(connCfg); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

func runMigrations(connCfg *pgx.ConnConfig) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}

	db := stdlib.OpenDB(*connCfg)
	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{})
	if err != nil {
		_ = db.Close()
		return err
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		_ = driver.Close()
		return err
	}

	upErr := m.Up()
	srcErr, dbErr := m.Close()
	if upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		return upErr
	}
	return errors.Join(srcErr, dbErr)
}
