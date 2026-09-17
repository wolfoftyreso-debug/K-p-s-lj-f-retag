package store

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type Workspace struct {
	ID                  string    `json:"id"`
	OwnerKind           string    `json:"owner_kind"`
	OwnerUserID         *string   `json:"owner_user_id,omitempty"`
	OwnerOrganizationID *string   `json:"owner_organization_id,omitempty"`
	Name                string    `json:"name"`
	Version             int64     `json:"version"`
	UpdatedAt           time.Time `json:"updated_at"`
}
type UpdateWorkspaceNameCommand struct {
	ActorID, WorkspaceID, Name string
	ExpectedVersion            int64
	RequestID, CorrelationID   string
}

func validName(name string) bool {
	if !utf8.ValidString(name) || strings.TrimSpace(name) != name || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 120 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func scanWorkspace(row pgx.Row) (Workspace, error) {
	var w Workspace
	err := row.Scan(&w.ID, &w.OwnerKind, &w.OwnerUserID, &w.OwnerOrganizationID, &w.Name, &w.Version, &w.UpdatedAt)
	w.UpdatedAt = w.UpdatedAt.UTC()
	return w, err
}

const workspaceColumns = `id::text,owner_kind,owner_user_id::text,owner_organization_id::text,name,version,updated_at`

func (s *Store) ReadWorkspace(ctx context.Context, actorID, workspaceID string) (w Workspace, err error) {
	tx, err := s.protectedTX(ctx, actorID, workspaceID, "workspace.read")
	if err != nil {
		return w, err
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	w, err = scanWorkspace(tx.QueryRow(ctx, `SELECT `+workspaceColumns+` FROM organizations.workspaces WHERE id=$1`, workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fault("workspace_read", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Workspace{}, fault("commit", err)
	}
	return w, nil
}

// UpdateWorkspaceName is the single material command in Package A. The mutation,
// bounded audit evidence and minimized outbox envelope share one transaction.
func (s *Store) UpdateWorkspaceName(ctx context.Context, c UpdateWorkspaceNameCommand) (w Workspace, err error) {
	if !validName(c.Name) || c.ExpectedVersion < 1 || !validID(c.RequestID) || !validID(c.CorrelationID) {
		return w, ErrInvalid
	}
	tx, err := s.protectedTX(ctx, c.ActorID, c.WorkspaceID, "workspace.update")
	if err != nil {
		return w, err
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	// SELECT policy additionally requires read permission. Update alone cannot reveal a resource.
	w, err = scanWorkspace(tx.QueryRow(ctx, `SELECT `+workspaceColumns+` FROM organizations.workspaces WHERE id=$1 FOR UPDATE`, c.WorkspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fault("workspace_lock", err)
	}
	if w.Version != c.ExpectedVersion {
		return Workspace{}, ErrConflict
	}
	auditID, err := newID()
	if err != nil {
		return Workspace{}, err
	}
	eventID, err := newID()
	if err != nil {
		return Workspace{}, err
	}
	w, err = scanWorkspace(tx.QueryRow(ctx, `UPDATE organizations.workspaces SET name=$2,version=version+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING `+workspaceColumns, c.WorkspaceID, c.Name))
	if err != nil {
		return Workspace{}, fault("workspace_update", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit.events(id,workspace_id,actor_user_id,action,target_kind,target_id,request_id,correlation_id,result,previous_version,resource_version,metadata)
VALUES($1,$2,$3,'workspace.name_updated','Workspace',$2,$4,$5,'SUCCESS',$6,$7,jsonb_build_object('previous_version',$6::bigint,'version',$7::bigint))`,
		auditID, c.WorkspaceID, c.ActorID, c.RequestID, c.CorrelationID, c.ExpectedVersion, w.Version)
	if err != nil {
		return Workspace{}, fault("audit_append", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO eventing.outbox(id,workspace_id,audit_event_id,aggregate_version,event_type,schema_version,payload,correlation_id)
VALUES($1,$2,$3,$4,'WorkspaceNameChanged',1,'{}'::jsonb,$5)`, eventID, c.WorkspaceID, auditID, w.Version, c.CorrelationID)
	if err != nil {
		return Workspace{}, fault("outbox_append", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Workspace{}, fault("commit", err)
	}
	return w, nil
}
