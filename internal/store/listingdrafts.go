package store

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/identity"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/listings"
)

func validateCreateDraft(c listings.CreateDraftCommand, personal bool) (listings.Fields, error) {
	if (!personal && !validID(c.WorkspaceID)) || (personal && c.WorkspaceID != "") ||
		!validID(c.CommandID) || !validID(c.RequestID) || !validID(c.CorrelationID) {
		return listings.Fields{}, ErrInvalid
	}
	f, err := c.Fields.Normalize()
	if err != nil {
		return listings.Fields{}, ErrInvalid
	}
	return f, nil
}

// recheckDraftAuthorization runs after any resource/receipt wait. Expiration
// uses PostgreSQL's current clock, not the transaction start time. Initial
// session/account/membership locks remain held, ordering revocation against work.
func recheckDraftAuthorization(ctx context.Context, tx pgx.Tx, hash []byte, permission identity.Permission) (Session, error) {
	session, err := resolveSession(ctx, tx, hash)
	if err != nil {
		return Session{}, err
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT listings.authorize_context($1)`, string(permission)).Scan(&allowed); err != nil {
		return Session{}, fault("draft_authorize", err)
	}
	if !allowed {
		return Session{}, ErrNotFound
	}
	return session, nil
}

func (s *Store) draftSessionTX(ctx context.Context, hash []byte, workspace string, permission identity.Permission) (pgx.Tx, Session, error) {
	tx, _, err := s.protectedSessionTX(ctx, hash, workspace, string(identity.ListingReadPrivate))
	if err != nil {
		return nil, Session{}, err
	}
	session, err := recheckDraftAuthorization(ctx, tx, hash, permission)
	if err != nil {
		return nil, Session{}, errors.Join(err, rollback(tx))
	}
	return tx, session, nil
}

// CreateListingDraftSession creates distinct Business and Listing Draft records
// with their material audit and immutable retry receipt in the same transaction.
func (s *Store) CreateListingDraftSession(ctx context.Context, hash []byte, c listings.CreateDraftCommand) (result listings.CreatedDraft, err error) {
	f, err := validateCreateDraft(c, false)
	if err != nil {
		return result, err
	}
	tx, session, err := s.draftSessionTX(ctx, hash, c.WorkspaceID, identity.ListingCreate)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	return createDraft(ctx, tx, hash, session, c, f, "WORKSPACE")
}

// CreatePersonalListingDraftSession requests the approved personal bootstrap
// explicitly. The organizations-owned capability derives the owner from a live
// human session and never replenishes revoked grants on assignment reuse.
func (s *Store) CreatePersonalListingDraftSession(ctx context.Context, hash []byte, c listings.CreateDraftCommand) (result listings.CreatedDraft, err error) {
	if s.role != "api" {
		return result, ErrUnsafeRole
	}
	if !validHash(hash) {
		return result, identity.ErrUnauthenticated
	}
	f, err := validateCreateDraft(c, true)
	if err != nil {
		return result, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return result, fault("begin", err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	var workspace *string
	if err = tx.QueryRow(ctx, `SELECT organizations.personal_drafting_workspace($1,$2,$3)::text`, hash, c.RequestID, c.CorrelationID).Scan(&workspace); err != nil {
		return result, fault("personal_draft_bootstrap", err)
	}
	if workspace == nil {
		return result, identity.ErrUnauthenticated
	}
	c.WorkspaceID = *workspace
	session, err := recheckDraftAuthorization(ctx, tx, hash, identity.ListingCreate)
	if err != nil {
		return result, err
	}
	if session.Principal.Kind != identity.Human {
		return result, identity.ErrUnauthenticated
	}
	return createDraft(ctx, tx, hash, session, c, f, "PERSONAL")
}

func createDraft(ctx context.Context, tx pgx.Tx, hash []byte, session Session, c listings.CreateDraftCommand, f listings.Fields, scope string) (listings.CreatedDraft, error) {
	fingerprint, err := f.Fingerprint()
	if err != nil {
		return listings.CreatedDraft{}, ErrInvalid
	}
	// Hash collisions can serialize unrelated commands, but cannot merge their
	// outcomes: the receipt primary key retains the complete unambiguous scope.
	lockKey := session.Principal.UserID + "/" + scope + "/" + c.WorkspaceID + "/listing_draft.create/" + c.CommandID
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,1707))`, lockKey); err != nil {
		return listings.CreatedDraft{}, fault("draft_command_lock", err)
	}
	session, err = recheckDraftAuthorization(ctx, tx, hash, identity.ListingCreate)
	if err != nil {
		return listings.CreatedDraft{}, err
	}
	result := listings.CreatedDraft{WorkspaceID: c.WorkspaceID}
	var recordedHash []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,business_id::text,listing_id::text,business_version,listing_version
FROM listings.command_receipts WHERE actor_user_id=$1 AND context_kind=$2 AND workspace_id=$3 AND operation='listing_draft.create' AND command_id=$4`,
		session.Principal.UserID, scope, c.WorkspaceID, c.CommandID).Scan(&recordedHash, &result.BusinessID, &result.ListingID, &result.BusinessVersion, &result.ListingVersion)
	if err == nil {
		if !bytes.Equal(recordedHash, fingerprint[:]) {
			return listings.CreatedDraft{}, ErrConflict
		}
		result.Replayed = true
		if err = finishDraftTX(ctx, tx, hash, identity.ListingCreate); err != nil {
			return listings.CreatedDraft{}, err
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return listings.CreatedDraft{}, fault("draft_receipt_read", err)
	}
	result.BusinessID, err = newID()
	if err != nil {
		return listings.CreatedDraft{}, fault("draft_identifier", err)
	}
	result.ListingID, err = newID()
	if err != nil {
		return listings.CreatedDraft{}, fault("draft_identifier", err)
	}
	result.BusinessVersion, result.ListingVersion = 1, 1
	if _, err = tx.Exec(ctx, `INSERT INTO listings.businesses(id,workspace_id,private_label,activity_description,country,region) VALUES($1,$2,$3,$4,$5,$6)`,
		result.BusinessID, c.WorkspaceID, f.PrivateLabel, f.ActivityDescription, f.Country, f.Region); err != nil {
		return listings.CreatedDraft{}, fault("business_create", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO listings.drafts(id,workspace_id,business_id,title,description,sale_subject,transfer_structure,sale_context,sale_method) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		result.ListingID, c.WorkspaceID, result.BusinessID, f.Title, f.Description, f.SaleSubject, f.TransferStructure, f.SaleContext, f.SaleMethod); err != nil {
		return listings.CreatedDraft{}, fault("draft_create", err)
	}
	if err = appendDraftAudit(ctx, tx, session, c.WorkspaceID, "Business", result.BusinessID, "business.created", 0, 1, c.RequestID, c.CorrelationID); err != nil {
		return listings.CreatedDraft{}, err
	}
	if err = appendDraftAudit(ctx, tx, session, c.WorkspaceID, "ListingDraft", result.ListingID, "listing_draft.created", 0, 1, c.RequestID, c.CorrelationID); err != nil {
		return listings.CreatedDraft{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO listings.command_receipts(actor_user_id,context_kind,workspace_id,operation,command_id,request_hash,business_id,listing_id,business_version,listing_version)
VALUES($1,$2,$3,'listing_draft.create',$4,$5,$6,$7,1,1)`, session.Principal.UserID, scope, c.WorkspaceID, c.CommandID, fingerprint[:], result.BusinessID, result.ListingID); err != nil {
		return listings.CreatedDraft{}, fault("draft_receipt_append", err)
	}
	if err = finishDraftTX(ctx, tx, hash, identity.ListingCreate); err != nil {
		return listings.CreatedDraft{}, err
	}
	return result, nil
}

