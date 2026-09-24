# European Business Marketplace

Production foundation for a European marketplace for businesses and operating businesses for sale.

**Current delivery: Phase 0 Package A — committed baseline passed clean-checkout Linux CI.** Go API/worker, workspace isolation and atomic audit/outbox are implemented within the approved scope. Read the [local completion report](docs/operations/PACKAGE-A-REPORT.md) and [release-gate record](docs/operations/PACKAGE-A-RELEASE-GATE.md) for exact commit/run evidence and follow-up verification. The [P0.4 identity proposal](docs/architecture/P0.4-IDENTITY-DESIGN.md) awaits its material D03 decisions before implementation. This is a backend foundation, not a deployed marketplace or a claim of production readiness. The repository name is not an approved product brand.

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
- Public authentication, frontend, listings and AWS provisioning are outside this package. Protected HTTP requests deny access until an approved authentication adapter exists.
- Planned controls and quality gates must not be described as implemented or passing.

Documentation and canonical identifiers use English. Product localization is a separate concern.
