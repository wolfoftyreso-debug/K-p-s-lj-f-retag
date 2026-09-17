// migrate is a deliberate administrative process, never part of API startup.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/db/migrations"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/config"
)

func main() { os.Exit(run()) }

func run() (status int) {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if len(os.Args) != 2 || os.Args[1] != "apply" {
		log.Error("usage: migrate apply", "code", "invalid_command")
		return 2
	}
	dsn := os.Getenv("MIGRATION_DATABASE_URL")
	if dsn == "" {
		log.Error("MIGRATION_DATABASE_URL is required", "code", "invalid_configuration")
		return 2
	}
	if err := config.ValidateDatabaseURL(dsn); err != nil {
		log.Error("invalid migration connection configuration", "code", "invalid_configuration")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Error("migration database connection failed", "code", "database_unavailable")
		return 1
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := conn.Close(cleanup); err != nil {
			log.Error("migration connection close failed", "code", "database_close_failed")
			status = 1
		}
	}()
	if err := migrations.Apply(ctx, conn); err != nil {
		state := "none"
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) {
			state = databaseError.Code
		}
		log.Error("migration failed; verify durable migration history before retry", "code", "migration_failed", "sql_state", state)
		return 1
	}
	log.Info("migrations applied", "code", "migration_complete")
	return 0
}
