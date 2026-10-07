package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/listings"
)

func draftCommand(t *testing.T, workspace string) listings.CreateDraftCommand {
	t.Helper()
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	return listings.CreateDraftCommand{WorkspaceID: workspace, CommandID: id,
		RequestID: "30000000-0000-4000-8000-000000000001", CorrelationID: "30000000-0000-4000-8000-000000000002"}
}

func grantDrafts(t *testing.T, h *databaseHarness, user, workspace string, permissions ...string) {
	t.Helper()
	for _, permission := range permissions {
		if _, err := h.admin.Exec(h.ctx, `INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, workspace, user, permission); err != nil {
			t.Fatal(err)
		}
	}
}

func secondSession(t *testing.T, h *databaseHarness, user string) []byte {
	t.Helper()
	digest := sha256.Sum256([]byte(user + "/second-synthetic-device"))
	if _, err := h.admin.Exec(h.ctx, `INSERT INTO identity.sessions(id,token_hash,user_id,issuer,subject,security_version,authenticated_at,created_at,last_accepted_at,absolute_expires_at,assurance_level,assurance_evidence)
SELECT gen_random_uuid(),$1,user_id,issuer,subject,security_version,authenticated_at,created_at,last_accepted_at,absolute_expires_at,assurance_level,assurance_evidence FROM identity.sessions WHERE token_hash=$2`, digest[:], fixtureSessionHash(user)); err != nil {
		t.Fatal(err)
	}
	return digest[:]
}

func updateDraftCommand(d listings.Draft) listings.UpdateDraftCommand {
	return listings.UpdateDraftCommand{WorkspaceID: d.Listing.WorkspaceID, DraftID: d.Listing.ID,
		RequestID: "30000000-0000-4000-8000-000000000001", CorrelationID: "30000000-0000-4000-8000-000000000002",
		ExpectedBusinessVersion: d.Business.Version, ExpectedListingVersion: d.Listing.Version,
		Fields: listings.Fields{PrivateLabel: d.Business.PrivateLabel, ActivityDescription: d.Business.ActivityDescription,
			Country: d.Business.Country, Region: d.Business.Region, Title: d.Listing.Title, Description: d.Listing.Description,
			SaleSubject: d.Listing.SaleSubject, TransferStructure: d.Listing.TransferStructure, SaleContext: d.Listing.SaleContext, SaleMethod: d.Listing.SaleMethod}}
}

func seedDraft(t *testing.T, h *databaseHarness, user, workspace string) (listings.CreatedDraft, listings.Draft) {
	t.Helper()
	grantDrafts(t, h, user, workspace, "listing.read_private", "listing.create", "listing.update")
	c := draftCommand(t, workspace)
	c.Fields = listings.Fields{PrivateLabel: "Private label canary", ActivityDescription: "Workshop activity", Country: "SE", Title: "Workshop", SaleSubject: "operating_business", TransferStructure: "business_transfer", SaleContext: "ordinary_sale", SaleMethod: "negotiation"}
	created, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(user), c)
	if err != nil {
		t.Fatal(err)
	}
	d, err := h.api.ReadListingDraftSession(h.ctx, fixtureSessionHash(user), workspace, created.ListingID)
	if err != nil {
		t.Fatal(err)
	}
	return created, d
}

func TestPostgresPrivateDrafts(t *testing.T) {
	h := integrationHarness(t)
	t.Run("old_grants_do_not_implicitly_authorize_drafts", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), draftCommand(t, workspaceA)); !errors.Is(err, ErrNotFound) {
			t.Fatal("workspace grants elevated to listing grants", err)
		}
		if _, err := h.worker.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), draftCommand(t, workspaceA)); !errors.Is(err, ErrUnsafeRole) {
			t.Fatal("worker acquired human drafting authority", err)
		}
		if _, err := h.worker.CreatePersonalListingDraftSession(h.ctx, fixtureSessionHash(userA), draftCommand(t, "")); !errors.Is(err, ErrUnsafeRole) {
			t.Fatal("worker acquired personal bootstrap authority", err)
		}
		assertCount(t, h, `SELECT count(*) FROM listings.businesses`, 0)
	})
	t.Run("incomplete_create_normalized_replay_conflict_and_durable_read", func(t *testing.T) {
		h.reset(t)
		grantDrafts(t, h, userA, workspaceA, "listing.read_private", "listing.create")
		c := draftCommand(t, workspaceA)
		c.Fields = listings.Fields{Title: " Workshop ", Country: "se", Description: "first\r\nsecond"}
		created, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), c)
		if err != nil || created.BusinessID == created.ListingID || created.BusinessVersion != 1 || created.ListingVersion != 1 || created.Replayed {
			t.Fatalf("create failed %+v %v", created, err)
		}
		c.Fields.Title, c.Fields.Country, c.Fields.Description = "Workshop", "SE", "first\nsecond"
		replay, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), c)
		if err != nil || !replay.Replayed || replay.BusinessID != created.BusinessID || replay.ListingID != created.ListingID {
			t.Fatalf("normalized replay %+v %v", replay, err)
		}
		c.Fields.Title = "Changed body"
		if _, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), c); !errors.Is(err, ErrConflict) {
			t.Fatal("changed request key accepted", err)
		}
		// A second restricted pool simulates application restart; no memory cache
		// or service instance is needed to recover authoritative state/receipts.
		other, err := Open(h.ctx, withDatabase(t, h.api.pool.Config().ConnString(), h.admin.Config().Database), "api")
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close()
		d, err := other.ReadListingDraftSession(h.ctx, fixtureSessionHash(userA), workspaceA, created.ListingID)
		if err != nil || d.Listing.Title != "Workshop" || d.Business.Country != "SE" || d.Listing.SaleSubject != "" || d.Listing.Status != "DRAFT" || d.Listing.UpdatedAt.Location() != time.UTC {
			t.Fatalf("durable incomplete draft %+v %v", d, err)
		}
		assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 2)
		assertCount(t, h, `SELECT count(*) FROM listings.command_receipts`, 1)
		assertCount(t, h, `SELECT count(*) FROM eventing.outbox`, 0)
		assertCount(t, h, `SELECT count(*) FROM organizations.workspaces WHERE id=$1 AND version=1`, 1, workspaceA)
	})
	t.Run("person_and_organization_isolation_and_explicit_permissions", func(t *testing.T) {
		h.reset(t)
		a, d := seedDraft(t, h, userA, workspaceA)
		b, _ := seedDraft(t, h, userB, workspaceB)
		org, _ := seedDraft(t, h, userA, workspaceOrg)
		for _, tc := range []struct{ user, workspace, id string }{{userA, workspaceB, b.ListingID}, {userA, workspaceA, b.ListingID}, {userB, workspaceA, a.ListingID}, {userA, workspaceA, unknown}, {userOrg, workspaceOrg, org.ListingID}, {userOwner, workspaceOwner, unknown}} {
			if _, err := h.api.ReadListingDraftSession(h.ctx, fixtureSessionHash(tc.user), tc.workspace, tc.id); !errors.Is(err, ErrNotFound) {
				t.Errorf("isolation read %+v %v", tc, err)
			}
			if _, err := h.api.PreviewListingDraftSession(h.ctx, fixtureSessionHash(tc.user), tc.workspace, tc.id); !errors.Is(err, ErrNotFound) {
				t.Errorf("isolation preview %+v %v", tc, err)
			}
			u := updateDraftCommand(d)
			u.WorkspaceID, u.DraftID = tc.workspace, tc.id
			if _, err := h.api.UpdateListingDraftSession(h.ctx, fixtureSessionHash(tc.user), u); !errors.Is(err, ErrNotFound) {
				t.Errorf("isolation update %+v %v", tc, err)
			}
		}
		grantDrafts(t, h, userRead, workspaceA, "listing.read_private")
		if _, err := h.api.ReadListingDraftSession(h.ctx, fixtureSessionHash(userRead), workspaceA, a.ListingID); err != nil {
			t.Fatal("explicit read denied", err)
		}
		if _, err := h.api.UpdateListingDraftSession(h.ctx, fixtureSessionHash(userRead), updateDraftCommand(d)); !errors.Is(err, ErrNotFound) {
			t.Fatal("read grant elevated to mutation", err)
		}
		var exposed int
		if err := h.api.pool.QueryRow(h.ctx, `SELECT count(*) FROM listings.drafts`).Scan(&exposed); err != nil || exposed != 0 {
			t.Fatal("transaction-local context escaped pool", exposed, err)
		}
	})
	t.Run("session_failures_fail_closed_including_bootstrap", func(t *testing.T) {
		for _, failure := range []string{"missing", "revoked", "expired", "idle", "disabled"} {
			t.Run(failure, func(t *testing.T) {
				h.reset(t)
				grantDrafts(t, h, userA, workspaceA, "listing.read_private", "listing.create")
				hash := fixtureSessionHash(userA)
				var err error
				switch failure {
				case "missing":
					hash = fixtureSessionHash(unknown)
				case "revoked":
					_, err = h.admin.Exec(h.ctx, `UPDATE identity.sessions SET revoked_at=clock_timestamp() WHERE user_id=$1`, userA)
				case "expired":
					_, err = h.admin.Exec(h.ctx, `WITH now_value AS (SELECT clock_timestamp() AS at) UPDATE identity.sessions SET authenticated_at=n.at-interval '9 hours',absolute_expires_at=n.at-interval '1 hour' FROM now_value n WHERE user_id=$1`, userA)
				case "idle":
					_, err = h.admin.Exec(h.ctx, `WITH now_value AS (SELECT clock_timestamp() AS at) UPDATE identity.sessions SET authenticated_at=n.at-interval '20 minutes',absolute_expires_at=n.at+interval '7 hours 40 minutes',created_at=n.at-interval '20 minutes',last_accepted_at=n.at-interval '16 minutes' FROM now_value n WHERE user_id=$1`, userA)
				case "disabled":
					_, err = h.admin.Exec(h.ctx, `UPDATE identity.users SET active=false WHERE id=$1`, userA)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err = h.api.CreateListingDraftSession(h.ctx, hash, draftCommand(t, workspaceA)); !errors.Is(err, identity.ErrUnauthenticated) {
					t.Fatal("credential failure permitted create", err)
				}
				if _, err = h.api.CreatePersonalListingDraftSession(h.ctx, hash, draftCommand(t, "")); !errors.Is(err, identity.ErrUnauthenticated) {
					t.Fatal("credential failure permitted bootstrap", err)
				}
				assertCount(t, h, `SELECT count(*) FROM organizations.personal_drafting_assignments`, 0)
				assertCount(t, h, `SELECT count(*) FROM listings.businesses`, 0)
			})
		}
	})
	t.Run("personal_bootstrap_two_devices_one_assignment_and_one_receipt", func(t *testing.T) {
		h.reset(t)
		hash2 := secondSession(t, h, userA)
		c := draftCommand(t, "")
		type outcome struct {
			value listings.CreatedDraft
			err   error
		}
		outcomes := make(chan outcome, 2)
		var wg sync.WaitGroup
		for _, hash := range [][]byte{fixtureSessionHash(userA), hash2} {
			wg.Add(1)
			go func(hash []byte) {
				defer wg.Done()
				value, err := h.api.CreatePersonalListingDraftSession(h.ctx, hash, c)
				outcomes <- outcome{value, err}
			}(hash)
		}
		wg.Wait()
		close(outcomes)
		var first listings.CreatedDraft
		replays := 0
		for o := range outcomes {
			if o.err != nil {
				t.Fatal(o.err)
			}
			if first.ListingID != "" && (o.value.WorkspaceID != first.WorkspaceID || o.value.ListingID != first.ListingID) {
				t.Fatal("concurrent bootstrap duplicated state")
			}
			first = o.value
			if o.value.Replayed {
				replays++
			}
		}
		if replays != 1 {
			t.Fatal("expected one immutable replay", replays)
		}
		assertCount(t, h, `SELECT count(*) FROM organizations.personal_drafting_assignments`, 1)
		assertCount(t, h, `SELECT count(*) FROM organizations.workspaces WHERE owner_user_id=$1`, 2, userA)
		assertCount(t, h, `SELECT count(*) FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2`, 5, first.WorkspaceID, userA)
		assertCount(t, h, `SELECT count(*) FROM listings.businesses`, 1)
		assertCount(t, h, `SELECT count(*) FROM listings.drafts`, 1)
		assertCount(t, h, `SELECT count(*) FROM listings.command_receipts`, 1)
		assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events WHERE actor_kind='HUMAN' AND actor_user_id=$1`, 3, userA)
	})
	t.Run("bootstrap_reuse_and_receipt_replay_never_regrant_revoked_access", func(t *testing.T) {
		for _, revoke := range []string{"listing.read_private", "listing.create", "membership"} {
			t.Run(revoke, func(t *testing.T) {
				h.reset(t)
				c := draftCommand(t, "")
				created, err := h.api.CreatePersonalListingDraftSession(h.ctx, fixtureSessionHash(userA), c)
				if err != nil {
					t.Fatal(err)
				}
				if revoke == "membership" {
					_, err = h.admin.Exec(h.ctx, `UPDATE organizations.workspace_memberships SET active=false WHERE workspace_id=$1 AND user_id=$2`, created.WorkspaceID, userA)
				} else {
					_, err = h.admin.Exec(h.ctx, `DELETE FROM organizations.workspace_permissions WHERE workspace_id=$1 AND user_id=$2 AND permission=$3`, created.WorkspaceID, userA, revoke)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := h.api.CreatePersonalListingDraftSession(h.ctx, fixtureSessionHash(userA), c); !errors.Is(err, ErrNotFound) {
					t.Fatal("revoked replay exposed receipt", err)
				}
				c.CommandID, err = newID()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := h.api.CreatePersonalListingDraftSession(h.ctx, fixtureSessionHash(userA), c); !errors.Is(err, ErrNotFound) {
					t.Fatal("reuse replenished revoked privilege", err)
				}
				assertCount(t, h, `SELECT count(*) FROM listings.drafts`, 1)
				if revoke == "membership" {
					assertCount(t, h, `SELECT count(*) FROM organizations.workspace_memberships WHERE workspace_id=$1 AND active`, 0, created.WorkspaceID)
				} else {
					assertCount(t, h, `SELECT count(*) FROM organizations.workspace_permissions WHERE workspace_id=$1 AND permission=$2`, 0, created.WorkspaceID, revoke)
				}
			})
		}
	})
	t.Run("independent_versions_noop_and_immutable_replay", func(t *testing.T) {
		h.reset(t)
		grantDrafts(t, h, userA, workspaceA, "listing.read_private", "listing.create", "listing.update")
		create := draftCommand(t, workspaceA)
		created, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), create)
		if err != nil {
			t.Fatal(err)
		}
		d, err := h.api.ReadListingDraftSession(h.ctx, fixtureSessionHash(userA), workspaceA, created.ListingID)
		if err != nil {
			t.Fatal(err)
		}
		u := updateDraftCommand(d)
		u.Fields.Title = "Listing only"
		d, err = h.api.UpdateListingDraftSession(h.ctx, fixtureSessionHash(userA), u)
		if err != nil || d.Business.Version != 1 || d.Listing.Version != 2 {
			t.Fatal("independent revision failed", d, err)
		}
		if _, err = h.api.UpdateListingDraftSession(h.ctx, fixtureSessionHash(userA), u); !errors.Is(err, ErrConflict) {
			t.Fatal("stale PUT accepted", err)
		}
		u = updateDraftCommand(d)
		if _, err = h.api.UpdateListingDraftSession(h.ctx, fixtureSessionHash(userA), u); err != nil {
			t.Fatal("no-op failed", err)
		}
		assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 3)
		replay, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), create)
		if err != nil || !replay.Replayed || replay.ListingVersion != 1 {
			t.Fatal("receipt rewrote immutable outcome", replay, err)
		}
		assertCount(t, h, `SELECT count(*) FROM organizations.workspaces WHERE id=$1 AND version=1`, 1, workspaceA)
	})
	t.Run("concurrent_writers_one_expected_version_winner", func(t *testing.T) {
		h.reset(t)
		_, d := seedDraft(t, h, userA, workspaceA)
		hash2 := secondSession(t, h, userA)
		outcomes := make(chan error, 2)
		var wg sync.WaitGroup
		for i, hash := range [][]byte{fixtureSessionHash(userA), hash2} {
			wg.Add(1)
			go func(i int, hash []byte) {
				defer wg.Done()
				u := updateDraftCommand(d)
				u.Fields.Title = []string{"First", "Second"}[i]
				u.Fields.PrivateLabel = u.Fields.Title
				_, err := h.api.UpdateListingDraftSession(h.ctx, hash, u)
				outcomes <- err
			}(i, hash)
		}
		wg.Wait()
		close(outcomes)
		wins, conflicts := 0, 0
		for err := range outcomes {
			if err == nil {
				wins++
			} else if errors.Is(err, ErrConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if wins != 1 || conflicts != 1 {
			t.Fatal("concurrent updates lost conflict", wins, conflicts)
		}
		assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 4)
		assertCount(t, h, `SELECT count(*) FROM listings.businesses WHERE version=2`, 1)
		assertCount(t, h, `SELECT count(*) FROM listings.drafts WHERE version=2`, 1)
	})
	t.Run("audit_and_receipt_failures_roll_back_bootstrap_and_updates", func(t *testing.T) {
		h.reset(t)
		if _, err := h.admin.Exec(h.ctx, `CREATE FUNCTION public.fail_draft_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic receipt failure'; END $$; CREATE TRIGGER fail_receipt BEFORE INSERT ON listings.command_receipts FOR EACH ROW EXECUTE FUNCTION public.fail_draft_receipt()`); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.CreatePersonalListingDraftSession(h.ctx, fixtureSessionHash(userA), draftCommand(t, "")); !errors.Is(err, ErrUnavailable) {
			t.Fatal("receipt failure not observable", err)
		}
		assertCount(t, h, `SELECT count(*) FROM organizations.personal_drafting_assignments`, 0)
		assertCount(t, h, `SELECT count(*) FROM organizations.workspaces`, 4)
		assertCount(t, h, `SELECT count(*) FROM listings.businesses`, 0)
		assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 0)
		if _, err := h.admin.Exec(h.ctx, `DROP TRIGGER fail_receipt ON listings.command_receipts; DROP FUNCTION public.fail_draft_receipt()`); err != nil {
			t.Fatal(err)
		}
		_, d := seedDraft(t, h, userA, workspaceA)
		if _, err := h.admin.Exec(h.ctx, `CREATE FUNCTION public.fail_draft_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='listing_draft.updated' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_audit BEFORE INSERT ON audit.aggregate_events FOR EACH ROW EXECUTE FUNCTION public.fail_draft_audit()`); err != nil {
			t.Fatal(err)
		}
		u := updateDraftCommand(d)
		u.Fields.Title = "Changed"
		u.Fields.PrivateLabel = "Changed"
		if _, err := h.api.UpdateListingDraftSession(h.ctx, fixtureSessionHash(userA), u); !errors.Is(err, ErrUnavailable) {
			t.Fatal("audit failure not observable", err)
		}
		assertCount(t, h, `SELECT count(*) FROM listings.businesses WHERE version=1 AND private_label='Private label canary'`, 1)
		assertCount(t, h, `SELECT count(*) FROM listings.drafts WHERE version=1 AND title='Workshop'`, 1)
		assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 2)
		if _, err := h.admin.Exec(h.ctx, `DROP TRIGGER fail_audit ON audit.aggregate_events; DROP FUNCTION public.fail_draft_audit()`); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("database_tenant_fk_immutable_evidence_and_reverse_audit_binding", func(t *testing.T) {
		h.reset(t)
		created, _ := seedDraft(t, h, userA, workspaceA)
		if _, err := h.admin.Exec(h.ctx, `INSERT INTO listings.drafts(id,workspace_id,business_id) VALUES($1,$2,$3)`, unknown, workspaceB, created.BusinessID); err == nil {
			t.Fatal("cross-tenant business link accepted")
		}
		tx, session, err := h.api.draftSessionTX(h.ctx, fixtureSessionHash(userA), workspaceA, identity.ListingUpdate)
		if err != nil {
			t.Fatal(err)
		}
		if err := appendDraftAudit(h.ctx, tx, session, workspaceA, "Business", created.BusinessID, "business.updated", 98, 99, "30000000-0000-4000-8000-000000000001", "30000000-0000-4000-8000-000000000002"); !errors.Is(err, ErrUnavailable) {
			t.Fatal("fabricated future audit accepted", err)
		}
		if err := rollback(tx); err != nil {
			t.Fatal(err)
		}
		tx, _, err = h.api.draftSessionTX(h.ctx, fixtureSessionHash(userA), workspaceA, identity.ListingUpdate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(h.ctx, `UPDATE listings.businesses SET version=version+1,private_label='Unaudited' WHERE id=$1`, created.BusinessID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(h.ctx); err == nil {
			t.Fatal("database committed state without audit")
		}
		if err := rollback(tx); err != nil {
			t.Fatal(err)
		}
		assertCount(t, h, `SELECT count(*) FROM listings.businesses WHERE version=1`, 1)
		for _, sql := range []string{`UPDATE audit.aggregate_events SET result='SUCCESS'`, `DELETE FROM listings.command_receipts`, `UPDATE listings.drafts SET status='PUBLISHED'`, `SELECT * FROM organizations.personal_drafting_assignments`} {
			if _, err := h.api.pool.Exec(h.ctx, sql); err == nil {
				t.Fatal("unexpected API capability", sql)
			}
		}
		for _, sql := range []string{`SELECT * FROM listings.drafts`, `SELECT * FROM audit.aggregate_events`, `SELECT organizations.personal_drafting_workspace(NULL,NULL,NULL)`} {
			if _, err := h.worker.pool.Exec(h.ctx, sql); err == nil {
				t.Fatal("unexpected worker capability", sql)
			}
		}
	})
	t.Run("cancellation_rolls_back_without_receipt", func(t *testing.T) {
		h.reset(t)
		grantDrafts(t, h, userA, workspaceA, "listing.read_private", "listing.create")
		ctx, cancel := context.WithCancel(h.ctx)
		cancel()
		if _, err := h.api.CreateListingDraftSession(ctx, fixtureSessionHash(userA), draftCommand(t, workspaceA)); !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation ignored", err)
		}
		assertCount(t, h, `SELECT count(*) FROM listings.command_receipts`, 0)
	})
}

func waitDraftLock(t *testing.T, h *databaseHarness, fragment string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := h.admin.QueryRow(h.ctx, `SELECT EXISTS(SELECT FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1)`, "%"+fragment+"%").Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("request never reached expected PostgreSQL lock", fragment)
}

func TestPostgresDraftRechecksExpirationAfterCommandLock(t *testing.T) {
	h := integrationHarness(t)
	h.reset(t)
	grantDrafts(t, h, userA, workspaceA, "listing.read_private", "listing.create")
	c := draftCommand(t, workspaceA)
	lock, err := pgx.ConnectConfig(h.ctx, h.admin.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	key := userA + "/WORKSPACE/" + workspaceA + "/listing_draft.create/" + c.CommandID
	if _, err = lock.Exec(h.ctx, `SELECT pg_advisory_lock(hashtextextended($1,1707))`, key); err != nil {
		t.Fatal(err)
	}
	if _, err = h.admin.Exec(h.ctx, `WITH now_value AS (SELECT clock_timestamp() AS at) UPDATE identity.sessions SET authenticated_at=n.at-interval '8 hours'+interval '1 second',absolute_expires_at=n.at+interval '1 second' FROM now_value n WHERE token_hash=$1`, fixtureSessionHash(userA)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), c); done <- err }()
	waitDraftLock(t, h, "pg_advisory_xact_lock")
	time.Sleep(1200 * time.Millisecond)
	if _, err = lock.Exec(h.ctx, `SELECT pg_advisory_unlock(hashtextextended($1,1707))`, key); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatal("expired credential passed command wait", err)
	}
	assertCount(t, h, `SELECT count(*) FROM listings.businesses`, 0)
}

