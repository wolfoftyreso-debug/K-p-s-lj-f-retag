# Phase 0 conceptual domain model

Status: architecture proposal for review. No application schema, state machine, legal workflow, or production resource is authorized by this document alone.

Source: the user's Phase 0 master directive. Terms are defined in [GLOSSARY.md](GLOSSARY.md). This model separates required constraints from design proposals and decisions that must be approved before dependent implementation.

## 1. Constraints established by the directive

1. Go modular monolith plus independently runnable asynchronous workers; PostgreSQL is authoritative. Search indexes and public discovery documents are projections.
2. Business, Listing, Organization, Seller, and User are distinct concepts. SaleSubject, TransferStructure, SaleContext, and SaleMethod are independent dimensions.
3. Application-owned, server-side authorization; authentication-provider details stay behind an adapter. Seller identity and authority to sell are separate.
4. Tenant ownership is explicit and protected access across Workspaces must be denied and tested. Opaque identifiers are never an authorization control.
5. Structured Listing records and financial provenance; country-specific extensions must not distort the universal core.
6. Explicit Listing transitions, protected documents, intentionally safe public representations, and durable audit of material actions.
7. State changes and events triggering asynchronous work commit through a transactional outbox. Delivery is at least once; consumers must be idempotent.
8. Money is exact and paired with currency. Timestamps are UTC; business deadlines preserve explicit timezone semantics. Language, country, currency, and jurisdiction remain separate.
9. Stripe initially pays for platform services only. Organic discovery and labelled paid placement remain separate. An auction or NDA workflow cannot be advertised as implemented before it exists.

## 2. Proposed bounded contexts and ownership

These are ownership boundaries inside a monolith, not a plan to deploy separate services or create empty package directories. A module is introduced when its behavior exists. Modules exchange explicit application contracts; one module must not directly update another module's tables.

| Context | Owns | Does not decide |
| --- | --- | --- |
| Identity | Internal User identifiers, authentication-subject mappings, account security state | Seller authority, arbitrary resource permissions |
| Organizations | Organizations, Workspaces, Memberships, scoped Role/Permission assignments | Listing publication, legal ownership of a Business |
| Listings | Listing content, versions, explicit transitions, sale dimensions, public representation eligibility | Identity-verification method, payment-provider truth, legal mandate sufficiency |
| Verification | Scoped claims, evidence references, verification outcomes; proposed home for SellerMandate evidence assessment | Global access merely because a person is verified |
| Documents | Document versions, quarantine/processing state, classifications, storage references, access decisions and grants | Legal effectiveness of an NDA or public publication of confidential files |
| Search | SearchPort, public search projections, normalized criteria and deterministic retrieval | Authoritative listing state, authorization, unlabelled paid ranking |
| Saved searches | SavedSearch lifecycle, normalized criteria, matching and alert preferences | Search-engine-specific query syntax as the persisted contract |
| Messaging | Inquiry, Conversation, explicit participation, communication state | Offer admissibility or settlement |
| Payments | Provider adapters, recorded provider events, platform payments and entitlements | Business acquisition settlement or escrow |
| Advertising | Promotions, advertisements, labelled placements, measured delivery | Organic relevance |
| Insolvency | Factual case/process records, authority references, OfferProcess/Offer evidence where approved | Legal findings, allegations, automatic participant disqualification |
| Audit | Durable intentional audit schema and append interface | General debugging log storage or unrestricted personal-data copies |
| Notifications | Delivery intentions, channel state, localization, deduplication and permitted recipient resolution | New grants of access to source data |

SellerMandate is first class regardless of its eventual module placement. The proposed assessment owner is Verification; Listings consumes an explicit mandate decision for a requested action. Ownership of mandate lifecycle and legal review remains a decision in section 11. SavedListing can initially be a small discovery capability; it does not require its own deployable service.

## 3. Protected ownership paths