func appendDraftAudit(ctx context.Context, tx pgx.Tx, session Session, workspace, kind, target, action string, previous, version int64, request, correlation string) error {
	var business, listing *string
	switch kind {
	case "Business":
		business = &target
	case "ListingDraft":
		listing = &target
	default:
		return ErrInvalid
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit.aggregate_events(workspace_id,actor_user_id,actor_session_id,action,target_kind,business_id,listing_id,request_id,correlation_id,previous_version,resource_version,metadata)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,jsonb_build_object('previous_version',$10::bigint,'version',$11::bigint))`,
		workspace, session.Principal.UserID, session.Principal.SessionID, action, kind, business, listing, request, correlation, previous, version); err != nil {
		return fault("draft_audit_append", err)
	}
	return nil
}

func finishDraftTX(ctx context.Context, tx pgx.Tx, hash []byte, permission identity.Permission) error {
	if _, err := recheckDraftAuthorization(ctx, tx, hash, permission); err != nil {
		return err
	}
	if err := acceptSession(ctx, tx, hash); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		// Neither a transport error nor a canceled COMMIT proves rollback. Create
		// callers retry the same command key; PUT callers reconcile with GET.
		return errors.Join(ErrCommitUncertain, fault("draft_commit", err))
	}
	return nil
}

const draftSelect = `SELECT b.id::text,b.workspace_id::text,b.private_label,b.activity_description,b.country,b.region,b.version,b.created_at,b.updated_at,
d.id::text,d.workspace_id::text,d.business_id::text,d.status,d.title,d.description,d.sale_subject,d.transfer_structure,d.sale_context,d.sale_method,d.version,d.created_at,d.updated_at
FROM listings.drafts d JOIN listings.businesses b ON b.workspace_id=d.workspace_id AND b.id=d.business_id WHERE d.workspace_id=$1 AND d.id=$2`

func scanDraft(row pgx.Row) (d listings.Draft, err error) {
	b, l := &d.Business, &d.Listing
	err = row.Scan(&b.ID, &b.WorkspaceID, &b.PrivateLabel, &b.ActivityDescription, &b.Country, &b.Region, &b.Version, &b.CreatedAt, &b.UpdatedAt,
		&l.ID, &l.WorkspaceID, &l.BusinessID, &l.Status, &l.Title, &l.Description, &l.SaleSubject, &l.TransferStructure, &l.SaleContext, &l.SaleMethod, &l.Version, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return listings.Draft{}, ErrNotFound
	}
	if err != nil {
		return listings.Draft{}, fault("draft_read", err)
	}
	b.CreatedAt, b.UpdatedAt = b.CreatedAt.UTC(), b.UpdatedAt.UTC()
	l.CreatedAt, l.UpdatedAt = l.CreatedAt.UTC(), l.UpdatedAt.UTC()
	return d, nil
}

func (s *Store) ReadListingDraftSession(ctx context.Context, hash []byte, workspaceID, draftID string) (d listings.Draft, err error) {
	if !validID(draftID) {
		return d, ErrNotFound
	}
	tx, _, err := s.draftSessionTX(ctx, hash, workspaceID, identity.ListingReadPrivate)
	if err != nil {
		return d, err
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	d, err = scanDraft(tx.QueryRow(ctx, draftSelect, workspaceID, draftID))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if _, authErr := recheckDraftAuthorization(ctx, tx, hash, identity.ListingReadPrivate); authErr != nil {
				return listings.Draft{}, authErr
			}
		}
		return listings.Draft{}, err
	}
	if err = finishDraftTX(ctx, tx, hash, identity.ListingReadPrivate); err != nil {
		return listings.Draft{}, err
	}
	return d, nil
}

func (s *Store) PreviewListingDraftSession(ctx context.Context, hash []byte, workspaceID, draftID string) (listings.PrivatePreview, error) {
	d, err := s.ReadListingDraftSession(ctx, hash, workspaceID, draftID)
	if err != nil {
		return listings.PrivatePreview{}, err
	}
	return d.Preview(), nil
}

func (s *Store) UpdateListingDraftSession(ctx context.Context, hash []byte, c listings.UpdateDraftCommand) (result listings.Draft, err error) {
	if !validID(c.WorkspaceID) || !validID(c.DraftID) || !validID(c.RequestID) || !validID(c.CorrelationID) || c.ExpectedBusinessVersion < 1 || c.ExpectedListingVersion < 1 {
		return result, ErrInvalid
	}
	f, err := c.Fields.Normalize()
	if err != nil {
		return result, ErrInvalid
	}
	tx, session, err := s.draftSessionTX(ctx, hash, c.WorkspaceID, identity.ListingUpdate)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	// Explicit aggregate lock order: account, session, grants, Business, Draft.
	// business_id is immutable to the runtime role, so this lookup cannot relink.
	var businessID string
	if err = tx.QueryRow(ctx, `SELECT business_id::text FROM listings.drafts WHERE workspace_id=$1 AND id=$2`, c.WorkspaceID, c.DraftID).Scan(&businessID); errors.Is(err, pgx.ErrNoRows) {
		if _, authErr := recheckDraftAuthorization(ctx, tx, hash, identity.ListingUpdate); authErr != nil {
			return listings.Draft{}, authErr
		}
		return listings.Draft{}, ErrNotFound
	} else if err != nil {
		return listings.Draft{}, fault("draft_target", err)
	}
	var version int64
	if err = tx.QueryRow(ctx, `SELECT version FROM listings.businesses WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, c.WorkspaceID, businessID).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if _, authErr := recheckDraftAuthorization(ctx, tx, hash, identity.ListingUpdate); authErr != nil {
				return listings.Draft{}, authErr
			}
			return listings.Draft{}, ErrNotFound
		}
		return listings.Draft{}, fault("business_lock", err)
	}
	if _, err = recheckDraftAuthorization(ctx, tx, hash, identity.ListingUpdate); err != nil {
		return listings.Draft{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT version FROM listings.drafts WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, c.WorkspaceID, c.DraftID).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if _, authErr := recheckDraftAuthorization(ctx, tx, hash, identity.ListingUpdate); authErr != nil {
				return listings.Draft{}, authErr
			}
			return listings.Draft{}, ErrNotFound
		}
		return listings.Draft{}, fault("draft_lock", err)
	}
	session, err = recheckDraftAuthorization(ctx, tx, hash, identity.ListingUpdate)
	if err != nil {
		return listings.Draft{}, err
	}
	current, err := scanDraft(tx.QueryRow(ctx, draftSelect, c.WorkspaceID, c.DraftID))
	if err != nil {
		return listings.Draft{}, err
	}
	b, l := current.Business, current.Listing
	if b.Version != c.ExpectedBusinessVersion || l.Version != c.ExpectedListingVersion {
		return listings.Draft{}, ErrConflict
	}
	if b.PrivateLabel != f.PrivateLabel || b.ActivityDescription != f.ActivityDescription || b.Country != f.Country || b.Region != f.Region {
		if _, err = tx.Exec(ctx, `UPDATE listings.businesses SET private_label=$3,activity_description=$4,country=$5,region=$6,version=version+1,updated_at=clock_timestamp() WHERE workspace_id=$1 AND id=$2`, c.WorkspaceID, b.ID, f.PrivateLabel, f.ActivityDescription, f.Country, f.Region); err != nil {
			return listings.Draft{}, fault("business_update", err)
		}
		if err = appendDraftAudit(ctx, tx, session, c.WorkspaceID, "Business", b.ID, "business.updated", b.Version, b.Version+1, c.RequestID, c.CorrelationID); err != nil {
			return listings.Draft{}, err
		}
	}
	if l.Title != f.Title || l.Description != f.Description || l.SaleSubject != f.SaleSubject || l.TransferStructure != f.TransferStructure || l.SaleContext != f.SaleContext || l.SaleMethod != f.SaleMethod {
		if _, err = tx.Exec(ctx, `UPDATE listings.drafts SET title=$3,description=$4,sale_subject=$5,transfer_structure=$6,sale_context=$7,sale_method=$8,version=version+1,updated_at=clock_timestamp() WHERE workspace_id=$1 AND id=$2`, c.WorkspaceID, l.ID, f.Title, f.Description, f.SaleSubject, f.TransferStructure, f.SaleContext, f.SaleMethod); err != nil {
			return listings.Draft{}, fault("draft_update", err)
		}
		if err = appendDraftAudit(ctx, tx, session, c.WorkspaceID, "ListingDraft", l.ID, "listing_draft.updated", l.Version, l.Version+1, c.RequestID, c.CorrelationID); err != nil {
			return listings.Draft{}, err
		}
	}
	result, err = scanDraft(tx.QueryRow(ctx, draftSelect, c.WorkspaceID, c.DraftID))
	if err != nil {
		return listings.Draft{}, err
	}
	if err = finishDraftTX(ctx, tx, hash, identity.ListingUpdate); err != nil {
		return listings.Draft{}, err
	}
	return result, nil
}
