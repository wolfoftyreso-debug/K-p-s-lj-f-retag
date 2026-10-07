package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestIntegrationDraftUpgradePreservesIdentityAndWorkspaceHistory(t *testing.T) {
	conn := migrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	items, err := catalogue(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = apply(ctx, conn, items[:2]); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO identity.users(id) VALUES('00000000-0000-4000-8000-000000000001');
INSERT INTO organizations.workspaces(id,owner_kind,owner_user_id,name) VALUES('10000000-0000-4000-8000-000000000001','PERSON','00000000-0000-4000-8000-000000000001','Existing workspace');
INSERT INTO organizations.workspace_memberships(workspace_id,user_id) VALUES('10000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000001');
INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission) VALUES('10000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000001','workspace.read');`)
	if err != nil {
		t.Fatal(err)
	}
	var before1, before2 string
	if err = conn.QueryRow(ctx, `SELECT (SELECT checksum FROM foundation_schema.migrations WHERE version=1),(SELECT checksum FROM foundation_schema.migrations WHERE version=2)`).Scan(&before1, &before2); err != nil {
		t.Fatal(err)
	}
	if err = Apply(ctx, conn); err != nil {
		t.Fatal(err)
	}
	var after1, after2, name string
	var grants, assignments, businesses int
	if err = conn.QueryRow(ctx, `SELECT (SELECT checksum FROM foundation_schema.migrations WHERE version=1),(SELECT checksum FROM foundation_schema.migrations WHERE version=2),w.name,(SELECT count(*) FROM organizations.workspace_permissions),(SELECT count(*) FROM organizations.personal_drafting_assignments),(SELECT count(*) FROM listings.businesses) FROM organizations.workspaces w`).Scan(&after1, &after2, &name, &grants, &assignments, &businesses); err != nil {
		t.Fatal(err)
	}
	if before1 != after1 || before2 != after2 || name != "Existing workspace" || grants != 1 || assignments != 0 || businesses != 0 {
		t.Fatal("draft upgrade changed existing state or implicitly granted drafting")
	}
	if before1 != "7875c3fb0dbd59222d80237df65037bbd82a9577162f7c539a7a7f2aedbe1b8e" || before2 != "b568cbadcda7c16ed3ec6c5fc45840b39216803302ca51f5513088f8adc40ec3" {
		t.Fatal("historical migration bytes changed")
	}
	if err = Apply(ctx, conn); err != nil {
		t.Fatal("repeat draft migration", err)
	}
}

func TestIntegrationDraftHelpersWithNonSuperuserMigrationOwner(t *testing.T) {
	conn := migrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	owner := "draft_migration_owner_" + hex.EncodeToString(suffix[:])
	// Capability roles are provisioned by the test administrator; migration
	// source may then run as an ordinary owning identity without CREATEROLE.
	_, err := conn.Exec(ctx, `DO $$ BEGIN
IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='foundation_api') THEN CREATE ROLE foundation_api NOLOGIN; END IF;
IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='foundation_worker') THEN CREATE ROLE foundation_worker NOLOGIN; END IF;
END $$; CREATE ROLE `+pgx.Identifier{owner}.Sanitize()+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
ALTER DATABASE `+pgx.Identifier{conn.Config().Database}.Sanitize()+` OWNER TO `+pgx.Identifier{owner}.Sanitize())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanup, `RESET ROLE; REASSIGN OWNED BY `+pgx.Identifier{owner}.Sanitize()+` TO `+pgx.Identifier{conn.Config().User}.Sanitize()+`; DROP OWNED BY `+pgx.Identifier{owner}.Sanitize()+`; DROP ROLE `+pgx.Identifier{owner}.Sanitize()); err != nil {
			t.Error(err)
		}
	})
	if _, err = conn.Exec(ctx, `SET ROLE `+pgx.Identifier{owner}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err = Apply(ctx, conn); err != nil {
		t.Fatal("non-superuser migration", err)
	}
	if _, err = conn.Exec(ctx, `RESET ROLE; INSERT INTO identity.users(id) VALUES('00000000-0000-4000-8000-000000000001');
INSERT INTO identity.external_identities(issuer,subject,provider,user_id) VALUES('https://synthetic.invalid','synthetic-user','synthetic','00000000-0000-4000-8000-000000000001');
INSERT INTO identity.sessions(id,token_hash,user_id,issuer,subject,security_version,authenticated_at,created_at,last_accepted_at,absolute_expires_at,assurance_level,assurance_evidence)
SELECT '70000000-0000-4000-8000-000000000001',sha256(convert_to('synthetic-session','UTF8')),'00000000-0000-4000-8000-000000000001','https://synthetic.invalid','synthetic-user',1,n.at,n.at,n.at,n.at+interval '8 hours','UNKNOWN','' FROM (SELECT clock_timestamp() AS at) n;
SET ROLE foundation_api;`); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && err != pgx.ErrTxClosed {
			t.Error(err)
		}
	}()
	var workspace string
	if err = tx.QueryRow(ctx, `SELECT organizations.personal_drafting_workspace(sha256(convert_to('synthetic-session','UTF8')),'30000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002')::text`).Scan(&workspace); err != nil {
		t.Fatal("non-superuser helper bootstrap", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO listings.businesses(id,workspace_id) VALUES('50000000-0000-4000-8000-000000000001',$1)`, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO listings.drafts(id,workspace_id,business_id) VALUES('60000000-0000-4000-8000-000000000001',$1,'50000000-0000-4000-8000-000000000001')`, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit.aggregate_events(workspace_id,actor_user_id,actor_session_id,action,target_kind,business_id,request_id,correlation_id,previous_version,resource_version,metadata) VALUES($1,'00000000-0000-4000-8000-000000000001','70000000-0000-4000-8000-000000000001','business.created','Business','50000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002',0,1,'{"previous_version":0,"version":1}');`, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit.aggregate_events(workspace_id,actor_user_id,actor_session_id,action,target_kind,listing_id,request_id,correlation_id,previous_version,resource_version,metadata) VALUES($1,'00000000-0000-4000-8000-000000000001','70000000-0000-4000-8000-000000000001','listing_draft.created','ListingDraft','60000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002',0,1,'{"previous_version":0,"version":1}');`, workspace); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal("non-superuser helper deferred audit", err)
	}
	if _, err = conn.Exec(ctx, `RESET ROLE`); err != nil {
		t.Fatal(err)
	}
	var unsafe bool
	if err = conn.QueryRow(ctx, `SELECT r.rolsuper OR r.rolbypassrls FROM pg_proc p JOIN pg_roles r ON r.oid=p.proowner WHERE p.oid='organizations.personal_drafting_workspace(bytea,uuid,uuid)'::regprocedure`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("helper silently depends on elevated owner", err)
	}
}
