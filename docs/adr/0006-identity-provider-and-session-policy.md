# ADR 0006: identity provider and session policy

Date: 2026-09-24. Status: **PROPOSED — awaiting D03 owner decision.** No implementation or commercial/provider provisioning approval is asserted.

## Context

Package A has a committed, CI-verified authentication boundary that denies protected HTTP access. P0.4 is authorized after that release gate. The owner confirmed no existing identity provider and requested a concrete proposal. Authentication, application permissions, workspace membership, seller mandate and verified claims must remain distinct.

## Proposed decision

Use a provider-independent Go OIDC adapter with authorization-code/PKCE, canonical human/service principals and application-owned PostgreSQL sessions and permissions. Prefer Auth0's EU locality, with the Enterprise back-channel/session capabilities conditional on acceptable terms, entitlement and actual sandbox proof. No purchase or EU-only processing guarantee is implied. The exact reasoning and alternatives are in [provider options](../architecture/P0.4-PROVIDER-OPTIONS.md).

Propose an eight-hour absolute session, fifteen-minute protected-request idle timeout, fresh interactive authentication after either expiry, no persistent provider refresh tokens, secure opaque browser cookies and transactional local revocation. First verified-email login may create an internal User and issuer/subject mapping with no workspace authority; disabled identities cannot be recreated. Provider-side revocation is asynchronous and cannot inherit the local immediate-revocation claim. Unknown assurance never satisfies step-up. See the complete [session/admission policy, threat review and test plan](../architecture/P0.4-IDENTITY-DESIGN.md) before approval; these concrete values and trade-offs are not approved defaults yet.

## Alternatives and reversal cost

Lower-plan Auth0 or Cognito may be appropriate if cost/terms favor them and the owner accepts a reviewed shorter-session/provider-change propagation policy; they must pass the same explicit assurance and revocation tests. Self-hosted Keycloak offers configuration control with substantial identity operating responsibility. Browser-held bearer tokens change theft/refresh/revocation trade-offs and are not recommended for this initial web boundary.

Keep immutable internal IDs and `(issuer,subject)` mappings so provider migration does not rewrite authorization. Replacing a provider still requires verified account relinking and credential/passkey enrollment planning; it is not a free configuration change. Do not silently link equal email addresses. Session-policy changes require explicit expiry/revocation migration behavior.

## Approval and evidence

Approval pending. No provider account, UI, P0.4 source code or identity schema exists from this ADR. Real provider conformance is not run. Provider feature/cost/processing decisions and the session/security policy must be explicitly resolved before dependent implementation; no later product or AWS package begins from this proposal.
