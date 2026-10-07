# Private Business and Listing Draft operations

Updated: 2026-10-07. **Approved backend scope; acceptance evidence must identify the actual source commit and executed checks.** This runbook covers the private draft slice in [ADR 0008](../adr/0008-private-listing-drafts.md), not production rollout. The original [proposal](../architecture/DRAFT-SLICE-PROPOSAL.md) remains historical planning; the [decision register](../architecture/DECISIONS.md) records its later approval.

## Capability and access boundary

The API persists separate workspace-owned Business and Listing Draft records and reads a protected Preview synchronously. Incomplete drafts are allowed; supplied text, country and independent sale dimensions must satisfy the current typed contract. Only `DRAFT` exists in this slice. There are no asking prices, financial figures, documents, photos, upload jobs, publication transitions, buyer grants, search results, seller-mandate approval or verified badges.

The isolated seller study stays disconnected. Its values and selected files are page-memory examples; they are not saved by these endpoints. There is no new client or development identity fallback. Do not use the study to test persistence or imply that its financial/report/photo fields are this backend contract.

Authentication configuration and session lifecycle remain in the [P0.4 identity runbook](P0.4-IDENTITY.md). A protected operation requires the real session boundary. `AUTHENTICATION_MODE=disabled` permits health-only operation and denies protected access; it cannot initialize a personal workspace. OIDC fixtures exist only in test composition. Actual provider acceptance, privacy/purpose/retention, controlled HTTPS ingress and named operating ownership remain gates before real-user use.

Permission checks use the current account/session, active workspace membership and explicit grants inside the resource transaction. Read/Preview require `listing.read_private`; create/update also require `listing.create`/`listing.update`. The requesting workspace ID is context, not proof. Managing ownership, organization membership, provider roles and caller-supplied actor/owner/role/permission fields do not bypass these checks. Existing workspace Reader/Editor grants are not expanded automatically.

Organization affiliation and workspace authorization also have independent lifecycles. Removing an organization relationship alone does not revoke a separately granted workspace membership or permission. Revoke the relevant workspace membership/grant to deny that access under the transaction ordering below; reuse/bootstrap cannot restore it. An already authorized transaction ordered before revocation may finish, while subsequent authorization sees the revocation. Professional offboarding and bulk-delegation policy/workflows remain deferred. Neither affiliation nor a workspace grant proves a SellerMandate.

## Explicit first personal draft

The personal-create command derives its User from the authenticated HUMAN session. On first initialization it atomically creates a PERSON-owned drafting Workspace, one personal drafting assignment, active membership and exactly these grants:

- `workspace.read`
- `workspace.update`
- `listing.read_private`
- `listing.create`
- `listing.update`

This is a default drafting context, not a restriction that a person may own only one Workspace. It creates neither an Organization nor authority to sell. An existing assignment is reused without reactivating membership or restoring removed grants. If current access has been revoked, bootstrap is not a repair or privilege-refill mechanism.

The account is locked before the bootstrap transaction resolves its session, avoiding concurrent shared-to-exclusive lock upgrades. Assignment uniqueness, resource creation, the exact grants, audit evidence and creation receipt must commit together. No external caller chooses the personal owner, grants or actor.

## Protected HTTP contract

The [versioned OpenAPI contract](../../api/openapi/listing-drafts.json) and [transport implementation](../../internal/httpapi/listingdrafts.go) compose through the existing identity handler. They provide only:

| Method/path | Behavior |
| --- | --- |
| `POST /api/v1/personal-listing-drafts` | Explicit personal bootstrap/reuse and Business/draft creation in one command. |
| `POST /api/v1/workspaces/{workspace_id}/listing-drafts` | Create in an existing currently authorized workspace. |
| `GET /api/v1/workspaces/{workspace_id}/listing-drafts/{draft_id}` | Read the current protected Business/draft state. |
| `PUT /api/v1/workspaces/{workspace_id}/listing-drafts/{draft_id}` | Replace the typed draft fields using both expected resource versions. |
| `GET /api/v1/workspaces/{workspace_id}/listing-drafts/{draft_id}/preview` | Read the explicit protected presentation and its source versions. |

Create requires a canonical lowercase UUID `command_id` in the body. The flat optional string fields are `private_label`, `activity_description`, `country`, `region`, `title`, `description`, `sale_subject`, `transfer_structure`, `sale_context`, `sale_method`. Empty means unset. PUT accepts that same field set plus positive `expected_business_version` and `expected_listing_version`; omitted optional fields are cleared, so send the complete desired replacement. PUT does not accept `command_id`.

The [domain contract](../../internal/listings/drafts.go) trims surrounding whitespace, normalizes CRLF/CR to LF in the two multiline descriptions, and uppercases country. Text limits are 120 characters for private label/region, 2,000 for activity description, 160 for title and 8,000 for listing description. Unsupported control characters reject; descriptions permit internal newline/tab. Country is empty or two ASCII letters after normalization: this checks code shape, not existence in an ISO country catalogue, launch eligibility or jurisdictional law.