Proposed tenant root: **Workspace**. The concrete recommendation for approval in [D02](../architecture/DECISIONS.md) is exactly one managing owner, a User or an Organization, represented by two typed foreign keys and an exclusive-or constraint. This accommodates individual sellers without a fictitious Organization. Managing ownership is not legal ownership of a Business or a determination of data-controller status. Organization affiliation and owner status do not bypass explicit active Workspace membership and permissions for internal workspace operations. Separately approved buyer/resource grants and User-owned personal resources follow their own bounded policy; they grant no general workspace access. This recommendation is not yet accepted. Ownership transfers and last-member/orphan handling need a reviewed workflow before being enabled.

| Resource | Proposed ownership path | Access implication |
| --- | --- | --- |
| Listing, Business record, SellerMandate | Workspace → resource | Authenticated actor must be authorized for this Workspace and action. A Business record is a workspace-controlled representation of a real-world operation, not a global cross-client record. |
| ListingVersion, ListingFinancial, ListingAsset, ListingMedia | Workspace → Listing → child | All identifiers and relationships must resolve within the same owning Workspace. |
| ListingDocument association | Workspace → Listing → association → owned document/version | Attaching a document from another Workspace must be impossible without a separately approved sharing mechanism. |
| Protected document/version and VerificationEvidence | Workspace → document/evidence, with explicit subject references | Membership alone may still be insufficient; classification, purpose, processing state, and approved policy apply. User-identity evidence requires a separately defined account ownership path. |
| InsolvencyCase and OfferProcess | Workspace → case/process → process records | Appointment, authority, participant access, and evidentiary visibility are separate from professional workspace membership. |
| Offer | OfferProcess → Offer, with recorded submitting party and actor | Submitter rights and process-administrator rights require a policy; ownership must not imply unrestricted visibility. |
| SavedListing, SavedSearch | User → resource; optional workspace scope only if approved | A personal saved item does not become readable to every colleague. Workspace-shared saved items require a distinct owner/access model. |
| Inquiry and Conversation | Defined communication owner/scope plus explicit participants | Cross-party participation is a bounded capability, not membership in the seller's entire Workspace. Exact ownership and retention need approval. |
| PublicListingProjection | Eligible approved ListingVersion → minimized public projection | Anonymous access applies only to the projection. No grant to the authoritative Listing or sibling documents follows. |
| Platform payments and entitlements | Explicit billing party/account → payment/entitlement → beneficiary | Payment actor, billed party, and Workspace beneficiary must be distinguishable. Billing model remains unresolved. |

Engineering proposal: carry `workspace_id` on tenant-owned records where it enables enforceable composite foreign keys and scoped query predicates. A child reference must not combine Workspace A with a parent in Workspace B. This is a storage design proposal, not an executed migration.

Repository/application methods should require actor/context and ownership scope for protected operations. A raw `GetByID(id)` must not bypass authorization at an HTTP handler or worker call site. Public reads use separate methods returning the public contract only.

PostgreSQL row-level security can be considered as an additional control after connection-pool and transaction-context behavior are tested. It must not substitute for the application authorization boundary. Whether to mandate RLS is an architecture decision, not silently assumed here.

Organization administrators, platform support staff, and background workers do not acquire unbounded cross-workspace permission by convention. Service identities, support access, impersonation, and exceptional intervention require explicit policy and audit.

## 4. Conceptual relational inventory

The following identifies structured responsibilities and invariants, not final SQL types, enum definitions, table names, or cardinalities.

