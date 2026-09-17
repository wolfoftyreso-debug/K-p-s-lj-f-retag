package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type WorkerOptions struct {
	LeaseDuration, RetryBase time.Duration
	MaxAttempts              int
}

func DefaultWorkerOptions() WorkerOptions {
	return WorkerOptions{LeaseDuration: 30 * time.Second, RetryBase: time.Second, MaxAttempts: 5}
}

type Processor struct {
	store   *Store
	options WorkerOptions
	// The default commits the real PostgreSQL transaction. This narrow boundary
	// also permits tests to inject a lost response after that commit completed.
	commitConsumer func(context.Context, pgx.Tx) error
}

func NewProcessor(s *Store, o WorkerOptions) (*Processor, error) {
	if s == nil || s.role != "worker" {
		return nil, ErrUnsafeRole
	}
	if o.LeaseDuration < time.Second || o.LeaseDuration > 5*time.Minute || o.RetryBase < time.Millisecond || o.RetryBase > time.Minute || o.MaxAttempts < 1 || o.MaxAttempts > 20 {
		return nil, ErrInvalid
	}
	return &Processor{store: s, options: o, commitConsumer: func(ctx context.Context, tx pgx.Tx) error {
		return tx.Commit(ctx)
	}}, nil
}

type delivery struct {
	ID, WorkspaceID, LeaseToken, EventType string
	Version                                int64
	SchemaVersion, Attempts                int
	Payload                                []byte
}

