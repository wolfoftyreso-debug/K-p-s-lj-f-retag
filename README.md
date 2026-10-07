# European Business Marketplace

Production foundation for a European marketplace for businesses and operating businesses for sale.

**Current delivery: Package A, P0.4 backend and the approved private draft slice verified in committed clean-checkout CI; real-provider acceptance remains open.** Go API/worker, workspace isolation and atomic audit/outbox remain the foundation. Read the [Package A release gate](docs/operations/PACKAGE-A-RELEASE-GATE.md), [P0.4 verification report](docs/operations/P0.4-REPORT.md), [identity runbook](docs/operations/P0.4-IDENTITY.md) and [identity design](docs/architecture/P0.4-IDENTITY-DESIGN.md) for actual scope and unresolved acceptance. This is a backend foundation, not a deployed marketplace or a claim of production readiness. The repository name is not an approved product brand.

The user subsequently approved the bounded [private draft slice](docs/architecture/DRAFT-SLICE-PROPOSAL.md) on 2026-10-07. Its implementation adds separate private Business/Listing Draft records, protected Preview, scoped permissions, personal drafting bootstrap and atomic evidence/retries. Read [ADR 0008](docs/adr/0008-private-listing-drafts.md) and the [private draft runbook](docs/operations/PRIVATE-DRAFTS.md) for actual behavior. Its own committed result and exact local/CI evidence are recorded in the [private draft verification report](docs/operations/PRIVATE-DRAFTS-REPORT.md); earlier CI is not used as an attestation of this slice. The browser study is disconnected. Publication, uploads, real-data admission and AWS remain excluded.

Start with the [Package A runbook](docs/operations/PACKAGE-A.md), [Phase 0 implementation plan](docs/architecture/PHASE-0-PLAN.md) and [decision register](docs/architecture/DECISIONS.md). The initial P0.1–P0.3 approval and later P0.4/release-gate authority remain recorded as history. Other later packages remain unapproved.

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
- P0.4 adds strict OIDC, durable local sessions and authenticated backend contracts for the existing workspace commands. Authentication mode is explicit; disabled mode denies protected access. No visible authentication interface or AWS provisioning is included. Real-provider conformance remains an independent acceptance gate.
- The later approved private draft package uses that boundary and current scoped grants; organization affiliation gives no implicit workspace authority. It implements only DRAFT and protected Preview, with no public listing route or asynchronous draft projection.
- Planned controls and quality gates must not be described as implemented or passing.

Documentation and canonical identifiers use English. Product localization is a separate concern.
