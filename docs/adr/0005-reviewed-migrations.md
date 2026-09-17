# ADR 0005 — Checksummed transactional SQL migrations

Date: 2026-09-17. Status: accepted reversible engineering choice within Package A.

## Decision

Embed reviewed, ordered SQL migrations in the administrative migration executable. Filenames use contiguous six-digit versions. Record SHA-256 of the exact SQL bytes, filename, version and application time. A dedicated session advisory lock serializes migration runs within a database. Reject unknown history, gaps and altered applied files instead of silently repairing history.

Each migration's DDL and history entry are one PostgreSQL transaction. Failed migration changes roll back. Repeat application is a validated no-op. There is no automatic down migration or schema mutation on API/worker startup. A later schema change is a new migration.

The initial migration creates the Package A schemas and NOLOGIN capability roles. Its administrative identity needs schema creation and role-bootstrap authority, or pre-provisioned roles. Runtime LOGIN identities are provisioned separately and are never schema owners. Local integration identities and data are disposable fixtures, not deployment credentials.

## Consequences

Initial environment bootstrap must apply migrations before runtime startup. Runtime readiness validates an actual required relation, not only a socket. A failed rollout can return to a compatible binary; destructive schema/data recovery requires a deliberate restore plan. Exact-byte checksums mean migrations must retain LF line endings across platforms.

A small embedded runner avoids a heavy migration framework without hiding SQL. Real PostgreSQL tests cover repeat application, changed history and DDL/history rollback. A migration dry-run claim is not made: PostgreSQL transaction tests execute actual SQL.
