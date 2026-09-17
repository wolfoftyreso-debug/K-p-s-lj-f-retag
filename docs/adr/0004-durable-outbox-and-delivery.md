# ADR 0004 — PostgreSQL outbox, leases and idempotent consumer

Date: 2026-09-17. Status: accepted engineering implementation of approved D04; no new AWS resources or transport selected for deployment.

## Decision and scope

The single material command in Package A updates a workspace name with an expected version. State, a bounded audit record and `WorkspaceNameChanged` outbox record commit in one PostgreSQL transaction. A failure before commit rolls the whole command back. A connection failure during commit can leave the client uncertain: re-read the authoritative version before deciding to retry; never infer that every returned transport error proves rollback. The event has workspace/version/audit/correlation references and an empty typed payload; it contains no old/new name, request body or credential.

A separately running worker claims durable rows using `FOR UPDATE SKIP LOCKED`, a unique lease token, database-clock expiry and bounded attempts. This is an explicit PostgreSQL dispatcher, not an in-memory broker or a silent SQS fallback. The SQS/DLQ AWS deployment from the target architecture is not implemented or claimed in this package. Introducing that transport later can reuse event IDs, version contracts and consumer idempotency without changing producer transactions.

The implemented consumer builds a private WorkspaceRevision projection containing only the latest version and last event ID. This is a real persisted processing effect, useful to inspect asynchronous progress, without introducing a marketplace feature. A unique `(consumer, event_id)` receipt and the projection update commit together. Updates are monotonic: a late older version cannot overwrite a newer one.

Acknowledgment is a separate step after consumer commit. A crash between commit and acknowledgment causes redelivery. The receipt prevents repeating the local effect; this does not promise exactly-once delivery. Claim, handler and acknowledgment are separately testable against real PostgreSQL.

## Failure semantics

Transient handler failures return to pending with bounded exponential retry and a stable error code. Unsupported/malformed events and exhausted attempts reach a visible `DEAD` state in the same durable outbox. Expired claims can be recovered, including repeated worker crashes. Lease-token fencing prevents an old worker from acknowledging another worker's claim. Cancellation leaves a recoverable lease rather than claiming success.

Recovery also handles pending rows whose attempt count exceeds a newly lowered retry budget. It first locks exhausted rows, then reads receipts in a fresh Read Committed statement snapshot; a concurrently committed receipt must not be missed. A receipt for this local transactional handler proves its effect completed even if the final attempt lost its acknowledgement, so recovery marks it delivered. Without that receipt, exhaustion is terminal. Transaction isolation is explicitly Read Committed.

No unrestricted replay API is introduced. Inspection uses the authorized operational database capability; a future replay workflow needs an approved administrative policy. External side effects and provider idempotency are not implemented and cannot inherit an exactly-once guarantee from this database receipt.

## Audit boundary

The append-only application ledger records actor, action, Workspace target, UTC occurrence time, request/correlation IDs, result, previous/current version and strictly constrained metadata. It does not copy the workspace label. The runtime API cannot update/delete audit records; the worker cannot rewrite the immutable event envelope. Privileged retention, tamper-evident archival and a general denied-action audit ledger require later policy and are not claimed.

## Alternatives

Dual writes after state commit fail atomicity and are rejected. An in-memory queue cannot prove crash recovery and is rejected. Deploying a broker for local foundation tests is unnecessary and outside this package. A PostgreSQL dispatcher is bounded and replaceable; its capacity has not been certified for the eventual marketplace workload.

## Release review addendum — uncertain consumer commit

The Package A release review identified a distinct failure window before delivery acknowledgement: PostgreSQL can commit the receipt and projection but the consumer can lose the `COMMIT` response. Treating that transport failure as a normal handler failure could mark an already completed final attempt `DEAD`.

Consumer commit errors now carry the explicit `ErrCommitUncertain` classification. Processing preserves the durable lease and returns the error; it does not record success or failure from that uncertain response. After lease expiry, the existing locked recovery checks the durable receipt: committed effects become `DELIVERED`, while an exhausted attempt without a committed receipt becomes `DEAD`. Earlier attempts remain eligible for normal leased redelivery and deduplication.

The regression tests inject an error at the commit boundary against real PostgreSQL. One first commits the actual receipt and projection, then substitutes a lost-response error; the other returns the error before committing, allowing the real transaction to roll back. Both exercise the final-attempt processing and recovery path. They prove the handling of both possible durable outcomes; they do not claim to reproduce a physical network interruption or database-server crash.
