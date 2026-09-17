// Package migrations applies the repository's reviewed, immutable SQL migrations.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

//go:embed *.sql
var source embed.FS

const advisoryLock int64 = 487092816301

type migration struct {
	version  int64
	name     string
	sql      string
	checksum string
}

func catalogue(files fs.FS) ([]migration, error) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	if len(names) == 0 {
		return nil, errors.New("migration catalogue is empty")
	}
	sort.Strings(names)
	pattern := regexp.MustCompile(`^([0-9]{6})_[a-z0-9_]+\.sql$`)
	result := make([]migration, 0, len(names))
	for i, name := range names {
		match := pattern.FindStringSubmatch(name)
		if match == nil {
			return nil, fmt.Errorf("invalid migration filename: %s", name)
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || version != int64(i+1) {
			return nil, errors.New("migration versions must be contiguous from 000001")
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("read migration: %w", err)
		}
		if len(body) == 0 {
			return nil, fmt.Errorf("empty migration: %s", name)
		}
		sum := sha256.Sum256(body)
		result = append(result, migration{version, name, string(body), hex.EncodeToString(sum[:])})
	}
	return result, nil
}

// Apply takes a dedicated migration connection. Never call it with a runtime
// identity or a shared pooled connection. Applied SQL files cannot be changed.
// Every migration and its history entry commit in the same transaction.
func Apply(ctx context.Context, conn *pgx.Conn) (result error) {
	items, err := catalogue(source)
	if err != nil {
		return err
	}
	return apply(ctx, conn, items)
}

func apply(ctx context.Context, conn *pgx.Conn, items []migration) (result error) {
	var err error
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLock); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var unlocked bool
		err := conn.QueryRow(cleanup, "SELECT pg_advisory_unlock($1)", advisoryLock).Scan(&unlocked)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("release migration lock: %w", err))
		} else if !unlocked {
			result = errors.Join(result, errors.New("migration lock was not held"))
		}
	}()
	if _, err = conn.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS foundation_schema;
		CREATE TABLE IF NOT EXISTS foundation_schema.migrations (
		version bigint PRIMARY KEY CHECK (version > 0),
		filename text NOT NULL UNIQUE,
		checksum text NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
		applied_at timestamptz NOT NULL DEFAULT clock_timestamp());
		REVOKE ALL ON SCHEMA foundation_schema FROM PUBLIC;
		REVOKE ALL ON foundation_schema.migrations FROM PUBLIC;`); err != nil {
		return fmt.Errorf("initialize migration history: %w", err)
	}
	rows, err := conn.Query(ctx, "SELECT version, filename, checksum FROM foundation_schema.migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("read migration history: %w", err)
	}
	applied := 0
	for rows.Next() {
		var version int64
		var name, sum string
		if err = rows.Scan(&version, &name, &sum); err != nil {
			rows.Close()
			return fmt.Errorf("scan migration history: %w", err)
		}
		if applied >= len(items) || version != items[applied].version || name != items[applied].name || sum != items[applied].checksum {
			rows.Close()
			return errors.New("migration history mismatch: unknown, noncontiguous or modified migration")
		}
		applied++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("read migration history rows: %w", err)
	}
	for _, item := range items[applied:] {
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, item.sql); err != nil {
				return fmt.Errorf("apply %s: %w", item.name, err)
			}
			_, err := tx.Exec(ctx, "INSERT INTO foundation_schema.migrations(version,filename,checksum) VALUES($1,$2,$3)", item.version, item.name, item.checksum)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration transaction: %w", err)
		}
	}
	return nil
}
