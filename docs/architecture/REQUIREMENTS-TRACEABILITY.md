# Directive traceability

Created: 2026-09-16. Package A update: 2026-09-17. This maps the directive to the target design and eventual proof. **Design coverage does not mean implementation coverage.** Package A implementation boundaries are recorded in the [runtime runbook](../operations/PACKAGE-A.md); future work below remains unimplemented. Package identifiers refer to [PHASE-0-PLAN.md](PHASE-0-PLAN.md).

| Directive | Design location | Implementation / evidence obligation |
| --- | --- | --- |
| 0 First principle | [Inspection](REPOSITORY-INSPECTION.md), [doctrine](../../AGENTS.md), [decisions](DECISIONS.md), [plan](PHASE-0-PLAN.md) | Inspection/design preceded code; P0.1–P0.3 approved 2026-09-17; later work unapproved |
| 1 Mission and distinct sale concepts | [Glossary](../domain/GLOSSARY.md), [model](../domain/MODEL.md) | P0.7: separate SaleSubject, TransferStructure, SaleContext and SaleMethod; validate approved combinations |
| 2 Product doctrine | [Experience](../product/EXPERIENCE-FOUNDATION.md) | Future journeys: anonymous browse, just-in-time auth and progressive disclosure; no unnecessary onboarding |
| 3 UX standard | [Experience](../product/EXPERIENCE-FOUNDATION.md) | Semantic token/contrast proposal; responsive, empty/loading/error and accessible state review before public UX |
| 4 Mobbin research | [Search research](../research/ux/2026-09-16-marketplace-search.md) | One actual research pass delivered; each subsequent major journey still requires its own pass |
| 5 Architecture | [ADR 0001](../adr/0001-directive-baseline.md), [system](SYSTEM.md) | P0.1/P0.3: monolith and independently runnable workers, measured need before distribution |
| 6 Stack | [ADR 0001](../adr/0001-directive-baseline.md), [system](SYSTEM.md), [operations](../operations/READINESS.md) | P0.1–P0.6: pinned supported tools, contracts/SQL, AWS/Terraform/CI after decisions |
| 7 Repository | [Doctrine](../../AGENTS.md), [system](SYSTEM.md) | Only actual responsibilities get runtime directories; documentation exists now |
| 8 Domain language | [Glossary](../domain/GLOSSARY.md) | All requested concepts defined; future contract/schema/event terminology review |
| 9 Identity and authorization | [Security](../security/SECURITY-BASELINE.md), [model](../domain/MODEL.md) | P0.2/P0.4/P0.7: application permissions, provider boundary, mandate independent of identity |
| 10 Multi-tenancy | [Model](../domain/MODEL.md), [threat model](../security/THREAT-MODEL.md) | D02 then P0.2: real DB/API negative tests, ownership keys, revoked access and pool reuse |
| 11 Listing model | [Model](../domain/MODEL.md) | D07 then P0.7: structured relational content, financial provenance, country extensions |
| 12 State machine | [Model](../domain/MODEL.md), [system](SYSTEM.md) | D07 then P0.7: approved explicit transition graph, policy predicates, version/audit/outbox atomicity |
| 13 Disclosure | [Security](../security/SECURITY-BASELINE.md), [experience](../product/EXPERIENCE-FOUNDATION.md) | D10/D15, P0.5/P0.7: safe projection, no identity leakage, no fake NDA workflow |
| 14 Document security | [Threat model](../security/THREAT-MODEL.md), [security](../security/SECURITY-BASELINE.md) | D05/D08/D15 then P0.5: validation/scanning/quarantine, version-bound private access, audit and deletion |
| 15 Search | [System](SYSTEM.md), [model](../domain/MODEL.md) | D10/D16 then P0.7: SearchPort, PostgreSQL adapter, stable filter/order, no public protected text or paid organic reordering |
| 16 Saved searches | [Model](../domain/MODEL.md), [system](SYSTEM.md) | P0.7 normalized contract/schema; later saved-search journey and matching/notification implementation after D16 |
| 17 Payments | [Security](../security/SECURITY-BASELINE.md), [system](SYSTEM.md) | D09 then future billing feature: signed durable inbox, replay/out-of-order tests and unique entitlements; platform services only |
| 18 Advertising | [Glossary](../domain/GLOSSARY.md), [model](../domain/MODEL.md) | D16 then future promotion feature: campaign/placement/schedule/package/delivery/status, clear labels and real measurement |
| 19 Insolvency | [Model](../domain/MODEL.md), [decisions](DECISIONS.md) | D14 then future dedicated context: factual process records and evidence, no legal conclusions |
| 20 Auditability | [Security](../security/SECURITY-BASELINE.md), [system](SYSTEM.md) | P0.3 schema and atomic recording; D08/D12 before real personal data or privileged access |
| 21 Events/outbox | [System](SYSTEM.md), [quality gates](../operations/QUALITY-GATES.md) | P0.3 crash, lease, duplicate, reorder and poison-item tests; state/audit/outbox atomic |
| 22 Internationalization | [Experience](../product/EXPERIENCE-FOUNDATION.md) | P0.1 catalog boundary; 24-language extensibility, later actual translations and locale rollout under D10 |
| 23 Money | [Model](../domain/MODEL.md), [system](SYSTEM.md) | P0.2 safe representation/round-trip/overflow tests; no inferred currency or silent conversion |
| 24 Time | [Model](../domain/MODEL.md), [system](SYSTEM.md) | P0.2 UTC and timezone primitives; approved process deadlines and DST cases before offers |
| 25 SEO | [Experience](../product/EXPERIENCE-FOUNDATION.md) | D10 then P0.7/future public journey: SSR, stable routes, canonical/hreflang/sitemaps, facet eligibility, sold/expired policy |
| 26 Accessibility | [Experience](../product/EXPERIENCE-FOUNDATION.md), [quality gates](../operations/QUALITY-GATES.md) | WCAG 2.2 AA automated and manual evidence per journey, no conformance claim from design alone |
| 27 Security baseline / no secrets | [Security](../security/SECURITY-BASELINE.md), [threat model](../security/THREAT-MODEL.md), [operations](../operations/READINESS.md) | ASVS applicability/evidence, browser/API controls, scans, encryption, least privilege, real restore/drill and release gates |

## Preventing false completion

The immediate result is an inspected repository and a reviewable design package. P0.1–P0.8 are planned implementation, not completed features. Live payments, advertising delivery, messaging, saved-search notifications and insolvency/offer processing require later feature work; defining their vocabulary is not implementing them. Those boundaries must remain visible in future progress reports.
