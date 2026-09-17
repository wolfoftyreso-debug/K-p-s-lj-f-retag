package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/db/migrations"
)

const (
	userA          = "00000000-0000-4000-8000-000000000001"
	userB          = "00000000-0000-4000-8000-000000000002"
	userOrg        = "00000000-0000-4000-8000-000000000003"
	userRead       = "00000000-0000-4000-8000-000000000004"
	userInactive   = "00000000-0000-4000-8000-000000000005"
	userOwner      = "00000000-0000-4000-8000-000000000006"
	workspaceA     = "10000000-0000-4000-8000-000000000001"
	workspaceB     = "10000000-0000-4000-8000-000000000002"
	workspaceOrg   = "10000000-0000-4000-8000-000000000003"
	workspaceOwner = "10000000-0000-4000-8000-000000000004"
	orgA           = "20000000-0000-4000-8000-000000000001"
	unknown        = "90000000-0000-4000-8000-000000000001"
)

type databaseHarness struct {
	admin       *pgx.Conn
	api, worker *Store
	processor   *Processor
	ctx         context.Context
}

func integrationHarness(t *testing.T) *databaseHarness {
	t.Helper()
	adminDSN, apiDSN, workerDSN := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_API_DATABASE_URL"), os.Getenv("TEST_WORKER_DATABASE_URL")
	if adminDSN == "" || apiDSN == "" || workerDSN == "" {
		if os.Getenv("REQUIRE_INTEGRATION") == "1" {
			t.Fatal("required real PostgreSQL DSNs TEST_DATABASE_URL, TEST_API_DATABASE_URL, TEST_WORKER_DATABASE_URL are not all configured")
		}
		t.Skip("real PostgreSQL integration not run: TEST_DATABASE_URL, TEST_API_DATABASE_URL, TEST_WORKER_DATABASE_URL are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	bootstrap, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal("cannot connect PostgreSQL test administrator")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := bootstrap.Close(cleanupCtx); err != nil {
			t.Errorf("close test bootstrap: %v", err)
		}
	})
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	dbName := "package_a_test_" + strings.ReplaceAll(id, "-", "")
	if _, err = bootstrap.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := bootstrap.Exec(cleanupCtx, "DROP DATABASE "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop isolated test database: %v", err)
		}
	})
	configure := func(dsn string) *pgx.ConnConfig {
		config, err := pgx.ParseConfig(dsn)
		if err != nil {
			t.Fatal("invalid test DSN")
		}
		config.Database = dbName
		return config
	}
	admin, err := pgx.ConnectConfig(ctx, configure(adminDSN))
	if err != nil {
		t.Fatal("connect isolated admin")
	}
	t.Cleanup(func() {
		if err := admin.Close(context.Background()); err != nil {
			t.Errorf("close admin: %v", err)
		}
	})
	if err := migrations.Apply(ctx, admin); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	apiDSNForDB := withDatabase(t, apiDSN, dbName)
	workerDSNForDB := withDatabase(t, workerDSN, dbName)
	api, err := Open(ctx, apiDSNForDB, "api")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(api.Close)
	worker, err := Open(ctx, workerDSNForDB, "worker")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(worker.Close)
	p, err := NewProcessor(worker, WorkerOptions{LeaseDuration: 10 * time.Second, RetryBase: time.Millisecond, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	return &databaseHarness{admin: admin, api: api, worker: worker, processor: p, ctx: ctx}
}

func withDatabase(t *testing.T, dsn, db string) string {
	t.Helper()
	_, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	quote := func(s string) string {
		return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal("invalid test DSN URL")
		}
		u.Path = "/" + db
		q := u.Query()
		q.Del("dbname")
		u.RawQuery = q.Encode()
		return u.String()
	}
	return fmt.Sprintf("%s dbname=%s", dsn, quote(db))
}