Nonempty sale values are finite and independent:

| Field | Accepted drafting values |
| --- | --- |
| `sale_subject` | `legal_entity`, `operating_business`, `business_division`, `asset_package` |
| `transfer_structure` | `share_sale`, `business_transfer` |
| `sale_context` | `ordinary_sale`, `succession`, `restructuring`, `insolvency`, `other` |
| `sale_method` | `asking_price`, `negotiation`, `invitation_for_offers`, `time_limited_bidding` |

These values record intent only. In particular `time_limited_bidding` does not create an offer process/deadline and no auction value or auction engine exists. There is no legal sale-combination, seller-authority or publication eligibility check in this slice.

The default `MAX_REQUEST_BODY_BYTES` is 128KiB, sufficient for the maximum typed fields even when supplementary Unicode is encoded as JSON surrogate pairs. Operators may configure a smaller bound, which can reject otherwise valid content; keep the bound explicit and finite. Bodies use the existing bounded strict decoder: unknown, duplicate/case-variant keys, null/non-string draft values, malformed Unicode, trailing JSON and unauthorized actor/role/owner/state claims are rejected. Request/correlation attribution is generated server-side. Mutations retain the P0.4 cookie, Origin and CSRF boundary. There is no collection-list endpoint, public route or query-filter API.

Initial creation returns 201; an authorized identical create replay returns 200. Both return exactly `workspace_id`, `business_id`, `listing_id`, `business_version`, `listing_version`, `replayed`. The versions in this creation outcome remain 1; obtain current versions through GET, which returns distinct `business` and `listing` objects. The `listing_id` identifies `{draft_id}` in subsequent paths.

Preview returns only `listing_id`, the two source versions, constant status, activity description, country, region, title, description and the four sale dimensions. It excludes private label, ownership/membership/identity data and evidence references. Free text can itself identify a business; this projection is protected and is not certified anonymous. Authenticated responses from a recognized draft route carry `X-Robots-Tag: noindex, nofollow` as well as the existing `Cache-Control: no-store`. Earlier authentication/CSRF rejection remains a denial and is not a public representation.

Inaccessible and nonexistent resources share 404 `not_found`. Conflicting create content under the same command identity returns 409 `command_conflict`; stale update versions return 409 `version_conflict`. Invalid input, unauthenticated credentials, CSRF failure and unavailable dependencies preserve the existing deterministic identity/error contract. An operational failure joined with a conflict takes precedence over reporting a benign conflict.

## Creation retries and edits

Supply one durable command identity per logical creation and retain it until the outcome is resolved. The receipt is scoped to authenticated User, operation and requested existing workspace or explicit personal context. Reuse that identity only for the same normalized typed input. The same request replays the original immutable IDs and initial Business/Listing versions with a replay flag; different input conflicts. Use a separate GET for current state, which may have changed since creation.

A replay still checks the current session and scoped permissions. Receipt existence never grants access. A lost response during COMMIT is an uncertain outcome, not evidence that nothing was created. Retry creation with the same command identity and content after dependency recovery. Using a new identity can create another deliberate draft.

Full replacement `PUT` requires both expected Business and Listing versions. Compare those values with a current read; do not omit version checks or automatically overwrite a conflict. Both current versions are locked and compared before changes. Only changed aggregates advance their versions; an exact no-op accepts the session and returns current state without a material audit event. Empty supplied draft fields clear those optional values.

Updates have no creation receipt. If an update response is lost, read current state and reconcile before another change. A committed changed update followed by a retry with its old expected versions conflicts rather than duplicating the revision. An unchanged no-op retains its versions. Do not describe this as exactly-once HTTP delivery or treat a stale version as permission to discard another user's changes.

## Audit and asynchronous work

`audit.aggregate_events` records only the finite approved actions: `workspace.personal_drafting_initialized`, `business.created`, `business.updated`, `listing_draft.created`, `listing_draft.updated`. Target references are constrained to their Workspace, Business or ListingDraft resource and workspace. Each row records the derived HUMAN actor/session, server request/correlation IDs, database timestamp, SUCCESS result and previous/current resource versions. Metadata is exactly `previous_version` and `version`; business text, raw request bodies, credentials and files do not belong there.

First initialization records Workspace version 0→1 together with the exact bootstrap grants. Reuse emits no new initialization event. Business/draft creation is 0→1; their changed updates record their own versions. Audit append failure rolls back state and receipts. Runtime access is append-only; privileged database operators remain outside that guarantee. No new audit browsing/administration endpoint is exposed.

