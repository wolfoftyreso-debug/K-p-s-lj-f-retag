package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
)

func migrationDatabase(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_INTEGRATION") == "1" {
			t.Fatal("TEST_DATABASE_URL is required")
		}
		t.Skip("real PostgreSQL integration: TEST_DATABASE_URL not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect to integration PostgreSQL")
	}
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "packagea_migrations_" + hex.EncodeToString(suffix[:])
	// The generated identifier is quoted by pgx; no caller SQL is accepted.
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid integration configuration")
	}
	config.Database = name
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("cannot connect to isolated migration database")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := conn.Close(cleanup); err != nil {
			t.Error(err)
		}
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Error(err)
		}
		if err := admin.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	return conn
}

func TestIntegrationMigrationApplyRepeatAndDrift(t *testing.T) {
	conn := migrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := Apply(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, conn); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM foundation_schema.migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("history count=%d, want 1", count)
	}
	if _, err := conn.Exec(ctx, "UPDATE foundation_schema.migrations SET checksum=repeat('0',64) WHERE version=1"); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, conn); err == nil {
		t.Fatal("modified migration history accepted")
	}
}

func TestIntegrationFailedMigrationRollsBackSQLAndHistory(t *testing.T) {
	conn := migrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	files := fstest.MapFS{
		"000001_first.sql":   {Data: []byte("CREATE TABLE public.first_proof(id integer PRIMARY KEY);")},
		"000002_failure.sql": {Data: []byte("CREATE TABLE public.second_proof(id integer PRIMARY KEY); SELECT 1/0;")},
	}
	items, err := catalogue(files)
	if err != nil {
		t.Fatal(err)
	}
	if err = apply(ctx, conn, items); err == nil {
		t.Fatal("expected migration failure")
	}
	var second *string
	if err = conn.QueryRow(ctx, "SELECT to_regclass('public.second_proof')::text").Scan(&second); err != nil {
		t.Fatal(err)
	}
	if second != nil {
		t.Fatal("failed migration left a table")
	}
	var count int
	if err = conn.QueryRow(ctx, "SELECT count(*) FROM foundation_schema.migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("failed migration left history: %d", count)
	}
	files["000002_failure.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE public.second_proof(id integer PRIMARY KEY);")}
	items, err = catalogue(files)
	if err != nil {
		t.Fatal(err)
	}
	if err = apply(ctx, conn, items); err != nil {
		t.Fatalf("retry after failed migration: %v", err)
	}
}
