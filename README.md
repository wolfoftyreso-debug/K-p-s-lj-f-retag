# European Business Marketplace

Production foundation for a European marketplace for businesses and operating businesses for sale.

**Current delivery: Package A verified in clean-checkout CI; P0.4 identity backend under verification.** Go API/worker, workspace isolation and atomic audit/outbox remain the foundation. Read the [Package A release gate](docs/operations/PACKAGE-A-RELEASE-GATE.md), [P0.4 implementation/runbook](docs/operations/P0.4-IDENTITY.md) and [identity design](docs/architecture/P0.4-IDENTITY-DESIGN.md) for actual scope and open provider acceptance. This is a backend foundation, not a deployed marketplace or a claim of production readiness. The repository name is not an approved product brand.

Start with the [Package A runbook](docs/operations/PACKAGE-A.md), [Phase 0 implementation plan](docs/architecture/PHASE-0-PLAN.md) and [decision register](docs/architecture/DECISIONS.md). The user approved P0.1–P0.3 on 2026-09-17 and subsequently authorized commit/publication/CI plus P0.4 after that release gate passes. Later packages remain unapproved.

## Review map

| Document | Purpose |
| --- | --- |
| [Repository inspection](docs/architecture/REPOSITORY-INSPECTION.md) | Observed starting state, provenance, limitations, assumptions |
| [Repository doctrine](AGENTS.md) | Rules for contributors and engineering agents |
| [System architecture](docs/architecture/SYSTEM.md) | Runtime, module, contract, transaction and failure boundaries |
| [Architecture decisions](docs/adr/0001-directive-baseline.md) | Decisions already established by the directive |
| [Decision register](docs/architecture/DECISIONS.md) | Proposed choices, alternatives, owners and blocking points |
| [Domain glossary](docs/domain/GLOSSARY.md) | Canonical English terminology |
| [Domain model](docs/domain/MODEL.md) | Ownership, structured listing model and proposed lifecycle |
| [Threat model](docs/security/THREAT-MODEL.md) | Trust boundaries, attacks, mitigations and verification |
| [Security baseline](docs/security/SECURITY-BASELINE.md) | Security and privacy engineering requirements |
| [Experience foundation](docs/product/EXPERIENCE-FOUNDATION.md) | Simple journeys, accessibility, localization and SEO |
| [Mobbin search research](docs/research/ux/2026-09-16-marketplace-search.md) | Inspected references and bounded conclusions |
| [Quality gates](docs/operations/QUALITY-GATES.md) | Required evidence before merge and release |
| [Operational readiness](docs/operations/READINESS.md) | Infrastructure, telemetry, recovery and incident ownership |
| [Directive coverage](docs/architecture/REQUIREMENTS-TRACEABILITY.md) | Every directive section mapped to design and delivery evidence |

## Delivery status

- Repository baseline inspected in full at `e048a121492aef8a4c1aa43a004a2fe6550ffac8`.
- The release gate uses `docs/phase-0-foundation` and requires committed source plus hosted CI evidence; no production deployment is authorized.
- API, worker, migrations, PostgreSQL authorization and transactional processing passed the local Package A checks, including real PostgreSQL integration, race detection and security scans.
- P0.4 adds strict OIDC, durable local sessions and authenticated backend contracts for the existing workspace commands. Authentication mode is explicit; disabled mode denies protected access. No visible authentication interface, listings or AWS provisioning are included. Real-provider conformance remains an independent acceptance gate.
- Planned controls and quality gates must not be described as implemented or passing.

Documentation and canonical identifiers use English. Product localization is a separate concern.