func (h *databaseHarness) reset(t *testing.T) {
	t.Helper()
	_, err := h.admin.Exec(h.ctx, `TRUNCATE eventing.consumer_receipts,eventing.workspace_revisions,eventing.outbox,audit.events,
organizations.workspace_permissions,organizations.workspace_memberships,organizations.workspaces,
organizations.organization_memberships,organizations.organizations,identity.users;
INSERT INTO identity.users(id,active) VALUES
('00000000-0000-4000-8000-000000000001',true),('00000000-0000-4000-8000-000000000002',true),
('00000000-0000-4000-8000-000000000003',true),('00000000-0000-4000-8000-000000000004',true),
('00000000-0000-4000-8000-000000000005',false),('00000000-0000-4000-8000-000000000006',true);
INSERT INTO organizations.organizations(id) VALUES('20000000-0000-4000-8000-000000000001');
INSERT INTO organizations.organization_memberships(organization_id,user_id) VALUES('20000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000003');
INSERT INTO organizations.workspaces(id,owner_kind,owner_user_id,owner_organization_id,name) VALUES
('10000000-0000-4000-8000-000000000001','PERSON','00000000-0000-4000-8000-000000000001',NULL,'Workspace A'),
('10000000-0000-4000-8000-000000000002','PERSON','00000000-0000-4000-8000-000000000002',NULL,'Workspace B'),
('10000000-0000-4000-8000-000000000003','ORGANIZATION',NULL,'20000000-0000-4000-8000-000000000001','Organization workspace'),
('10000000-0000-4000-8000-000000000004','PERSON','00000000-0000-4000-8000-000000000006',NULL,'Owner without membership');
INSERT INTO organizations.workspace_memberships(workspace_id,user_id) VALUES
('10000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000001'),
('10000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000002'),
('10000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000001'),
('10000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000004'),
('10000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000005');
INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission)
SELECT workspace_id,user_id,'workspace.read' FROM organizations.workspace_memberships;
INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission)
SELECT workspace_id,user_id,'workspace.update' FROM organizations.workspace_memberships WHERE user_id!='00000000-0000-4000-8000-000000000004';`)
	if err != nil {
		t.Fatal(err)
	}
}
func command(actor, workspace string, version int64) UpdateWorkspaceNameCommand {
	return UpdateWorkspaceNameCommand{ActorID: actor, WorkspaceID: workspace, Name: "Changed workspace", ExpectedVersion: version, RequestID: "30000000-0000-4000-8000-000000000001", CorrelationID: "30000000-0000-4000-8000-000000000002"}
}
func assertCount(t *testing.T, h *databaseHarness, query string, want int64, args ...any) {
	t.Helper()
	var got int64
	if err := h.admin.QueryRow(h.ctx, query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got count %d, want %d: %s", got, want, query)
	}
}