func TestDraftCommandValidation(t *testing.T) {
	for _, test := range []struct {
		c        listings.CreateDraftCommand
		personal bool
	}{
		{listings.CreateDraftCommand{}, false}, {listings.CreateDraftCommand{WorkspaceID: workspaceA}, true},
		{listings.CreateDraftCommand{WorkspaceID: workspaceA, CommandID: unknown, RequestID: unknown, CorrelationID: unknown, Fields: listings.Fields{Title: strings.Repeat("x", 161)}}, false},
	} {
		if _, err := validateCreateDraft(test.c, test.personal); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid create accepted", err)
		}
	}
}

// A fault at the commit boundary operates on real PostgreSQL, never an in-memory
// persistence replacement. One scenario actually commits and loses its response;
// the other loses the transport before committing and cleanup rolls it back.
type draftCommitFaultTX struct {
	pgx.Tx
	committed bool
}

func (tx draftCommitFaultTX) Commit(ctx context.Context) error {
	if tx.committed {
		if err := tx.Tx.Commit(ctx); err != nil {
			return err
		}
	}
	return io.ErrUnexpectedEOF
}

func TestPostgresDraftCommitUncertaintySafeRetry(t *testing.T) {
	h := integrationHarness(t)
	for _, committed := range []bool{true, false} {
		name := "not_committed"
		if committed {
			name = "committed_response_lost"
		}
		t.Run(name, func(t *testing.T) {
			h.reset(t)
			grantDrafts(t, h, userA, workspaceA, "listing.read_private", "listing.create")
			c := draftCommand(t, workspaceA)
			f, err := validateCreateDraft(c, false)
			if err != nil {
				t.Fatal(err)
			}
			tx, session, err := h.api.draftSessionTX(h.ctx, fixtureSessionHash(userA), workspaceA, identity.ListingCreate)
			if err != nil {
				t.Fatal(err)
			}
			_, err = createDraft(h.ctx, draftCommitFaultTX{Tx: tx, committed: committed}, fixtureSessionHash(userA), session, c, f, "WORKSPACE")
			if !errors.Is(err, ErrCommitUncertain) || !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, ErrUnavailable) {
				t.Fatal("uncertain commit misclassified", err)
			}
			if err := rollback(tx); err != nil {
				t.Fatal(err)
			}
			count := int64(0)
			if committed {
				count = 1
			}
			assertCount(t, h, `SELECT count(*) FROM listings.drafts`, count)
			assertCount(t, h, `SELECT count(*) FROM listings.command_receipts`, count)
			retry, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), c)
			if err != nil || retry.Replayed != committed {
				t.Fatal("durable retry failed", retry, err)
			}
			assertCount(t, h, `SELECT count(*) FROM listings.drafts`, 1)
			assertCount(t, h, `SELECT count(*) FROM listings.businesses`, 1)
			assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 2)
		})
	}
}

