# Repository doctrine

## Scope and authority

This file applies to the whole repository. Read the root README, the Phase 0 plan, the decision register and relevant domain/security documents before changing behavior. User instructions govern this work. Record approval evidence and scope in the decision register; a proposal is not an accepted decision. The user's initial 2026-09-17 approval authorized P0.1–P0.3 only. The subsequent Package A acceptance directive authorizes review, commit, source publication and clean-checkout CI first; P0.4 Identity & Authorization Integration may begin only after that gate passes. No listings, unrelated product features or production AWS are authorized. Visible authentication UI requires fresh targeted Mobbin research; prefer backend scope when sufficient. Later packages remain unapproved.

Once a work package is approved, proceed autonomously with reversible engineering choices inside its scope. Surface material decisions affecting security, privacy, legal interpretation, money, authorization, public UX, infrastructure, ownership, auditability or transaction semantics before dependent implementation. Do not reopen already accepted decisions without new evidence.

## Engineering rules

1. Keep the product focused on finding and selling businesses. Progressive disclosure and anonymous public browsing are requirements.
2. Use canonical English terminology from `docs/domain/GLOSSARY.md`. Sale subject, transfer structure, sale context and sale method are distinct dimensions.
3. Use Go for authoritative application rules and independently runnable workers; Next.js/TypeScript is a client of explicit versioned contracts. No domain decisions exclusively in React.
4. Prefer a modular monolith. Each module owns its tables, writes, contracts and event schemas. Do not create empty modules or generic abstractions without an existing responsibility.
5. Use explicit SQL, pgx and versioned migrations; use sqlc where its generated queries improve safety. No heavy ORM or unreviewed destructive migration.
6. Authorize on the server. Every protected object has an ownership path. Every new protected operation needs negative tests for workspace boundaries and revoked access.
7. Public representations are explicit allowlists. Never serialize private domain aggregates into public HTML, JSON, metadata, logs, indexes or caches.
8. Persist material state changes, durable audit evidence and asynchronous intent atomically. Assume delivery can repeat and reorder. Test crash windows and duplicate effects.
9. Never use floating point for money. Always carry currency. Store instants in UTC and preserve explicit deadline timezone semantics.
10. Treat identity evidence, seller mandate evidence and commercial verification claims separately. Do not implement legal conclusions, escrow, auction, NDA completion or verification badges without the corresponding approved workflow.
11. Public media and protected documents have separate access paths. Uploads remain unavailable until required validation and scanning succeed. Never use a public bucket as a shortcut.
12. No secrets, production data, access tokens, presigned URLs, financial documents or personal information in Git or test fixtures. Use synthetic data. Repository visibility is public at inspection.
13. External dependencies need a clear purpose, license review, version lock and vulnerability review. Pin CI actions and runtime/build images to immutable references; record upgrades through review.
14. Use semantic design tokens and message catalogs. A new major journey requires an actual Mobbin research record before UI implementation. Document unavailable access honestly.
15. WCAG 2.2 AA and security verification belong to acceptance criteria. Automated scans alone do not establish accessibility or security compliance.
16. Do not claim a test, scanner, deployment, restore or user journey succeeded unless it ran and its result was inspected. Distinguish planned, implemented, verified and accepted.

## Working method

- Inspect working tree changes before editing; preserve unrelated user work.
- Keep changes reviewable and focused. Use an ADR for a significant architectural decision and include alternatives and reversal cost.
- Reference requirements and decision IDs in work descriptions. Update docs with behavior.
- Run checks appropriate to the changed surface. Do not create implementation-mirroring tests for documentation or formatting edits.
- New API/schema behavior needs contract and migration evidence. Security changes need abuse-case evidence. UI changes need keyboard, responsive and accessibility review.
- Add directories only when they contain real responsibilities or artifacts. Planned topology is documented in `SYSTEM.md`, not represented by empty scaffolding.
- Follow `docs/operations/QUALITY-GATES.md`; its future gates are not currently installed. Do not silently skip a required gate or fabricate reviewer ownership.

## Definition of done

A work package is done when its acceptance criteria are met, relevant checks have run, residual risks and unresolved decisions are explicit, operational/security documentation matches behavior, and the change can be reviewed and reversed safely. Production readiness additionally requires the release gates and named operational owners; passing unit tests is insufficient.
