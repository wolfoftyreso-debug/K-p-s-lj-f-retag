# ADR 0002 — Package A runtime and authentication boundary

Date: 2026-09-17. Status: accepted within explicit Package A approval (D01); vendor authentication remains deferred under D03.

## Decision

Use one Go module, standard-library HTTP, pgx, and independently compiled API and worker programs. Migrations have a separate administrative executable. Configuration is explicit and validated at startup; no process automatically applies schema changes. No frontend, listing or authentication-provider integration is created.

The API exposes liveness/readiness and an intentionally closed versioned protected boundary. The normal composition never accepts actor, role or workspace claims from HTTP headers or bodies. It returns `authentication_required` for protected requests. This is not a usable sign-in or workspace HTTP product API.

The real workspace application/storage operations take an internal authenticated actor reference, evaluate active membership and permissions in PostgreSQL, and operate only in that scope. Synthetic principals exist in tests only. This is real authorization with no authentication provider yet; it is not evidence that real users can sign in. A future approved adapter must authenticate before calling these operations.

The worker directly polls the PostgreSQL outbox under its own database capability. No task requires the API process to be running. Cancellation propagates to PostgreSQL operations; HTTP readiness is withdrawn before draining. Logs use bounded structured fields and do not include DSNs, request bodies or raw database errors.

## Alternatives and consequences

An arbitrary development token, trusted `X-User-ID`, or automatic fallback principal would create a bypass and is rejected. Implementing OIDC/mTLS account policy now would exceed D03 and this approval. Keeping the HTTP boundary closed is deliberate and tested, not a placeholder authorization mechanism. Authenticated operation tests run against real PostgreSQL through the application boundary.

Go dependencies and toolchain versions are pinned in `go.mod`/`go.sum`; supported release metadata was verified at implementation. Container deployment and OpenTelemetry export remain later infrastructure work. Package A is not independently production-launchable: account/bootstrap policy, real authentication, secret management and operational release decisions are still required.