func TestPostgresDraftRechecksExpirationAfterBusinessLock(t *testing.T) {
	h := integrationHarness(t)
	h.reset(t)
	_, d := seedDraft(t, h, userA, workspaceA)
	lock, err := pgx.ConnectConfig(h.ctx, h.admin.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	blocking, err := lock.Begin(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rollback(blocking); err != nil {
			t.Error(err)
		}
	}()
	if _, err = blocking.Exec(h.ctx, `SELECT id FROM listings.businesses WHERE id=$1 FOR UPDATE`, d.Business.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.admin.Exec(h.ctx, `WITH now_value AS (SELECT clock_timestamp() AS at) UPDATE identity.sessions SET authenticated_at=n.at-interval '8 hours'+interval '1 second',absolute_expires_at=n.at+interval '1 second' FROM now_value n WHERE token_hash=$1`, fixtureSessionHash(userA)); err != nil {
		t.Fatal(err)
	}
	u := updateDraftCommand(d)
	u.Fields.Title = "Must not commit"
	done := make(chan error, 1)
	go func() { _, err := h.api.UpdateListingDraftSession(h.ctx, fixtureSessionHash(userA), u); done <- err }()
	waitDraftLock(t, h, "SELECT version FROM listings.businesses")
	time.Sleep(1200 * time.Millisecond)
	if err = blocking.Commit(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatal("expired credential passed resource wait", err)
	}
	assertCount(t, h, `SELECT count(*) FROM listings.drafts WHERE version=1 AND title='Workshop'`, 1)
	assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 2)
}

func TestPostgresRuntimeRejectsOwnershipOfListingSchema(t *testing.T) {
	h := integrationHarness(t)
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	login := "draft_owner_login_" + strings.ReplaceAll(id, "-", "")
	if _, err = h.admin.Exec(h.ctx, `CREATE ROLE `+pgx.Identifier{login}.Sanitize()+` LOGIN INHERIT PASSWORD '`+id+`'; GRANT foundation_api TO `+pgx.Identifier{login}.Sanitize()+`; ALTER TABLE listings.businesses OWNER TO `+pgx.Identifier{login}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := h.admin.Exec(h.ctx, `ALTER TABLE listings.businesses OWNER TO `+pgx.Identifier{h.admin.Config().User}.Sanitize()+`; DROP ROLE `+pgx.Identifier{login}.Sanitize()); err != nil {
			t.Error(err)
		}
	})
	config := h.admin.Config().Copy()
	config.User, config.Password = login, id
	s, err := Open(h.ctx, config.ConnString(), "api")
	if s != nil {
		s.Close()
	}
	if !errors.Is(err, ErrUnsafeRole) {
		t.Fatal("runtime listing-table ownership passed guard", err)
	}
}

