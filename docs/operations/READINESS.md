# Operational readiness design

Date: 2026-09-16. Status: proposed operational contract. **Nothing in this file asserts infrastructure has been provisioned or a recovery drill has run.**

## Infrastructure ownership and reproducibility

The directive selects AWS and Terraform. D06 must establish account ownership, permitted regions, network/ingress topology, state backend and environment boundaries before infrastructure implementation. D11 must establish budget and recovery/availability objectives. Environment configuration is reviewed alongside code; manual production resources are not the normal provisioning method.

Proposed controls: isolated production/nonproduction accounts; private database and task networking; narrowly scoped ingress; distinct API, worker, migration and deployment IAM; encrypted storage, transport and Terraform state; secrets obtained through workload identity; RDS Multi-AZ for production; S3 public access block for protected storage and private public-media origin with authorized CDN delivery; SQS queue policies and DLQs; KMS ownership/rotation/restore considerations; deletion protection where approved. Exact choices, backup locations and costs require a concrete Terraform plan under D06/D08/D11.

Terraform bootstrap is itself reproducible code with a recorded procedure. State access can expose sensitive data and needs least privilege, versioning and locking. Never commit state or plans containing secrets. A successful plan does not authorize an apply to an unidentified account. Use account/region/environment assertions to prevent a nonproduction command targeting production.

## Observability and failure budgets

Structured logs and OpenTelemetry traces/metrics correlate request → command → outbox → consumer using non-sensitive IDs. Instrument the application boundaries and provider calls; redact before export. No raw bodies, tokens, signed URLs, protected filenames, financial evidence or high-cardinality personal labels. Audit is durable application evidence, separate from sampled telemetry.

| Signal | Why it matters | Required action contract |
| --- | --- | --- |
| API availability, error rate, latency and saturation | Customer-visible service health | Approved SLO window/burn thresholds and named on-call route |
| Database connection/lock pressure and query latency | Detect transaction failure or capacity bottleneck | Investigate bounded query plans, connection pools and migration impact |
| Oldest outbox record, dispatch lag, retry and DLQ counts | Detect lost progress despite successful writes | Alert, diagnose poison item vs dependency failure, audited controlled replay |
| Document quarantine age and failed scans | Detect unavailable or unsafe document pipeline | Keep quarantine closed; no operational “skip scan” toggle |
| Authorization denials and privileged operations | Detect misuse, tenancy errors or compromised accounts | Bounded security triage; don't emit sensitive target contents |
| Webhook verification failures, processing lag, entitlement anomalies | Detect billing integrity failures | Verify provider state and replay safely; do not hand-edit entitlements |
| Public projection lag and revocation failures | Detect stale discovery or confidentiality exposure | Enforce current eligibility and approved cache behavior; escalate exposure |
| Notification delivery failures, suppression and duplicates | Detect user-impacting message issues | Respect current consent/preferences; distinguish retry from duplicate delivery |
| Storage growth, task utilization and spend | Detect leaks, attack load and uncontrolled cost | Approved budgets/alerts; capacity change review |

Numerical thresholds are not invented in Phase 0 design. Each production alarm requires a defined metric, threshold, observation window, owner, runbook and tested notification route. Absence of traffic is not evidence of availability. Synthetic health checks must exercise safe meaningful boundaries.

Readiness checks include dependencies required to serve the corresponding capability; liveness answers whether the process should restart. Do not restart every task solely because the database is temporarily down. Shutdown stops new work, drains bounded requests, and safely relinquishes/retries worker leases.

## Release and rollback

Each release records Git commit, locked dependencies, build provenance, image digests, SBOM, schema migration set, configuration version, approved infrastructure plan, applicable test results and deploy actor. Promote the same scanned artifacts across environments. Protect migrations and production deployment with the relevant repository/environment rules and short-lived identity.

Use backward-compatible schema expansion before application rollout; backfill with observed progress and bounded load; remove old schema only after consumers are migrated. Roll back to a compatible previous artifact if health gates fail. Do not automatically run destructive SQL “down” scripts. A database restore can lose accepted writes and requires incident-level reconciliation against approved RPO.

Future runbooks must cover at least: deploy/rollback, database migration failure, DB outage/failover, outbox/DLQ backlog and replay, scanner failure/malicious upload, access revocation/data exposure, identity-provider outage, payment reconciliation, cache withdrawal, and account/secret compromise. Each has prerequisites, diagnostic steps, containment, recovery, owner and exit criteria.

## Backup and restore acceptance

RDS Multi-AZ is not a backup or a tested recovery process. D11 defines RPO/RTO, outage scenarios and drill cadence; D08 defines retention, erasure and legal holds; D06 defines backup regions and account isolation. Back up authoritative database, required object versions, configuration/state and encryption-key dependencies under their approved policies. Search can be rebuilt; state, documents and process evidence may not be reconstructible.

The acceptance drill must:

1. Select a recovery point and record incident clock and expected recoverable data boundary.
2. Restore database and needed objects/configuration into an isolated environment with working key access.
3. Verify schema version, integrity constraints, document version/hash references, audit continuity and ownership permissions using real runtime roles.
4. Apply deletion, revocation and legal-hold reconciliation before serving or dispatching anything. Suppress restored pending email/payment work until deduplication and external state are reconciled.
5. Rebuild public/search projections only from current eligible data; test that revoked/confidential content stays absent.
6. Reconcile Stripe and other external effects; resume outbox/consumers through deliberate, audited procedures.
7. Exercise meaningful application checks, measure data loss and restore duration, compare with approved RPO/RTO and record gaps/remediation.

No drill is marked passed based on a snapshot existing or a database accepting TCP connections.

## Incident and privacy readiness

Name the service owner, on-call responder, security incident commander, privacy/legal contact and backup decision-maker before production. Define severity, escalation, evidence preservation, user/provider communication and applicable reporting decisions with qualified owners. This document does not invent legal notification deadlines or a contact address.

Access to production support data is least privilege, time-bounded and audited under D12. Emergency access must identify an actor, purpose, scope, review and revocation; it is not a permanent cross-tenant superuser endpoint. Keep evidence narrowly scoped and protect it from accidental publication in the public source repository.

## Release checklist status

| Required acceptance | Current status |
| --- | --- |
| Approved region/accounts, budget, SLO/RPO/RTO, named operators | Pending owner decisions |
| Reproducible Terraform, private storage/network, least-privilege workload identities | Not implemented |
| Commit-linked deploy, verified CI/security gates and artifact promotion | Not implemented |
| Working telemetry, monitored DLQ, tested actionable alarms | Not implemented |
| Successful measured restore and rollback exercises | Not performed |
| Real auth/authorization, document and public-disclosure assurance | Not implemented |
| Privacy/retention/legal-hold decisions and data handling procedures | Pending owner decisions |
| Production launch authorization | Outside this documentation delivery |