| Conceptual record | Structured information to preserve | Invariant or unresolved boundary |
| --- | --- | --- |
| User / identity mapping | Internal User ID; provider and provider-subject binding; security status | Provider subject is not a domain primary key. Linking identities needs a controlled policy. |
| Workspace / Membership / assignment | Workspace identity; User association; status; scoped permission assignments | A role assignment is evaluated within an explicit scope. Inheritance and professional client separation need approval. |
| Business representation | Workspace; economic operation description; relevant legal-entity associations | Shared real-world identity is not permission to join confidential records across tenants. |
| SellerMandate | Represented party; actor/representative; subject scope; claimed authority; supporting evidence; assessment; validity/revocation | Identity checks never imply mandate approval. Required evidence and action gating depend on jurisdiction and policy. |
| Listing | Workspace; stable identity; Business reference where applicable; current lifecycle state; current content/version reference; sale axes; publication timestamps | Only explicit application transitions change lifecycle. Status does not encode payment or verification truth. |
| Listing content/version | Title; description; industry reference; location; sale dimensions; asking price; workforce; establishment; premises; seller involvement; disclosure configuration | Draft/edit/version relationships must be explicit. Material amendment and publication rules remain unresolved. |
| ListingFinancial | Metric code; exact amount/value; currency/unit; period boundaries; source type/reference; result definition; claim/verification reference | Revenue and result periods and sources cannot be inferred from display labels. Missing, zero, undisclosed, and not applicable must remain distinct. |
| ListingAsset | Structured asset description/category; included/excluded relation; relevant restrictions | Listing does not establish title. Asset categories and marketplace eligibility need product review. |
| ListingMedia | Private original reference; processing state; approved derivative reference; ordering; localized alternative text where applicable | Public derivatives require explicit release and sanitization. User filenames and original metadata must not become public identifiers. |
| Document / document version | Owning scope; classification; object reference; checksum; validated media type; size; processing state; retention/deletion metadata | Original file remains inaccessible until required checks pass. New file bytes produce a new version. |
| ListingDocument / access grant | Association; document version; disclosure policy; grantee; scope; validity/revocation; decision provenance | A signed URL is an output of an authorized decision, not the source of authorization. |
| Listing translation | Content/version reference; language tag; translated fields; provenance/status | Original and translated claims remain distinguishable; translation cannot silently change authoritative money or conditions. |
| PublicListingProjection | Source version; projection schema/version; approved public fields; eligibility state | Only explicitly eligible data may enter public HTML, metadata, APIs, image derivatives, caches, feeds, and search. |
| SavedSearch | Owner; normalized versioned criteria; name; notification preferences/status | URL is derived presentation. Editing criteria must define matching/deduplication consequences before alerts launch. |
| Payment event / entitlement | Provider/event identifier; validation outcome; processing state; semantic grant key; beneficiary | Unique provider-event handling and unique business entitlement issuance are both needed. Two provider events can describe one commercial transaction. |
| InsolvencyCase / OfferProcess | Jurisdiction; asserted appointment/authority references; process rules and versions; deadline semantics; releases; amendments; closure evidence | No binding effect, statutory compliance, or admissibility is inferred. Process changes require explicit authority. |
| AuditEvent / outbox | Stable event identity; schema version; actor/service; subject; ownership scope; time; outcome; correlation; minimized context | Audit and dispatch purposes differ. Required state, audit record, and outbox record should commit atomically for a material local change. |

Country extensions should be separately typed and owned modules/tables linked to the core entity and jurisdiction. JSON may hold deliberately bounded, schema-versioned extension data when justified; it must not become the unvalidated primary Listing model. A country's company identifier, a listing's display language, and a transaction's jurisdiction do not substitute for one another.

## 5. Listing versions, disclosure, and public content

Proposed content structure separates a stable Listing identity, editable draft content, immutable snapshots at approved versioning points, and public representations derived from eligible snapshots. This supports changes without rewriting historical evidence.

Before implementation, decide which edits are material, which need review or republishing, whether an already public version remains visible while an amendment is reviewed, and how translations, documents, and financial corrections attach to a version. A seller must not be able to replace historical process evidence by editing a shared row.

Public content is built with an allowlist. It must not include a protected LegalEntity name, registration identifier, precise location, original filename, EXIF/geolocation, source object key, document URL, evidence reference, internal notes, or protected financial source merely because those values exist in the authoritative record. Whether any such field may be public depends on approved disclosure policy and the seller's permitted release.

The same public contract governs server-rendered HTML, JSON responses, structured metadata, canonical/alternate URLs, sitemaps, social previews, search documents, notification previews, and media derivatives. Logs and traces must not serialize whole protected records.

The directive's disclosure examples are **not assumed to form a simple numeric ladder**. “Verified” requires a defined claim; “Approved” requires a defined approver and grant; “NDARequired” requires a real workflow. Approval, verification, and NDA requirements may be independent conditions. The policy composition, granularity, expiry, and revocation rules require approval.