func TestPostgresPackageA(t *testing.T) {
	h := integrationHarness(t)
	t.Run("runtime_roles_reject_privilege_and_wrong_process", func(t *testing.T) {
		for _, test := range []struct{ dsn, role string }{{os.Getenv("TEST_DATABASE_URL"), "api"}, {os.Getenv("TEST_API_DATABASE_URL"), "worker"}, {os.Getenv("TEST_WORKER_DATABASE_URL"), "api"}} {
			s, err := Open(h.ctx, withDatabase(t, test.dsn, h.admin.Config().Database), test.role)
			if s != nil {
				s.Close()
			}
			if !errors.Is(err, ErrUnsafeRole) {
				t.Fatal("unsafe runtime login accepted", err)
			}
		}
		// A startup SET ROLE must not disguise an administrator as a capability
		// role: RESET ROLE would restore its privileged session identity.
		for _, parameter := range []struct{ key, value string }{{"options", "-c role=foundation_api"}, {"role", "foundation_api"}, {"session_authorization", "foundation_api"}} {
			dsn := withDatabase(t, os.Getenv("TEST_DATABASE_URL"), h.admin.Config().Database)
			if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
				u, err := url.Parse(dsn)
				if err != nil {
					t.Fatal("invalid test DSN")
				}
				q := u.Query()
				q.Set(parameter.key, parameter.value)
				// PostgreSQL/libpq URI values treat '+' literally, unlike HTML
				// form encoding. Use percent-encoded spaces in startup options.
				u.RawQuery = strings.ReplaceAll(q.Encode(), "+", "%20")
				dsn = u.String()
			} else {
				dsn += " " + parameter.key + "='" + parameter.value + "'"
			}
			s, err := Open(h.ctx, dsn, "api")
			if s != nil {
				s.Close()
			}
			if err == nil {
				t.Fatalf("startup masking %s accepted an unsafe login", parameter.key)
			}
			if !errors.Is(err, ErrUnsafeRole) {
				var pgError *pgconn.PgError
				if errors.As(err, &pgError) {
					t.Fatalf("startup masking %s rejected by PostgreSQL before role validation (SQLSTATE %s: %s); this does not prove the role guard", parameter.key, pgError.Code, pgError.Message)
				}
				t.Fatalf("startup masking %s unexpectedly returned %v", parameter.key, err)
			}
		}
		id, err := newID()
		if err != nil {
			t.Fatal(err)
		}
		suffix := strings.ReplaceAll(id, "-", "")
		login, privileged := "unsafe_login_"+suffix, "unsafe_group_"+suffix
		_, err = h.admin.Exec(h.ctx, `CREATE ROLE `+pgx.Identifier{privileged}.Sanitize()+` NOLOGIN BYPASSRLS;
CREATE ROLE `+pgx.Identifier{login}.Sanitize()+` LOGIN INHERIT PASSWORD '`+id+`';
GRANT foundation_api,`+pgx.Identifier{privileged}.Sanitize()+` TO `+pgx.Identifier{login}.Sanitize())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := h.admin.Exec(h.ctx, `DROP ROLE `+pgx.Identifier{login}.Sanitize()+`; DROP ROLE `+pgx.Identifier{privileged}.Sanitize()); err != nil {
				t.Error(err)
			}
		})
		dsn := withDatabase(t, os.Getenv("TEST_DATABASE_URL"), h.admin.Config().Database)
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatal("invalid test DSN")
			}
			u.User = url.UserPassword(login, id)
			dsn = u.String()
		} else {
			dsn += " user='" + login + "' password='" + id + "'"
		}
		s, err := Open(h.ctx, dsn, "api")
		if s != nil {
			s.Close()
		}
		if !errors.Is(err, ErrUnsafeRole) {
			t.Fatal("inherited privileged role accepted", err)
		}
	})
	t.Run("ownership_constraints", func(t *testing.T) {
		h.reset(t)
		for _, q := range []string{
			`INSERT INTO organizations.workspaces(id,owner_kind,name) VALUES('90000000-0000-4000-8000-000000000001','PERSON','Bad')`,
			`INSERT INTO organizations.workspaces(id,owner_kind,owner_user_id,owner_organization_id,name) VALUES('90000000-0000-4000-8000-000000000001','PERSON','00000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001','Bad')`,
			`INSERT INTO organizations.workspaces(id,owner_kind,owner_user_id,name) VALUES('90000000-0000-4000-8000-000000000001','PERSON','90000000-0000-4000-8000-000000000001','Bad')`,
			`INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission) VALUES('10000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','workspace.update')`,
		} {
			if _, err := h.admin.Exec(h.ctx, q); err == nil {
				t.Fatalf("invariant accepted: %s", q)
			}
		}
	})
	t.Run("isolation_and_explicit_permissions", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.ReadWorkspace(h.ctx, userA, workspaceA); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.ReadWorkspace(h.ctx, userA, workspaceOrg); err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct{ actor, workspace string }{{userA, workspaceB}, {userA, unknown}, {userA, "not-a-uuid"}, {userOrg, workspaceOrg}, {userOwner, workspaceOwner}, {userInactive, workspaceA}, {unknown, workspaceA}, {userB, workspaceA}} {
			if _, err := h.api.ReadWorkspace(h.ctx, test.actor, test.workspace); !errors.Is(err, ErrNotFound) {
				t.Errorf("read did not uniformly deny %+v: %v", test, err)
			}
			if _, err := h.api.UpdateWorkspaceName(h.ctx, command(test.actor, test.workspace, 1)); !errors.Is(err, ErrNotFound) {
				t.Errorf("update did not uniformly deny %+v: %v", test, err)
			}
		}
		if _, err := h.api.ReadWorkspace(h.ctx, userRead, workspaceA); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userRead, workspaceA, 1)); !errors.Is(err, ErrNotFound) {
			t.Fatal("read permission elevated to update", err)
		}
		assertCount(t, h, `SELECT count(*) FROM audit.events`, 0)
	})
	t.Run("revocation_and_pool_context", func(t *testing.T) {
		h.reset(t)
		for i := 0; i < 16; i++ {
			if _, err := h.api.ReadWorkspace(h.ctx, userA, workspaceA); err != nil {
				t.Fatal(err)
			}
			if _, err := h.api.ReadWorkspace(h.ctx, userA, workspaceB); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
		}
		var count int64
		if err := h.api.pool.QueryRow(h.ctx, `SELECT count(*) FROM organizations.workspaces`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("tenant context escaped transaction: %d %v", count, err)
		}
		if _, err := h.admin.Exec(h.ctx, `UPDATE organizations.workspace_memberships SET active=false WHERE workspace_id=$1 AND user_id=$2`, workspaceA, userA); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.ReadWorkspace(h.ctx, userA, workspaceA); !errors.Is(err, ErrNotFound) {
			t.Fatal("revocation ignored", err)
		}
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); !errors.Is(err, ErrNotFound) {
			t.Fatal("revoked mutation allowed", err)
		}
	})
	t.Run("direct_sql_cannot_change_scope_or_audit", func(t *testing.T) {
		h.reset(t)
		tx, err := h.api.protectedTX(h.ctx, userA, workspaceA, "workspace.update")
		if err != nil {
			t.Fatal(err)
		}
		result, err := tx.Exec(h.ctx, `UPDATE organizations.workspaces SET name='Attack' WHERE id=$1`, workspaceB)
		if err != nil || result.RowsAffected() != 0 {
			t.Fatal("RLS cross tenant update", err)
		}
		if err := rollback(tx); err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{`UPDATE audit.events SET result='SUCCESS'`, `DELETE FROM audit.events`, `SELECT * FROM identity.users`, `UPDATE organizations.workspace_memberships SET active=true`} {
			if _, err := h.api.pool.Exec(h.ctx, q); err == nil {
				t.Fatal("runtime unexpectedly permitted", q)
			}
		}
		if _, err := h.worker.pool.Exec(h.ctx, `SELECT * FROM organizations.workspaces`); err == nil {
			t.Fatal("worker can read workspace names")
		}
		for _, q := range []string{`UPDATE eventing.outbox SET payload='{}'`, `UPDATE eventing.outbox SET aggregate_version=10`, `UPDATE eventing.outbox SET workspace_id='10000000-0000-4000-8000-000000000002'`} {
			if _, err := h.worker.pool.Exec(h.ctx, q); err == nil {
				t.Fatal("worker can rewrite immutable envelope", q)
			}
		}
	})
	t.Run("permission_revocation_does_not_fall_back_to_owner", func(t *testing.T) {
		h.reset(t)
		if _, err := h.admin.Exec(h.ctx, `DELETE FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2 AND permission='workspace.update'`, workspaceA, userA); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.ReadWorkspace(h.ctx, userA, workspaceA); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); !errors.Is(err, ErrNotFound) {
			t.Fatal("owner bypassed revoked update permission", err)
		}
		if _, err := h.admin.Exec(h.ctx, `DELETE FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2`, workspaceA, userA); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.ReadWorkspace(h.ctx, userA, workspaceA); !errors.Is(err, ErrNotFound) {
			t.Fatal("owner bypassed revoked read permission", err)
		}
	})
	t.Run("atomic_success_conflict_and_scope_fk", func(t *testing.T) {
		h.reset(t)
		w, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1))
		if err != nil {
			t.Fatal(err)
		}
		if w.Version != 2 || w.Name != "Changed workspace" || w.UpdatedAt.Location() != time.UTC {
			t.Fatal(w)
		}
		assertCount(t, h, `SELECT count(*) FROM audit.events WHERE workspace_id=$1 AND actor_user_id=$2 AND metadata='{"previous_version":1,"version":2}'::jsonb`, 1, workspaceA, userA)
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE payload='{}'::jsonb AND aggregate_version=2`, 1)
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); !errors.Is(err, ErrConflict) {
			t.Fatal("stale write", err)
		}
		assertCount(t, h, `SELECT count(*) FROM audit.events`, 1)
		if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET workspace_id=$1`, workspaceB); err == nil {
			t.Fatal("cross workspace audit/event link accepted")
		}
	})
	t.Run("audit_failure_rolls_back_mutation", func(t *testing.T) {
		h.reset(t)
		injectFailure(t, h, "audit.events", "audit_failure")
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); !errors.Is(err, ErrUnavailable) {
			t.Fatal("expected audit failure", err)
		}
		assertCount(t, h, `SELECT version FROM organizations.workspaces WHERE id=$1`, 1, workspaceA)
		assertCount(t, h, `SELECT count(*) FROM audit.events`, 0)
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox`, 0)
	})
	t.Run("outbox_failure_rolls_back_audit_and_mutation", func(t *testing.T) {
		h.reset(t)
		injectFailure(t, h, "eventing.outbox", "outbox_failure")
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); !errors.Is(err, ErrUnavailable) {
			t.Fatal("expected outbox failure", err)
		}
		assertCount(t, h, `SELECT version FROM organizations.workspaces WHERE id=$1`, 1, workspaceA)
		assertCount(t, h, `SELECT count(*) FROM audit.events`, 0)
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox`, 0)
	})
	t.Run("concurrent_optimistic_writes", func(t *testing.T) {
		h.reset(t)
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1))
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		succeeded, conflicted := 0, 0
		for err := range results {
			if err == nil {
				succeeded++
			} else if errors.Is(err, ErrConflict) {
				conflicted++
			} else {
				t.Fatal(err)
			}
		}
		if succeeded != 1 || conflicted != 1 {
			t.Fatalf("success=%d conflict=%d", succeeded, conflicted)
		}
		assertCount(t, h, `SELECT count(*) FROM audit.events`, 1)
	})
	t.Run("duplicate_and_crash_after_effect", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); err != nil {
			t.Fatal(err)
		}
		d, found, err := h.processor.claim(h.ctx)
		if err != nil || !found {
			t.Fatal(found, err)
		}
		if err := h.processor.consume(h.ctx, d); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- h.processor.consume(h.ctx, d) }()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}
		// Simulate process death after its effect committed but before acknowledgement.
		if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET leased_until=clock_timestamp()-interval '1 second' WHERE id=$1`, d.ID); err != nil {
			t.Fatal(err)
		}
		if did, err := h.processor.ProcessNext(h.ctx); err != nil || !did {
			t.Fatal(did, err)
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.consumer_receipts`, 1)
		assertCount(t, h, `SELECT version FROM eventing.workspace_revisions WHERE workspace_id=$1`, 2, workspaceA)
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status='DELIVERED' AND attempts=2`, 1)
		if err := h.processor.acknowledge(h.ctx, d); !errors.Is(err, ErrLeaseLost) {
			t.Fatal("stale lease acknowledged", err)
		}
	})
	t.Run("reordered_versions_do_not_regress_projection", func(t *testing.T) {
		h.reset(t)
		for v := int64(1); v <= 2; v++ {
			if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, v)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET available_at=CASE WHEN aggregate_version=2 THEN clock_timestamp()+interval '1 hour' ELSE clock_timestamp() END`); err != nil {
			t.Fatal(err)
		}
		if _, err := h.processor.ProcessNext(h.ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET available_at=clock_timestamp() WHERE aggregate_version=2`); err != nil {
			t.Fatal(err)
		}
		if _, err := h.processor.ProcessNext(h.ctx); err != nil {
			t.Fatal(err)
		}
		assertCount(t, h, `SELECT version FROM eventing.workspace_revisions WHERE workspace_id=$1`, 3, workspaceA)
		assertCount(t, h, `SELECT count(*) FROM eventing.consumer_receipts`, 2)
	})
	t.Run("last_attempt_lost_ack_recovers_completed_receipt", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); err != nil {
			t.Fatal(err)
		}
		d, found, err := h.processor.claim(h.ctx)
		if err != nil || !found {
			t.Fatal(found, err)
		}
		if err := h.processor.consume(h.ctx, d); err != nil {
			t.Fatal(err)
		}
		if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET attempts=3,leased_until=clock_timestamp()-interval '1 second' WHERE id=$1`, d.ID); err != nil {
			t.Fatal(err)
		}
		if did, err := h.processor.ProcessNext(h.ctx); did || err != nil {
			t.Fatal(did, err)
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status='DELIVERED' AND attempts=3 AND last_error_code IS NULL`, 1)
		assertCount(t, h, `SELECT count(*) FROM eventing.consumer_receipts`, 1)
	})
	for _, scenario := range []struct {
		name        string
		committed   bool
		finalStatus string
	}{
		{"final_attempt_lost_commit_response_recovers_delivered", true, "DELIVERED"},
		{"final_attempt_uncommitted_uncertainty_exhausts_dead", false, "DEAD"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			h.reset(t)
			if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); err != nil {
				t.Fatal(err)
			}
			// The next claim consumes the last permitted attempt.
			if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET attempts=$1`, h.processor.options.MaxAttempts-1); err != nil {
				t.Fatal(err)
			}
			processor, err := NewProcessor(h.worker, h.processor.options)
			if err != nil {
				t.Fatal(err)
			}
			processor.commitConsumer = func(ctx context.Context, tx pgx.Tx) error {
				if scenario.committed {
					if err := tx.Commit(ctx); err != nil {
						return err
					}
				}
				// Fault injection at the commit boundary, not a substitute for
				// persistence: the success case commits real PostgreSQL state;
				// the other case lets consume's cleanup roll the transaction back.
				return io.ErrUnexpectedEOF
			}
			worked, err := processor.ProcessNext(h.ctx)
			if !worked || !errors.Is(err, ErrCommitUncertain) || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("expected classified commit uncertainty: worked=%v error=%v", worked, err)
			}
			assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status='PROCESSING' AND attempts=$1 AND lease_token IS NOT NULL AND last_error_code IS NULL`, 1, h.processor.options.MaxAttempts)
			var effects int64
			if scenario.committed {
				effects = 1
			}
			assertCount(t, h, `SELECT count(*) FROM eventing.consumer_receipts`, effects)
			assertCount(t, h, `SELECT count(*) FROM eventing.workspace_revisions WHERE workspace_id=$1 AND version=2`, effects, workspaceA)
			if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET leased_until=clock_timestamp()-interval '1 second'`); err != nil {
				t.Fatal(err)
			}
			if worked, err := h.processor.ProcessNext(h.ctx); worked || err != nil {
				t.Fatalf("recovery must reconcile the exhausted lease: worked=%v error=%v", worked, err)
			}
			assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status=$1 AND attempts=$2 AND lease_token IS NULL`, 1, scenario.finalStatus, h.processor.options.MaxAttempts)
			assertCount(t, h, `SELECT count(*) FROM eventing.consumer_receipts`, effects)
			assertCount(t, h, `SELECT count(*) FROM eventing.workspace_revisions`, effects)
		})
	}
	t.Run("poison_event_does_not_block_healthy_work", func(t *testing.T) {
		h.reset(t)
		for v := int64(1); v <= 2; v++ {
			if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, v)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET payload='{"unexpected":true}' WHERE aggregate_version=2`); err != nil {
			t.Fatal(err)
		}
		if did, err := h.processor.ProcessNext(h.ctx); !did || !errors.Is(err, ErrInvalidEvent) {
			t.Fatal(did, err)
		}
		if did, err := h.processor.ProcessNext(h.ctx); !did || err != nil {
			t.Fatal(did, err)
		}
		q, err := h.worker.QueueStats(h.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if q.Dead != 1 || q.Delivered != 1 {
			t.Fatal(q)
		}
	})
	t.Run("projection_failure_retries_atomically", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); err != nil {
			t.Fatal(err)
		}
		remove := injectFailure(t, h, "eventing.workspace_revisions", "projection_failure")
		if _, err := h.processor.ProcessNext(h.ctx); !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.consumer_receipts`, 0)
		assertCount(t, h, `SELECT count(*) FROM eventing.workspace_revisions`, 0)
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status='PENDING' AND attempts=1 AND last_error_code='processing_failed'`, 1)
		remove()
		if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET available_at=clock_timestamp()`); err != nil {
			t.Fatal(err)
		}
		if did, err := h.processor.ProcessNext(h.ctx); !did || err != nil {
			t.Fatal(did, err)
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.consumer_receipts`, 1)
	})
	t.Run("retry_exhaustion_and_crashed_lease_exhaustion", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); err != nil {
			t.Fatal(err)
		}
		injectFailure(t, h, "eventing.workspace_revisions", "exhaustion_failure")
		for i := 0; i < 3; i++ {
			if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET available_at=clock_timestamp()`); err != nil {
				t.Fatal(err)
			}
			if _, err := h.processor.ProcessNext(h.ctx); !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status='DEAD' AND attempts=3`, 1)
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 2)); err != nil {
			t.Fatal(err)
		}
		if _, err := h.admin.Exec(h.ctx, `UPDATE eventing.outbox SET status='PROCESSING',attempts=3,lease_token=$1,leased_until=clock_timestamp()-interval '1 second' WHERE aggregate_version=3`, unknown); err != nil {
			t.Fatal(err)
		}
		if did, err := h.processor.ProcessNext(h.ctx); did || err != nil {
			t.Fatal(did, err)
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status='DEAD' AND last_error_code='lease_expired'`, 1)
	})
	t.Run("reduced_retry_budget_does_not_strand_pending_event", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); err != nil {
			t.Fatal(err)
		}
		remove := injectFailure(t, h, "eventing.workspace_revisions", "reduced_budget_failure")
		if _, err := h.processor.ProcessNext(h.ctx); !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
		remove()
		options := h.processor.options
		options.MaxAttempts = 1
		stricter, err := NewProcessor(h.worker, options)
		if err != nil {
			t.Fatal(err)
		}
		if did, err := stricter.ProcessNext(h.ctx); did || err != nil {
			t.Fatal(did, err)
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status='DEAD' AND attempts=1 AND last_error_code='processing_failed'`, 1)
	})
	t.Run("cancellation_and_connection_reuse", func(t *testing.T) {
		h.reset(t)
		ctx, cancel := context.WithCancel(h.ctx)
		cancel()
		if _, err := h.api.ReadWorkspace(ctx, userA, workspaceA); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled read", err)
		}
		if _, err := h.api.UpdateWorkspaceName(ctx, command(userA, workspaceA, 1)); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled update", err)
		}
		if _, err := h.processor.ProcessNext(ctx); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled worker", err)
		}
		if _, err := h.api.ReadWorkspace(h.ctx, userA, workspaceA); err != nil {
			t.Fatal("pool unusable after cancellation", err)
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox`, 0)
	})
	t.Run("cancellation_during_transaction_rolls_back", func(t *testing.T) {
		h.reset(t)
		_, err := h.admin.Exec(h.ctx, `CREATE FUNCTION public.cancel_delay() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(5); RETURN NEW; END $$;
CREATE TRIGGER cancel_delay BEFORE INSERT ON eventing.outbox FOR EACH ROW EXECUTE FUNCTION public.cancel_delay()`)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := h.admin.Exec(h.ctx, `DROP TRIGGER cancel_delay ON eventing.outbox; DROP FUNCTION public.cancel_delay()`); err != nil {
				t.Error(err)
			}
		})
		ctx, cancel := context.WithTimeout(h.ctx, 100*time.Millisecond)
		defer cancel()
		if _, err := h.api.UpdateWorkspaceName(ctx, command(userA, workspaceA, 1)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("expected in-flight deadline", err)
		}
		assertCount(t, h, `SELECT version FROM organizations.workspaces WHERE id=$1`, 1, workspaceA)
		assertCount(t, h, `SELECT count(*) FROM audit.events`, 0)
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox`, 0)
	})
	t.Run("concurrent_claims_have_one_effect", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.UpdateWorkspaceName(h.ctx, command(userA, workspaceA, 1)); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := h.processor.ProcessNext(h.ctx); results <- err }()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}
		assertCount(t, h, `SELECT count(*) FROM eventing.consumer_receipts`, 1)
		assertCount(t, h, `SELECT count(*) FROM eventing.workspace_revisions`, 1)
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox WHERE status='DELIVERED' AND attempts=1`, 1)
	})
}

func injectFailure(t *testing.T, h *databaseHarness, table, name string) func() {
	t.Helper()
	// All identifiers are constant test code and the database is disposable.
	_, err := h.admin.Exec(h.ctx, `CREATE FUNCTION public.`+name+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$;
CREATE TRIGGER `+name+` BEFORE INSERT ON `+table+` FOR EACH ROW EXECUTE FUNCTION public.`+name+`()`)
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	remove := func() {
		if removed {
			return
		}
		removed = true
		if _, err := h.admin.Exec(h.ctx, `DROP TRIGGER `+name+` ON `+table+`; DROP FUNCTION public.`+name+`() `); err != nil {
			t.Errorf("remove test failure: %v", err)
		}
	}
	t.Cleanup(remove)
	return remove
}
