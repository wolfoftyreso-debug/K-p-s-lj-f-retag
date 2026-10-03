package store

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
)

// These compatibility adapters exist only in test binaries. The historical
// Package A scenarios now run through real seeded PostgreSQL sessions. No
// production operation accepts a caller-asserted actor identifier.
func fixtureSessionHash(user string) []byte { h := sha256.Sum256([]byte(user)); return h[:] }

type fixtureWorkspaceCommand struct {
	ActorID string
	UpdateWorkspaceNameCommand
}

func fixtureError(err error) error {
	if errors.Is(err, identity.ErrUnauthenticated) {
		return ErrNotFound
	}
	return err
}
func (s *Store) ReadWorkspace(ctx context.Context, actor, workspace string) (Workspace, error) {
	w, err := s.ReadWorkspaceSession(ctx, fixtureSessionHash(actor), workspace)
	return w, fixtureError(err)
}
func (s *Store) UpdateWorkspaceName(ctx context.Context, c fixtureWorkspaceCommand) (Workspace, error) {
	w, err := s.UpdateWorkspaceNameSession(ctx, fixtureSessionHash(c.ActorID), c.UpdateWorkspaceNameCommand)
	return w, fixtureError(err)
}
func (s *Store) protectedTX(ctx context.Context, actor, workspace, permission string) (pgx.Tx, error) {
	tx, _, err := s.protectedSessionTX(ctx, fixtureSessionHash(actor), workspace, permission)
	return tx, fixtureError(err)
}