func TestPostgresDraftLiveCancellationDuringCommandLock(t *testing.T) {
	h := integrationHarness(t)
	h.reset(t)
	grantDrafts(t, h, userA, workspaceA, "listing.read_private", "listing.create")
	c := draftCommand(t, workspaceA)
	var acceptedBefore time.Time
	if err := h.admin.QueryRow(h.ctx, `SELECT last_accepted_at FROM identity.sessions WHERE token_hash=$1`, fixtureSessionHash(userA)).Scan(&acceptedBefore); err != nil {
		t.Fatal(err)
	}
	lock, err := pgx.ConnectConfig(h.ctx, h.admin.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	key := userA + "/WORKSPACE/" + workspaceA + "/listing_draft.create/" + c.CommandID
	if _, err = lock.Exec(h.ctx, `SELECT pg_advisory_lock(hashtextextended($1,1707))`, key); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := h.api.CreateListingDraftSession(ctx, fixtureSessionHash(userA), c)
		done <- err
	}()
	// Observe the real PostgreSQL lock wait before cancellation, rather than
	// relying on a delay or a context canceled before the operation starts.
	waitDraftLock(t, h, "pg_advisory_xact_lock")
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("live cancellation was not propagated", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("live cancellation did not complete promptly")
	}
	assertCount(t, h, `SELECT count(*) FROM listings.businesses`, 0)
	assertCount(t, h, `SELECT count(*) FROM listings.drafts`, 0)
	assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 0)
	assertCount(t, h, `SELECT count(*) FROM listings.command_receipts`, 0)
	var acceptedAfter time.Time
	if err := h.admin.QueryRow(h.ctx, `SELECT last_accepted_at FROM identity.sessions WHERE token_hash=$1`, fixtureSessionHash(userA)).Scan(&acceptedAfter); err != nil {
		t.Fatal(err)
	}
	if !acceptedAfter.Equal(acceptedBefore) {
		t.Fatal("canceled command extended idle-session activity")
	}
	var released bool
	if err = lock.QueryRow(h.ctx, `SELECT pg_advisory_unlock(hashtextextended($1,1707))`, key).Scan(&released); err != nil || !released {
		t.Fatal("command lock was not released", err)
	}
	created, err := h.api.CreateListingDraftSession(h.ctx, fixtureSessionHash(userA), c)
	if err != nil || created.Replayed {
		t.Fatal("same command could not succeed after cancellation", created, err)
	}
	assertCount(t, h, `SELECT count(*) FROM listings.businesses`, 1)
	assertCount(t, h, `SELECT count(*) FROM listings.drafts`, 1)
	assertCount(t, h, `SELECT count(*) FROM audit.aggregate_events`, 2)
	assertCount(t, h, `SELECT count(*) FROM listings.command_receipts`, 1)
}