Allowlisting alone cannot detect identities typed into a title, description, image, or PDF. Anonymous/confidential publication therefore also needs an approved content review and sanitization workflow; until it exists, the system cannot claim that arbitrary seller content is anonymous.

Private originals must not share public object access policies. A public photo derivative is distinct from its quarantined original. Document authorization must be checked against the current actor, resource version, effective grant, required conditions, and processing state before issuing a short-lived access mechanism.

Projection invalidation after withdrawal, confidentiality changes, revoked permissions, or deletion is security-sensitive. Delivery may lag, so a stale search hit or cache must never be sufficient authority for protected access. Public cache lifetime, purge guarantees, stale result behavior, and the precise meaning of withdrawal must be explicitly approved before production publication.

## 6. Lifecycle: candidate states, no invented transition policy

The directive lists possible states, not a fully approved directed graph. In particular, payment and review may be optional or ordered differently; `SOLD`, `WITHDRAWN`, and `EXPIRED` may have distinct terminal and historical behaviors. Do not implement a linear chain from the example arrows.

| Candidate state | Meaning to validate | Open rule |
| --- | --- | --- |
| DRAFT | Editable, unpublished content | Who can edit or collaborate; autosave/version policy |
| READY_FOR_REVIEW | Seller considers the submission ready | Completeness rules, mandate checks, next step |
| PENDING_PAYMENT | Publication or service awaits a platform entitlement | When payment is required; failure/refund handling |
| PENDING_REVIEW | Submission awaits an approved review process | Reviewer authority, SLA, rejection and appeal |
| PUBLISHED | Approved public representation is available | Publication prerequisites, payment/mandate dependencies |
| PAUSED | Seller/platform has suspended current exposure | What remains reachable/indexable; resumption conditions |
| UNDER_OFFER | Seller records an ongoing offer/deal stage | Evidence needed; public visibility and contact behavior |
| SOLD | Seller/platform records a completed sale outcome | Meaning of “sold,” verifier, correction and retention |
| WITHDRAWN | Offer has been withdrawn from current exposure | Permitted actor, reason, reversal and process consequences |
| EXPIRED | Approved publication term elapsed | Term source, renewal, scheduled job semantics |
| ARCHIVED | Retained historical state | Archive eligibility, access, deletion and restoration |

Before building transitions, produce an approved matrix with command, source/target states, permitted actor and scope, required mandate/entitlement/review, validations, visibility effects, audit event, domain event, idempotency behavior, and reversibility. No HTTP endpoint should accept an arbitrary replacement status.

Reversible engineering proposal: use explicit commands and optimistic concurrency around the authoritative Listing/version. For a successful material command, commit changed state, required audit record, and outbox event within one PostgreSQL transaction. On a version conflict return an explicit conflict rather than silently overwriting another user's update. Exact user-facing recovery behavior needs journey design.

Record unsuccessful security-relevant attempts through an intentional audit path even when the domain transaction rolls back. Do not emit a success event before commit. A repeated command must not cause duplicate publication side effects or duplicate commercial grants.

## 7. Search and saved search contracts

Proposed SearchPort accepts validated normalized criteria, locale/language context where relevant, explicit sort, and bounded pagination. It returns eligible public projections, stable identifiers/cursors, and transparent result grouping. It exposes no SQL fragments, OpenSearch DSL, private document contents, or protected Listing serialization.

The criteria vocabulary must be able to represent free text, industry, country, region, price, revenue, business size, TransferStructure, SaleContext, SaleMethod, digital/physical characteristics, verification claims, and recency. Product decisions must define multi-select logic, omitted/unknown values, financial comparison periods, exact metric definitions, text matching and ranking. Those decisions must be shared between search and saved-search matching.

Price ranges must include currency semantics. No implicit cross-currency numeric ordering or conversion is allowed. A range such as “100000–200000” without currency and unit is invalid. Any future reference conversion is separately labelled and never overwrites the seller's authoritative amount.