func (p *Processor) claim(ctx context.Context) (d delivery, found bool, err error) {
	token, err := newID()
	if err != nil {
		return d, false, err
	}
	tx, err := p.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return d, false, fault("claim_begin", err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	// Lock first, then read receipts in a NEW Read Committed statement snapshot.
	// A consumer can commit its receipt just before our row lock is acquired; a
	// receipt joined in the locking statement could miss that committed effect.
	exhaustedRows, err := tx.Query(ctx, `SELECT id::text FROM eventing.outbox
 WHERE attempts >= $1 AND (status='PENDING' OR (status='PROCESSING' AND leased_until<=clock_timestamp()))
 ORDER BY leased_until,id FOR UPDATE SKIP LOCKED LIMIT 100`, p.options.MaxAttempts)
	if err != nil {
		return d, false, fault("lease_recovery_lock", err)
	}
	var exhaustedIDs []string
	for exhaustedRows.Next() {
		var id string
		if err = exhaustedRows.Scan(&id); err != nil {
			exhaustedRows.Close()
			return d, false, fault("lease_recovery_lock", err)
		}
		exhaustedIDs = append(exhaustedIDs, id)
	}
	err = exhaustedRows.Err()
	exhaustedRows.Close()
	if err != nil {
		return d, false, fault("lease_recovery_lock", err)
	}
	// A committed receipt proves completion for this local handler (lost ACK).
	// An exhausted pending retry or expired lease without it becomes terminal.
	if len(exhaustedIDs) > 0 {
		_, err = tx.Exec(ctx, `WITH exhausted AS (
 SELECT o.id,o.status AS previous_status,r.processed_at FROM eventing.outbox o
 LEFT JOIN eventing.consumer_receipts r ON r.event_id=o.id AND r.consumer='workspace_revision_v1'
 WHERE o.id=ANY($1::uuid[]))
UPDATE eventing.outbox o SET status=CASE WHEN e.processed_at IS NULL THEN 'DEAD' ELSE 'DELIVERED' END,
 delivered_at=e.processed_at,lease_token=NULL,leased_until=NULL,
 last_error_code=CASE WHEN e.processed_at IS NOT NULL THEN NULL WHEN e.previous_status='PENDING' THEN 'processing_failed' ELSE 'lease_expired' END
FROM exhausted e WHERE o.id=e.id`, exhaustedIDs)
		if err != nil {
			return d, false, fault("lease_recovery", err)
		}
	}
	err = tx.QueryRow(ctx, `WITH candidate AS (
 SELECT id FROM eventing.outbox WHERE attempts < $1 AND
 ((status='PENDING' AND available_at<=clock_timestamp()) OR (status='PROCESSING' AND leased_until<=clock_timestamp()))
 ORDER BY available_at,occurred_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
UPDATE eventing.outbox o SET status='PROCESSING',attempts=attempts+1,lease_token=$2,
 leased_until=clock_timestamp()+($3::bigint*interval '1 millisecond')
FROM candidate c WHERE o.id=c.id
RETURNING o.id::text,o.workspace_id::text,o.lease_token::text,o.event_type,o.aggregate_version,o.schema_version,o.attempts,o.payload`,
		p.options.MaxAttempts, token, p.options.LeaseDuration.Milliseconds()).Scan(&d.ID, &d.WorkspaceID, &d.LeaseToken, &d.EventType, &d.Version, &d.SchemaVersion, &d.Attempts, &d.Payload)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.Commit(ctx); err != nil {
			return d, false, fault("claim_commit", err)
		}
		return d, false, nil
	}
	if err != nil {
		return d, false, fault("claim", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return d, false, fault("claim_commit", err)
	}
	return d, true, nil
}

func validDelivery(d delivery) bool {
	if d.EventType != "WorkspaceNameChanged" || d.SchemaVersion != 1 || d.Version < 2 {
		return false
	}
	var payload map[string]json.RawMessage
	return json.Unmarshal(d.Payload, &payload) == nil && payload != nil && len(payload) == 0
}

func (p *Processor) consume(ctx context.Context, d delivery) (err error) {
	if !validDelivery(d) {
		return ErrInvalidEvent
	}
	tx, err := p.store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fault("consume_begin", err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	var held bool
	err = tx.QueryRow(ctx, `SELECT true FROM eventing.outbox WHERE id=$1 AND lease_token=$2 AND status='PROCESSING' AND leased_until>clock_timestamp() FOR UPDATE`, d.ID, d.LeaseToken).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return fault("lease_check", err)
	}
	result, err := tx.Exec(ctx, `INSERT INTO eventing.consumer_receipts(consumer,event_id,workspace_id)
VALUES('workspace_revision_v1',$1,$2) ON CONFLICT (consumer,event_id) DO NOTHING`, d.ID, d.WorkspaceID)
	if err != nil {
		return fault("receipt_append", err)
	}
	if result.RowsAffected() == 1 {
		_, err = tx.Exec(ctx, `INSERT INTO eventing.workspace_revisions(workspace_id,version,last_event_id)
VALUES($1,$2,$3) ON CONFLICT (workspace_id) DO UPDATE SET version=EXCLUDED.version,last_event_id=EXCLUDED.last_event_id,updated_at=clock_timestamp()
WHERE eventing.workspace_revisions.version<EXCLUDED.version`, d.WorkspaceID, d.Version, d.ID)
		if err != nil {
			return fault("projection_update", err)
		}
	}
	if err = p.commitConsumer(ctx, tx); err != nil {
		// The server may have committed both receipt and effect. Preserve the
		// lease so recovery can determine the durable outcome, including on the
		// final attempt; recording failure here could incorrectly mark it DEAD.
		return errors.Join(ErrCommitUncertain, fault("consume_commit", err))
	}
	return nil
}

func (p *Processor) acknowledge(ctx context.Context, d delivery) error {
	result, err := p.store.pool.Exec(ctx, `UPDATE eventing.outbox SET status='DELIVERED',delivered_at=clock_timestamp(),lease_token=NULL,leased_until=NULL,last_error_code=NULL
WHERE id=$1 AND lease_token=$2 AND status='PROCESSING'`, d.ID, d.LeaseToken)
	if err != nil {
		return fault("acknowledge", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (p *Processor) fail(ctx context.Context, d delivery, cause error) error {
	status, code := "PENDING", "processing_failed"
	if errors.Is(cause, ErrInvalidEvent) {
		status, code = "DEAD", "invalid_event"
	}
	if d.Attempts >= p.options.MaxAttempts {
		status = "DEAD"
	}
	delay := p.options.RetryBase * time.Duration(1<<uint(d.Attempts-1))
	if delay > time.Minute {
		delay = time.Minute
	}
	result, err := p.store.pool.Exec(ctx, `UPDATE eventing.outbox SET status=$3,last_error_code=$4,lease_token=NULL,leased_until=NULL,
available_at=clock_timestamp()+($5::bigint*interval '1 millisecond') WHERE id=$1 AND lease_token=$2 AND status='PROCESSING'`, d.ID, d.LeaseToken, status, code, delay.Milliseconds())
	if err != nil {
		return fault("failure_record", err)
	}
	if result.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

// ProcessNext performs durable at-least-once delivery. Receipt + projection commit
// precedes acknowledgement deliberately: a crash in between safely repeats delivery.
// No network/external effect is performed by the Package A handler.
func (p *Processor) ProcessNext(ctx context.Context) (bool, error) {
	d, found, err := p.claim(ctx)
	if err != nil || !found {
		return found, err
	}
	if err = p.consume(ctx, d); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrLeaseLost) || errors.Is(err, ErrCommitUncertain) {
			return true, err
		}
		return true, errors.Join(err, p.fail(ctx, d, err))
	}
	return true, p.acknowledge(ctx, d)
}

// ProcessNext is a convenience for processes using the documented default retry policy.
func (s *Store) ProcessNext(ctx context.Context) (bool, error) {
	p, err := NewProcessor(s, DefaultWorkerOptions())
	if err != nil {
		return false, err
	}
	return p.ProcessNext(ctx)
}

type QueueStats struct {
	Pending, Processing, Delivered, Dead int64
	OldestPendingSeconds                 float64
}

func (s *Store) QueueStats(ctx context.Context) (q QueueStats, err error) {
	if s.role != "worker" {
		return q, ErrUnsafeRole
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='PENDING'),count(*) FILTER(WHERE status='PROCESSING'),
count(*) FILTER(WHERE status='DELIVERED'),count(*) FILTER(WHERE status='DEAD'),
coalesce(extract(epoch FROM (clock_timestamp()-min(occurred_at) FILTER(WHERE status IN ('PENDING','PROCESSING')))),0)::double precision
FROM eventing.outbox`).Scan(&q.Pending, &q.Processing, &q.Delivered, &q.Dead, &q.OldestPendingSeconds)
	if err != nil {
		return q, fault("queue_stats", err)
	}
	return q, nil
}
