-- Disposable synthetic integration database only. Run after migrations.
-- Authentication is configured by the isolated test PostgreSQL instance.
\set ON_ERROR_STOP on
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'packagea_test_api') THEN
    CREATE ROLE packagea_test_api LOGIN INHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'packagea_test_worker') THEN
    CREATE ROLE packagea_test_worker LOGIN INHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  END IF;
END $$;
GRANT foundation_api TO packagea_test_api;
GRANT foundation_worker TO packagea_test_worker;