Deterministic ordering needs explicit tie-breakers and a stated consistency model during concurrent updates. A publication-time sort with stable ID as tie-breaker is a possible initial option, not an approved default relevance policy. PostgreSQL implementation must support the agreed contract before OpenSearch is required. Adapter compatibility tests must cover normalization, eligibility, filtering, and sorting semantics rather than require identical undocumented full-text scoring.

Sponsored placements occupy an explicitly labelled group/slot or carry an unambiguous placement contract. Promotion must not secretly modify organic ranking. Search results cannot fabricate impression, count, or relevance metrics.

SavedSearch stores criteria schema version, normalized values, owner, name, and notification state/preferences. Editing, migration of older criteria, pausing, deletion, user locale, matching checkpoints, notification frequency, and deduplication all require intentional behavior. A match must be rechecked for eligibility before sending an alert. An alert must not disclose content the recipient cannot access.

## 8. Exact values and time semantics

- Monetary values use integer minor units or an approved exact decimal representation plus explicit currency. The representation must account for currency minor-unit exponents and validated bounds. JSON transport must not lose integer precision in JavaScript; a decimal-string amount is a proposal to decide in the API contract.
- Revenue and result each carry their own period, definition where relevant, source, and verification claim. Do not aggregate incomparable periods or equate revenue with operating result. An undated headline amount is not sufficient financial provenance.
- Unknown, undisclosed, not applicable, and zero are distinct. Employee count versus full-time equivalent, range versus point estimate, and establishment-year interpretation need product definitions before validation rules are written.
- Store event instants in UTC. Use calendar dates for date-only concepts rather than manufacturing midnight timestamps.
- A process deadline needs an unambiguous instant, original local date/time, named IANA timezone, and the relevant rules/version context. Daylight-saving ambiguity/nonexistent times must be rejected or explicitly resolved, never guessed from a browser timezone.
- Server receipt/commit timestamps and asserted external occurrence timestamps are distinct. Late submissions, provider delays, clock skew, and deadline amendments require policy before offers are accepted.

## 9. Audit, events, and document processing

Domain event envelopes should carry stable event ID, event type/schema version, aggregate identity and version, ownership scope, occurrence/recording time as appropriate, and correlation/causation references. Payloads contain only what a consumer needs; consumers reauthorize protected reads under an explicit service identity.

The outbox records dispatch intent in the same transaction as the state change. Workers independently claim work using a recoverable strategy, retry bounded failures, expose failures to operations, and tolerate repeated dispatch. Unique consumer-processing keys or equivalent transactional guards prevent duplicate effects. A delivery acknowledgement does not prove an unrelated external side effect was completed exactly once.

Audit records answer actor, action, subject, time, context, and outcome. Audit context uses a controlled schema; no bearer tokens, signed URLs, document text, free-form user submissions, or payment secrets. Actor references, IP-related information, and retention are personal-data decisions and must be minimized and approved. “Append-only” at the application interface is not a claim of tamper-proof storage; operational access and evidence-integrity controls require separate design.

Document processing states must be independent of Listing lifecycle and disclosure. A draft design should distinguish upload initiation, receipt, validation, quarantine/scanning, release eligibility, rejection/failure, and deletion; state names and retry rules need a subsystem design. Upload completion alone never makes a document accessible. Scan failure or unavailability cannot be interpreted as clean. Original content, derived content, and version identity remain traceable without exposing storage paths.

## 10. Required invariant tests when implementation is approved

These are acceptance obligations for later executable tests, not a claim that tests or features already exist.