The earlier `audit.events`, WorkspaceNameChanged outbox and private Workspace revision worker remain their existing capability. Draft Preview reads authoritative data synchronously. This slice adds no draft outbox, consumer or fabricated event-processing claim. Do not enqueue draft events through the Workspace name-change contract, increase a Workspace version for listing edits, or expect the worker to reconstruct drafts. A later asynchronous feature needs its own approved real effect and versioned idempotent consumer.

The [additive migration](../../db/migrations/000003_listing_drafts.sql) introduces `listings.businesses`, `listings.drafts`, `listings.command_receipts`, `organizations.personal_drafting_assignments` and `audit.aggregate_events`. Composite foreign keys bind ownership/Business/resource references; the assignment references the same User's PERSON ownership. Deferred revision/audit constraints prevent committing a Business/draft revision without its corresponding evidence; the reverse check rejects evidence for a future revision that has not occurred. RLS and column-specific privileges protect runtime reads/writes; API privileges do not permit resource relinking, receipt mutation, ledger mutation or deletion. The organizations-owned `personal_drafting_workspace` capability performs the narrow explicit bootstrap, not general grant management.

The integrity/bootstrap SECURITY DEFINER functions use a fixed search path and explicit policies for the migration owner under FORCE RLS. They do not depend on that owner being a superuser or BYPASSRLS role. The API/worker must not inherit or use this owner; startup rejects table-owning runtime identities. Preserve function ownership and its corresponding owner policies together in any migration. A runtime-only restore that loses those policies is not an equivalent schema.

## Migration and local verification

Use the separately provisioned migration login and the existing checksummed runner. The additive draft migration is applied after the two historical migrations; never edit their applied SQL or enable migrations on API startup. The [Package A runbook](PACKAGE-A.md#migration-operations) and [tooling instructions](../../scripts/dev/TOOLING.md) describe local credential handling and disposable PostgreSQL.

From the repository root, after the pinned tools and local synthetic database have been prepared:

```powershell
. ./scripts/dev/Set-LocalTestEnvironment.ps1
go run ./cmd/migrate apply
./scripts/dev/Configure-TestRoles.ps1
./scripts/verify.ps1
```

These are operator commands, not evidence that they have passed. The environment loader does not print credentials. It requires the local ignored connection file and sets the three test DSNs plus `REQUIRE_INTEGRATION=1`. Verification requires the actual PostgreSQL database and restricted API/worker logins; a test run that skips database suites is insufficient. Do not run application processes with migration/admin credentials.

The API, worker and migration commands build independently. Preserve the existing authentication, process health, startup validation and graceful-shutdown configuration. The worker retains its separate login and owns no draft/bootstrap authority. API startup/readiness checks now reference the three required listings relations, and runtime-role validation also checks ownership of the listings schema's tables. Apply the new migration before starting the new API binary; an older database cannot satisfy those dependency checks. Never select an in-memory substitute or treat an authenticated operation as anonymous to conceal a schema failure.

Required evidence includes empty-database and prior-schema upgrade, runtime grants/RLS, cross-workspace reads/updates/Preview, organization-affiliation denial, account/session/grant revocation, concurrent bootstrap, identical and changed-key replay, expected-version conflicts, no-op behavior, rollback after audit failure, private-preview allowlisting and HTTP strict-input/CSRF/error behavior. Run applicable format, vet, unit, integration, race, migration, contract, dependency vulnerability, static security, secret and documentation gates from [QUALITY-GATES.md](QUALITY-GATES.md). Record their exact outcomes separately, then run committed clean-checkout CI. Existing Package A/P0.4 results do not verify these new changes.

## Recovery, retention and rollout limits

On migration COMMIT uncertainty, reconnect and inspect durable checksummed migration history before retrying. A repeat apply validates history and applies only missing migrations. There is no automatic destructive down migration. Reverting an application binary does not remove new drafts, grants, receipts or evidence; use a reviewed forward repair or separately approved restore procedure. Never reset a shared database as an application recovery shortcut.

On authorization failure, investigate the current session, active membership and exact workspace grants using approved operational authority. Do not reactivate an account or broaden an organization/workspace scope to make a request pass. A personal assignment with revoked grants intentionally remains unusable until an approved membership/grant-management capability restores legitimate access.

There is no automated receipt, draft or audit deletion/expiry workflow. Deleting creation receipts can invalidate durable retry guarantees. Privacy retention, erasure, legal hold and named operator procedures require explicit policy before real data is admitted; this package does not settle them by silently keeping data forever. Evidence uses bounded schema fields, but a digest/identifier is not itself an anonymization claim.

Protected Preview is private, no-store and non-indexable. Do not cache it in a public CDN, expose it as a public URL, use it as an eligibility/verification signal or share credentials in an attempt to emulate buyer access. Public publication, money, media/document security and real seller authority are later approved packages. No production AWS resources or actual provider tenant are provisioned here.
