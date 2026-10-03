package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
)

const identityRequestID = "30000000-0000-4000-8000-000000000001"
const identityCorrelationID = "30000000-0000-4000-8000-000000000002"

func randomHash(t *testing.T) []byte {
	t.Helper()
	token, err := identity.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := identity.HashToken(token)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
func loginFixture(t *testing.T) LoginTransaction {
	t.Helper()
	nonce, err := identity.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := identity.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	return LoginTransaction{StateHash: randomHash(t), BindingHash: randomHash(t), Nonce: nonce, PKCEVerifier: verifier, RequestID: identityRequestID, CorrelationID: identityCorrelationID}
}
func authFixture(subject string) identity.Authentication {
	return identity.Authentication{Provider: "synthetic", Issuer: "https://synthetic.invalid", Subject: subject, AuthenticatedAt: time.Now().UTC(), EmailVerified: true, Assurance: identity.Assurance{Level: identity.AssuranceUnknown}}
}
func completeFixture(t *testing.T, h *databaseHarness, subject string) (Session, []byte) {
	t.Helper()
	hash := randomHash(t)
	s, err := h.api.CompleteLogin(h.ctx, authFixture(subject), hash, nil, time.Now().UTC(), identityRequestID, identityCorrelationID)
	if err != nil {
		t.Fatal(err)
	}
	return s, hash
}

func TestPostgresIdentity(t *testing.T) {
	h := integrationHarness(t)
	t.Run("one time browser bound login and expiry", func(t *testing.T) {
		h.reset(t)
		c := loginFixture(t)
		if err := h.api.CreateLogin(h.ctx, c); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.ConsumeLogin(h.ctx, c.StateHash, randomHash(t)); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal(err)
		}
		got, err := h.api.ConsumeLogin(h.ctx, c.StateHash, c.BindingHash)
		if err != nil || got.Nonce != c.Nonce || got.PKCEVerifier != c.PKCEVerifier || got.StartedAt.IsZero() {
			t.Fatalf("consume: %v", err)
		}
		if _, err = h.api.ConsumeLogin(h.ctx, c.StateHash, c.BindingHash); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal(err)
		}
		c = loginFixture(t)
		if err = h.api.CreateLogin(h.ctx, c); err != nil {
			t.Fatal(err)
		}
		if _, err = h.admin.Exec(h.ctx, `UPDATE identity.login_transactions SET started_at=clock_timestamp()-interval '6 minutes',expires_at=clock_timestamp()-interval '1 minute'`); err != nil {
			t.Fatal(err)
		}
		if _, err = h.api.ConsumeLogin(h.ctx, c.StateHash, c.BindingHash); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
	t.Run("parallel login consumption has one winner", func(t *testing.T) {
		h.reset(t)
		c := loginFixture(t)
		if err := h.api.CreateLogin(h.ctx, c); err != nil {
			t.Fatal(err)
		}
		results := make(chan error, 8)
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := h.api.ConsumeLogin(h.ctx, c.StateHash, c.BindingHash)
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		winners := 0
		for err := range results {
			if err == nil {
				winners++
			} else if !errors.Is(err, identity.ErrUnauthenticated) {
				t.Fatal(err)
			}
		}
		if winners != 1 {
			t.Fatalf("winners=%d", winners)
		}
	})
	t.Run("global persistent login admission and cleanup", func(t *testing.T) {
		h.reset(t)
		if _, err := h.admin.Exec(h.ctx, `INSERT INTO identity.login_admission(bucket,attempts) VALUES(date_trunc('minute',clock_timestamp()),120)`); err != nil {
			t.Fatal(err)
		}
		if err := h.api.CreateLogin(h.ctx, loginFixture(t)); !errors.Is(err, ErrRateLimited) {
			t.Fatal(err)
		}
		assertCount(t, h, `SELECT count(*) FROM identity.login_transactions`, 0)
		if _, err := h.admin.Exec(h.ctx, `UPDATE identity.login_admission SET bucket=bucket-interval '1 minute'`); err != nil {
			t.Fatal(err)
		}
		if err := h.api.CreateLogin(h.ctx, loginFixture(t)); err != nil {
			t.Fatal(err)
		}
		assertCount(t, h, `SELECT count(*) FROM identity.login_admission`, 1)
	})
	t.Run("verified admission no implicit grants canonical principal", func(t *testing.T) {
		h.reset(t)
		s, hash := completeFixture(t, h, "new-subject")
		if s.Principal.Kind != identity.Human || s.Principal.PrincipalID != s.Principal.UserID || s.Principal.SessionID == "" || s.Principal.Authentication.Subject != "new-subject" {
			t.Fatal("incorrect principal")
		}
		if s.AbsoluteExpiresAt.Sub(s.Principal.Authentication.AuthenticatedAt) != 8*time.Hour {
			t.Fatal("absolute lifetime")
		}
		assertCount(t, h, `SELECT count(*) FROM organizations.workspace_memberships WHERE user_id=$1`, 0, s.Principal.UserID)
		assertCount(t, h, `SELECT count(*) FROM audit.security_events WHERE actor_kind='HUMAN' AND actor_user_id=$1 AND target_user_id=$1`, 2, s.Principal.UserID)
		if _, err := h.api.ReadWorkspaceSession(h.ctx, hash, workspaceA); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
		got, err := h.api.ResolveSession(h.ctx, hash)
		if err != nil || got.Principal != s.Principal {
			t.Fatalf("resolve %v", err)
		}
	})
	t.Run("unverified stale future and disabled identity denied", func(t *testing.T) {
		h.reset(t)
		for _, mutate := range []func(*identity.Authentication){func(a *identity.Authentication) { a.EmailVerified = false }, func(a *identity.Authentication) { a.AuthenticatedAt = time.Now().Add(-2 * time.Minute) }, func(a *identity.Authentication) { a.AuthenticatedAt = time.Now().Add(2 * time.Minute) }, func(a *identity.Authentication) { a.Subject = userInactive }} {
			a := authFixture("not-admitted")
			mutate(&a)
			if _, err := h.api.CompleteLogin(h.ctx, a, randomHash(t), nil, time.Now().UTC(), identityRequestID, identityCorrelationID); !errors.Is(err, identity.ErrUnauthenticated) {
				t.Fatal(err)
			}
		}
		assertCount(t, h, `SELECT count(*) FROM identity.users`, 6)
		assertCount(t, h, `SELECT count(*) FROM audit.security_events`, 0)
		for _, hash := range [][]byte{nil, []byte("malformed"), randomHash(t), fixtureSessionHash(userInactive)} {
			if _, err := h.api.ResolveSession(h.ctx, hash); !errors.Is(err, identity.ErrUnauthenticated) {
				t.Fatal(err)
			}
		}
	})
	t.Run("parallel new subject mapping is unique", func(t *testing.T) {
		h.reset(t)
		hashes := [][]byte{randomHash(t), randomHash(t), randomHash(t), randomHash(t)}
		results := make(chan error, len(hashes))
		var wg sync.WaitGroup
		for _, hash := range hashes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := h.api.CompleteLogin(h.ctx, authFixture("parallel"), hash, nil, time.Now().UTC(), identityRequestID, identityCorrelationID)
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}
		assertCount(t, h, `SELECT count(*) FROM identity.external_identities WHERE subject='parallel'`, 1)
		assertCount(t, h, `SELECT count(*) FROM audit.security_events WHERE action='account.created'`, 1)
	})
	t.Run("admission audit failure rolls back user identity session", func(t *testing.T) {
		h.reset(t)
		remove := injectFailure(t, h, "audit.security_events", "identity_audit_failure")
		defer remove()
		if _, err := h.api.CompleteLogin(h.ctx, authFixture("rollback"), randomHash(t), nil, time.Now().UTC(), identityRequestID, identityCorrelationID); !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
		assertCount(t, h, `SELECT count(*) FROM identity.users`, 6)
		assertCount(t, h, `SELECT count(*) FROM identity.external_identities WHERE subject='rollback'`, 0)
		assertCount(t, h, `SELECT count(*) FROM identity.sessions`, 6)
	})
	t.Run("rotation current revocation all revocation and audit rollback", func(t *testing.T) {
		h.reset(t)
		_, hash := completeFixture(t, h, userA)
		replacement := randomHash(t)
		if _, err := h.api.CompleteLogin(h.ctx, authFixture(userA), replacement, hash, time.Now().UTC(), identityRequestID, identityCorrelationID); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.ResolveSession(h.ctx, hash); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal(err)
		}
		if _, err := h.api.CompleteLogin(h.ctx, authFixture(userB), randomHash(t), replacement, time.Now().UTC(), identityRequestID, identityCorrelationID); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal("cross account rotation", err)
		}
		remove := injectFailure(t, h, "audit.security_events", "revoke_audit_failure")
		if err := h.api.RevokeSession(h.ctx, replacement, true, identityRequestID, identityCorrelationID); !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
		remove()
		if _, err := h.api.ResolveSession(h.ctx, replacement); err != nil {
			t.Fatal("audit failure revoked session", err)
		}
		if err := h.api.RevokeSession(h.ctx, replacement, true, identityRequestID, identityCorrelationID); err != nil {
			t.Fatal(err)
		}
		for _, hash := range [][]byte{replacement, fixtureSessionHash(userA)} {
			if _, err := h.api.ResolveSession(h.ctx, hash); !errors.Is(err, identity.ErrUnauthenticated) {
				t.Fatal(err)
			}
		}
		if _, err := h.api.ResolveSession(h.ctx, fixtureSessionHash(userB)); err != nil {
			t.Fatal("revoked other account", err)
		}
		_, hash = completeFixture(t, h, userA)
		if err := h.api.RevokeSession(h.ctx, hash, false, identityRequestID, identityCorrelationID); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.ResolveSession(h.ctx, hash); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
	t.Run("only successful protected transaction extends idle", func(t *testing.T) {
		h.reset(t)
		hash := fixtureSessionHash(userA)
		if _, err := h.admin.Exec(h.ctx, `UPDATE identity.sessions SET authenticated_at=statement_timestamp()-interval '11 minutes',absolute_expires_at=statement_timestamp()+interval '7 hours 49 minutes',created_at=statement_timestamp()-interval '11 minutes',last_accepted_at=statement_timestamp()-interval '10 minutes' WHERE token_hash=$1`, hash); err != nil {
			t.Fatal(err)
		}
		before, err := h.api.ResolveSession(h.ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.api.ReadWorkspaceSession(h.ctx, hash, workspaceB); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
		if _, err = h.api.UpdateWorkspaceNameSession(h.ctx, hash, command(userA, workspaceA, 2).UpdateWorkspaceNameCommand); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		after, err := h.api.ResolveSession(h.ctx, hash)
		if err != nil || !after.IdleExpiresAt.Equal(before.IdleExpiresAt) {
			t.Fatal("failed request touched idle", err)
		}
		if _, err = h.api.ReadWorkspaceSession(h.ctx, hash, workspaceA); err != nil {
			t.Fatal(err)
		}
		after, err = h.api.ResolveSession(h.ctx, hash)
		if err != nil || !after.IdleExpiresAt.After(before.IdleExpiresAt) || !after.AbsoluteExpiresAt.Equal(before.AbsoluteExpiresAt) {
			t.Fatal("incorrect idle/absolute touch", err)
		}
	})
	t.Run("idle absolute account version expiry are authoritative", func(t *testing.T) {
		for _, query := range []string{`UPDATE identity.sessions SET authenticated_at=statement_timestamp()-interval '17 minutes',absolute_expires_at=statement_timestamp()+interval '7 hours 43 minutes',created_at=statement_timestamp()-interval '17 minutes',last_accepted_at=statement_timestamp()-interval '16 minutes' WHERE user_id=$1`, `UPDATE identity.sessions SET authenticated_at=statement_timestamp()-interval '9 hours',absolute_expires_at=statement_timestamp()-interval '1 hour' WHERE user_id=$1`, `UPDATE identity.users SET security_version=security_version+1 WHERE id=$1`} {
			h.reset(t)
			if _, err := h.admin.Exec(h.ctx, query, userA); err != nil {
				t.Fatal(err)
			}
			if _, err := h.api.ReadWorkspaceSession(h.ctx, fixtureSessionHash(userA), workspaceA); !errors.Is(err, identity.ErrUnauthenticated) {
				t.Fatal(err)
			}
		}
	})
	t.Run("operator disable machine attribution and runtime denial", func(t *testing.T) {
		h.reset(t)
		if _, err := h.api.pool.Exec(h.ctx, `SELECT identity.disable_account($1,$2,$3)`, userA, identityRequestID, identityCorrelationID); err == nil {
			t.Fatal("API could disable accounts")
		}
		var disabled bool
		if err := h.admin.QueryRow(h.ctx, `SELECT identity.disable_account($1,$2,$3)`, userA, identityRequestID, identityCorrelationID).Scan(&disabled); err != nil || !disabled {
			t.Fatal(err)
		}
		if _, err := h.api.ReadWorkspaceSession(h.ctx, fixtureSessionHash(userA), workspaceA); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal(err)
		}
		assertCount(t, h, `SELECT count(*) FROM audit.security_events WHERE action='account.disabled' AND actor_kind='SERVICE' AND actor_user_id IS NULL AND actor_service_id='40000000-0000-4000-8000-000000000002' AND target_user_id=$1`, 1, userA)
		if _, err := h.api.CompleteLogin(h.ctx, authFixture(userA), randomHash(t), nil, time.Now().UTC(), identityRequestID, identityCorrelationID); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
	t.Run("revocation is ordered after in flight protected transaction", func(t *testing.T) {
		h.reset(t)
		hash := fixtureSessionHash(userA)
		tx, _, err := h.api.protectedSessionTX(h.ctx, hash, workspaceA, "workspace.read")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := rollback(tx); err != nil {
				t.Error(err)
			}
		}()
		ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- h.api.RevokeSession(ctx, hash, true, identityRequestID, identityCorrelationID) }()
		select {
		case err := <-result:
			t.Fatalf("revocation did not wait for protected transaction: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		if err = tx.Commit(h.ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; err != nil {
			t.Fatal(err)
		}
		if _, err = h.api.ReadWorkspaceSession(h.ctx, hash, workspaceA); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
	t.Run("concurrent same session operations preserve authorization", func(t *testing.T) {
		h.reset(t)
		results := make(chan error, 12)
		var wg sync.WaitGroup
		for range 12 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := h.api.ReadWorkspaceSession(h.ctx, fixtureSessionHash(userA), workspaceA)
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err := h.admin.Exec(h.ctx, `DELETE FROM organizations.workspace_permissions WHERE user_id=$1 AND permission='workspace.update'`, userA); err != nil {
			t.Fatal(err)
		}
		if _, err := h.api.UpdateWorkspaceNameSession(h.ctx, fixtureSessionHash(userA), command(userA, workspaceA, 1).UpdateWorkspaceNameCommand); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	})
	t.Run("expiry and committed revocation while waiting cannot authorize", func(t *testing.T) {
		for _, scenario := range []string{"session expires during lock wait", "revocation commits before pending read"} {
			t.Run(scenario, func(t *testing.T) {
				h.reset(t)
				hash := fixtureSessionHash(userA)
				if scenario == "session expires during lock wait" {
					// Commit the near-future expiry before taking the blocking lock.
					// The row remains unchanged while database wall-clock time passes it.
					if _, err := h.admin.Exec(h.ctx, `WITH timing AS (SELECT clock_timestamp()+interval '500 milliseconds' AS deadline)
UPDATE identity.sessions SET authenticated_at=timing.deadline-interval '8 hours',absolute_expires_at=timing.deadline
FROM timing WHERE token_hash=$1`, hash); err != nil {
						t.Fatal(err)
					}
				}
				tx, err := h.admin.Begin(h.ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := rollback(tx); err != nil {
						t.Error(err)
					}
				}()
				if scenario == "session expires during lock wait" {
					if _, err = tx.Exec(h.ctx, `SELECT id FROM identity.sessions WHERE token_hash=$1 FOR UPDATE`, hash); err != nil {
						t.Fatal(err)
					}
					var unexpired bool
					if err = tx.QueryRow(h.ctx, `SELECT absolute_expires_at>clock_timestamp() FROM identity.sessions WHERE token_hash=$1`, hash).Scan(&unexpired); err != nil || !unexpired {
						t.Fatalf("temporal-crossing precondition failed: session must still be valid before the waiting read: %v", err)
					}
				} else {
					if _, err = tx.Exec(h.ctx, `SELECT identity.revoke_session($1,true,$2,$3)`, hash, identityRequestID, identityCorrelationID); err != nil {
						t.Fatal(err)
					}
				}
				ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
				defer cancel()
				result := make(chan error, 1)
				go func() { _, err := h.api.ReadWorkspaceSession(ctx, hash, workspaceA); result <- err }()
				select {
				case err := <-result:
					t.Fatalf("read did not wait for security state: %v", err)
				case <-time.After(100 * time.Millisecond):
				}
				if scenario == "session expires during lock wait" {
					for {
						var expired bool
						if err = tx.QueryRow(ctx, `SELECT absolute_expires_at<=clock_timestamp() FROM identity.sessions WHERE token_hash=$1`, hash).Scan(&expired); err != nil {
							t.Fatal(err)
						}
						if expired {
							break
						}
						select {
						case <-ctx.Done():
							t.Fatal("database expiry was not crossed within the test deadline")
						case <-time.After(25 * time.Millisecond):
						}
					}
					var waitingBeforeExpiry bool
					if err = tx.QueryRow(ctx, `SELECT EXISTS (
SELECT FROM pg_stat_activity a CROSS JOIN identity.sessions s
WHERE s.token_hash=$1 AND a.datname=current_database() AND a.wait_event_type='Lock'
  AND a.query LIKE 'SELECT user_id::text,session_id::text,provider,%identity.resolve_session%'
  AND a.query_start<s.absolute_expires_at)`, hash).Scan(&waitingBeforeExpiry); err != nil || !waitingBeforeExpiry {
						t.Fatalf("temporal-crossing precondition failed: protected request must have begun before expiry and still wait for the lock: %v", err)
					}
				}
				if err = tx.Commit(h.ctx); err != nil {
					t.Fatal(err)
				}
				if err = <-result; !errors.Is(err, identity.ErrUnauthenticated) {
					t.Fatalf("stale security facts authorized: %v", err)
				}
			})
		}
	})
	t.Run("restricted credentials direct context forged and mapping constraints", func(t *testing.T) {
		h.reset(t)
		for _, q := range []string{`SELECT * FROM identity.sessions`, `SELECT * FROM identity.login_transactions`, `UPDATE audit.security_events SET metadata='{}'`, `DELETE FROM audit.security_events`} {
			if _, err := h.api.pool.Exec(h.ctx, q); err == nil {
				t.Fatalf("runtime allowed %s", q)
			}
		}
		if _, err := h.worker.ResolveSession(h.ctx, fixtureSessionHash(userA)); !errors.Is(err, ErrUnsafeRole) {
			t.Fatal(err)
		}
		tx, err := h.api.pool.Begin(h.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := rollback(tx); err != nil {
				t.Error(err)
			}
		}()
		if _, err = tx.Exec(h.ctx, `SELECT set_config('app.actor_id',$1,true),set_config('app.workspace_id',$2,true)`, userA, workspaceA); err != nil {
			t.Fatal(err)
		}
		var n int
		if err = tx.QueryRow(h.ctx, `SELECT count(*) FROM organizations.workspaces`).Scan(&n); err != nil || n != 0 {
			t.Fatal("forged user context access", err)
		}
		if _, err = h.admin.Exec(h.ctx, `UPDATE identity.sessions SET user_id=$1 WHERE user_id=$2`, userB, userA); err == nil {
			t.Fatal("session user identity mismatch accepted")
		}
	})
	t.Run("cancellation and persistence outage fail safely", func(t *testing.T) {
		h.reset(t)
		ctx, cancel := context.WithCancel(h.ctx)
		cancel()
		if _, err := h.api.ResolveSession(ctx, fixtureSessionHash(userA)); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if _, err := h.api.ConsumeLogin(ctx, randomHash(t), randomHash(t)); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		h.api.Close()
		if _, err := h.api.ResolveSession(h.ctx, fixtureSessionHash(userA)); !errors.Is(err, ErrUnavailable) {
			t.Fatal("closed persistence dependency did not fail safely", err)
		}
	})
}