| Area | Required evidence |
| --- | --- |
| Tenant isolation | User in Workspace A cannot read, mutate, search protected content, attach documents, or obtain signed access for Workspace B, even using valid B identifiers. Cross-workspace parent/child associations fail at the data boundary. |
| Authority | Authenticated or identity-verified User without the required mandate and permission cannot perform a mandate-gated action. Revocation/expiry takes effect under the approved policy. |
| Public disclosure | HTML, API payloads, JSON-LD, metadata, sitemap, search documents, notifications, URLs, filenames, and media metadata exclude protected fixture markers. Anonymous content review has separate acceptance criteria. |
| Lifecycle | Every allowed command follows the approved matrix; every disallowed source/target/actor combination fails; concurrent edits do not silently overwrite one another. |
| Atomicity | An induced transaction failure cannot leave changed business state without its required local audit/outbox records or emit a successful domain event for uncommitted state. |
| At-least-once delivery | Repeated/concurrent input cannot duplicate transactional business effects or notification intentions. Distinct provider events for one purchase cannot duplicate an entitlement. External delivery uses provider idempotency or explicit reconciliation; residual duplicate-delivery risk must be stated rather than claiming exactly once. |
| Documents | Quarantined, unscanned, rejected, cross-workspace, expired-grant, and revoked-grant cases cannot obtain access; issued access follows the approved expiry and audit design. |
| Money and time | Exact round trips at supported bounds/currency exponents; no floating-point conversion; DST gap/overlap and UTC deadline comparison cases. |
| Search | Stable ordering for equal primary sort values; currency-aware filters; eligibility checks; normalized saved criteria round trip; paid placement is distinguishable from organic results. |
| Version history | Material changes preserve earlier approved evidence and public projection source version; corrections do not overwrite historical submissions. |

## 11. Decisions that gate dependent implementation

The central Phase 0 decision register owns decision identifiers. The domain decisions below are intentionally unresolved, not defaults, and must be incorporated into that register.

| Decision required | Proposed owner | Blocks |
| --- | --- | --- |
| Workspace ownership/cardinality; organization membership inheritance; separate professional client engagements; individual seller participation | Product owner + security architect | Tenant schema, role catalogue, onboarding, authorization policy |
| Marketplace eligibility: operating businesses only versus selected standalone asset packages; permitted combinations of the four sale dimensions | Product owner + relevant legal adviser | Taxonomy, listing validation, public search/filter UX |
| Seller party and mandate representation; evidence requirements; verification authority; expiry/revocation; actions requiring an effective mandate | Product owner + legal adviser + security architect | Publishing, seller badges, representative authority checks |
| Approved lifecycle graph; review requirements; payment dependencies; pausing/expiry/renewal; sold/withdrawn/archive semantics and reversal | Product/commercial owner + operations | State machine, jobs, public availability, entitlements |
| Disclosure granularity/composition; verification prerequisites; explicit approvals; confidentiality review; NDA scope and workflow | Product owner + privacy/security + legal adviser | Public projections, protected access, confidential seller journey |
| Version triggers and materiality; amendment review; document/translation pinning; historical visibility and correction policy | Product owner + process/legal adviser | Immutable version schema, publication amendments, process evidence |
| Financial metric definitions, time periods and provenance; price expression; supported currency representation; employee/size semantics | Product owner + domain specialist | Form/API validation, financial filters, reliable comparisons |
| Billing party/beneficiary model; publication pricing; subscription/promotion entitlements; refunds and revocations | Commercial owner | Platform payment flows and entitlement transitions |
| Insolvency jurisdictions/process eligibility; authority and offer access; deadline/late submission/amendment/withdrawal rules | Product owner + jurisdiction-qualified legal adviser | Offer acceptance, process operation, legal-facing claims |
| Data controller/processor roles; retention/deletion/holds; audit evidence retention; support access; buyer/seller communication ownership | Privacy owner + legal adviser + security architect | Production personal-data handling, deletion jobs, support operations |
| Search semantics/ranking; saved-search alert frequency/deduplication; public withdrawal and cache purge guarantees | Product owner + engineering/operations | Discovery contract, notifications, production cache policy |
| Public URL/slug identity; anonymous listing leak controls; country/language launch scope; sold/expired page and SEO eligibility policy | Product owner + UX/SEO + privacy | Public routes, metadata, sitemaps, translated content release |

The approved model should be captured in ADRs and explicit application contracts before migrations or dependent workflows are implemented. Phase 0 may establish terminology, boundary diagrams, proposal documents, and test obligations without answering these business/legal questions on the owner's behalf.
