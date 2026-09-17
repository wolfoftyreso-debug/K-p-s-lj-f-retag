# ADR 0001 — Directive-established foundation

Date: 2026-09-16. Status: **Accepted by the user's Master Build Directive**, limited to the choices explicitly stated below. This ADR is not approval of implementation proposals or of production deployment.

## Context

The inspected repository contains only its initial README. The platform must support a long-lived European business marketplace, sensitive information, professional users and many jurisdictions. Simplicity in user journeys must coexist with explicit internal boundaries.

## Decisions already established

- Monorepo by default; Go modular monolith with independently runnable asynchronous workers. No initial microservices, Kubernetes or Kafka.
- Next.js App Router, TypeScript and React; public server rendering and SEO; Go owns authoritative application rules.
- PostgreSQL is transactional authority, pgx and explicit SQL, sqlc where appropriate, versioned SQL migrations; no heavy ORM.
- AWS target platform: RDS PostgreSQL with production Multi-AZ, ECS/Fargate/ECR/ALB, CloudFront/WAF, S3, SQS/DLQ, CloudWatch and OpenTelemetry; Terraform and GitHub Actions.
- SearchPort supports PostgreSQL first and later OpenSearch. Indexes are replaceable projections. Sponsored placements remain separate from organic ranking.
- Application-owned authorization, explicit tenant ownership, first-class SellerMandate, independent identity and authority verification.
- Public representations are intentionally safe; protected documents remain private. Durable audit and transactional outbox are foundational. Delivery is at least once.
- Stripe charges platform services only. No purchase-price transfer or escrow. No pretend auction or NDA workflow, and no legal conclusions in insolvency.
- Canonical English domain language; localized product; all 24 EU languages supported architecturally. Currency, language, country and jurisdiction are distinct.
- Safe money representation, UTC instants and explicit deadline timezone semantics; WCAG 2.2 AA and OWASP ASVS baseline.
- Inspect, document and obtain approval for the foundation plan before implementing it. Surface material unresolved decisions.

## Consequences and boundaries

The default stack requires no new vendor selection debate at this stage. It does require precise ownership, transaction boundaries, security controls, operational evidence and tests. Modules and infrastructure are added for actual responsibilities rather than appearance.

The directive does **not** select an AWS region, account topology, authentication vendor, tenant database isolation strategy, public disclosure policy, seller verification threshold, price, listing review/payment order, retention period, SLO or launch jurisdiction. Those remain in the [decision register](../architecture/DECISIONS.md).

Changes to this baseline require an explicit superseding ADR and user decision, with migration and reversal costs. Ordinary choices consistent with it can be documented and made by engineering inside approved scope.
