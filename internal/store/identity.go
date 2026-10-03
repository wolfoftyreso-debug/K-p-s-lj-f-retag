package store

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
)

var ErrRateLimited = errors.New("login admission limit exceeded")

// LoginTransaction contains short-lived protocol secrets, never audit metadata.
// The database supplies StartedAt; callers cannot choose transaction lifetime.
type LoginTransaction struct {
	StateHash, BindingHash   []byte
	Nonce, PKCEVerifier      string
	StartedAt                time.Time
	RequestID, CorrelationID string
}

type Session struct {
	Principal                        identity.Principal
	AbsoluteExpiresAt, IdleExpiresAt time.Time
}

func validHash(hash []byte) bool { return len(hash) == 32 }

func (s *Store) CreateLogin(ctx context.Context, c LoginTransaction) error {
	if s.role != "api" {
		return ErrUnsafeRole
	}
	if !validHash(c.StateHash) || !validHash(c.BindingHash) || !validID(c.RequestID) || !validID(c.CorrelationID) {
		return ErrInvalid
	}
	if _, err := identity.HashToken(c.Nonce); err != nil {
		return ErrInvalid
	}
	if _, err := identity.HashToken(c.PKCEVerifier); err != nil {
		return ErrInvalid
	}
	var admitted bool
	if err := s.pool.QueryRow(ctx, `SELECT identity.create_login($1,$2,$3,$4,$5,$6)`, c.StateHash, c.BindingHash, c.Nonce, c.PKCEVerifier, c.RequestID, c.CorrelationID).Scan(&admitted); err != nil {
		return fault("login_create", err)
	}
	if !admitted {
		return ErrRateLimited
	}
	return nil
}

func (s *Store) ConsumeLogin(ctx context.Context, stateHash, bindingHash []byte) (c LoginTransaction, err error) {
	if s.role != "api" {
		return c, ErrUnsafeRole
	}
	if !validHash(stateHash) || !validHash(bindingHash) {
		return c, identity.ErrUnauthenticated
	}
	err = s.pool.QueryRow(ctx, `SELECT nonce,pkce_verifier,started_at,request_id::text,correlation_id::text FROM identity.consume_login($1,$2)`, stateHash, bindingHash).Scan(&c.Nonce, &c.PKCEVerifier, &c.StartedAt, &c.RequestID, &c.CorrelationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, identity.ErrUnauthenticated
	}
	if err != nil {
		return c, fault("login_consume", err)
	}
	c.StateHash = stateHash
	c.BindingHash = bindingHash
	return c, nil
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func resolveSession(ctx context.Context, q rowQuerier, hash []byte) (Session, error) {
	var session Session
	a := &session.Principal.Authentication
	err := q.QueryRow(ctx, `SELECT user_id::text,session_id::text,provider,issuer,subject,authenticated_at,assurance_level,assurance_evidence,absolute_expires_at,idle_expires_at FROM identity.resolve_session($1)`, hash).Scan(
		&session.Principal.UserID, &session.Principal.SessionID, &a.Provider, &a.Issuer, &a.Subject, &a.AuthenticatedAt, &a.Assurance.Level, &a.Assurance.Evidence, &session.AbsoluteExpiresAt, &session.IdleExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, identity.ErrUnauthenticated
	}
	if err != nil {
		return Session{}, fault("session_resolve", err)
	}
	session.Principal.PrincipalID = session.Principal.UserID
	session.Principal.Kind = identity.Human
	session.Principal.CredentialSource = identity.OIDCSession
	a.EmailVerified = true
	return session, nil
}

// ResolveSession never refreshes idle expiry. Authorization must revalidate
// inside the transaction that reads or changes a protected resource.
func (s *Store) ResolveSession(ctx context.Context, hash []byte) (Session, error) {
	if s.role != "api" {
		return Session{}, ErrUnsafeRole
	}
	if !validHash(hash) {
		return Session{}, identity.ErrUnauthenticated
	}
	return resolveSession(ctx, s.pool, hash)
}

// CompleteLogin admits only provider-verified facts. New users receive no
// memberships or permissions. Credentials are hashes; raw tokens are not stored.
func (s *Store) CompleteLogin(ctx context.Context, a identity.Authentication, hash, previousHash []byte, startedAt time.Time, requestID, correlationID string) (session Session, err error) {
	if s.role != "api" {
		return session, ErrUnsafeRole
	}
	if !validHash(hash) || (len(previousHash) != 0 && !validHash(previousHash)) || !validID(requestID) || !validID(correlationID) {
		return session, ErrInvalid
	}
	if !a.EmailVerified || a.AuthenticatedAt.IsZero() || startedAt.IsZero() {
		return session, identity.ErrUnauthenticated
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return session, fault("begin", err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	var accepted []byte
	err = tx.QueryRow(ctx, `SELECT identity.complete_login($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, a.Provider, a.Issuer, a.Subject, a.EmailVerified, a.AuthenticatedAt, a.Assurance.Level, a.Assurance.Evidence, hash, previousHash, startedAt, requestID, correlationID).Scan(&accepted)
	if err != nil {
		return session, fault("login_complete", err)
	}
	if len(accepted) == 0 {
		return session, identity.ErrUnauthenticated
	}
	session, err = resolveSession(ctx, tx, accepted)
	if err != nil {
		return Session{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Session{}, fault("commit", err)
	}
	return session, nil
}

func (s *Store) RevokeSession(ctx context.Context, hash []byte, all bool, requestID, correlationID string) error {
	if s.role != "api" {
		return ErrUnsafeRole
	}
	if !validHash(hash) {
		return identity.ErrUnauthenticated
	}
	if !validID(requestID) || !validID(correlationID) {
		return ErrInvalid
	}
	var revoked bool
	if err := s.pool.QueryRow(ctx, `SELECT identity.revoke_session($1,$2,$3,$4)`, hash, all, requestID, correlationID).Scan(&revoked); err != nil {
		return fault("session_revoke", err)
	}
	if !revoked {
		return identity.ErrUnauthenticated
	}
	return nil
}

func (s *Store) protectedSessionTX(ctx context.Context, hash []byte, workspace, permission string) (pgx.Tx, Session, error) {
	if s.role != "api" {
		return nil, Session{}, ErrUnsafeRole
	}
	if !validHash(hash) {
		return nil, Session{}, identity.ErrUnauthenticated
	}
	if !validID(workspace) {
		return nil, Session{}, ErrNotFound
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, Session{}, fault("begin", err)
	}
	session, err := resolveSession(ctx, tx, hash)
	if err != nil {
		return nil, Session{}, errors.Join(err, rollback(tx))
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.actor_id',$1,true),set_config('app.workspace_id',$2,true),set_config('app.session_hash',$3,true)`, session.Principal.UserID, workspace, hex.EncodeToString(hash)); err != nil {
		return nil, Session{}, errors.Join(fault("tenant_context", err), rollback(tx))
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT organizations.authorize_workspace($1)`, permission).Scan(&allowed); err != nil {
		return nil, Session{}, errors.Join(fault("authorize", err), rollback(tx))
	}
	if !allowed {
		return nil, Session{}, errors.Join(ErrNotFound, rollback(tx))
	}
	return tx, session, nil
}

func acceptSession(ctx context.Context, tx pgx.Tx, hash []byte) error {
	var accepted bool
	if err := tx.QueryRow(ctx, `SELECT identity.accept_session($1)`, hash).Scan(&accepted); err != nil {
		return fault("session_accept", err)
	}
	if !accepted {
		return identity.ErrUnauthenticated
	}
	return nil
}
