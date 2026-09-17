# Experience foundation

Date: 2026-09-16
Status: proposed Phase 0 specification; no public UX, locale rollout, disclosure policy or visual identity is approved by this document. Requirements explicitly supplied in the master directive remain binding.

## Product contract

The product exists to help a seller offer a business and a buyer find it and continue a deal. Public discovery must work without authentication. Introduce authentication when the requested action requires an accountable user, such as saving, contacting, publishing or accessing protected information. Do not infer that authentication establishes verification, buyer eligibility or seller authority.

The primary navigation should express the two user intentions: find a business and sell a business. Labels and layout require product approval and localization. No dashboard, onboarding survey or organizational setup should interrupt anonymous discovery.

## Buyer and seller journeys

| Journey | Sequence from the directive | Architectural consequence |
| --- | --- | --- |
| Buyer | Search → view business → save or contact → access more information when authorized → continue the deal | Search and details consume safe public projections. Save/contact are explicit authenticated commands. Restricted data requires server-side authorization for each request. No deal-completion or escrow capability is implied. |
| Seller | Start selling → describe business → financial information → media → documents → disclosure → preview → publish | Draft saving and validation are application contracts. Publication is a deliberate authorized state transition with an audit event, not a form submission that directly changes a database status. Payment/review gates remain business decisions. |

Show why a field is needed at the step where it becomes relevant. Reuse previously entered information, preserve drafts, distinguish unknown from zero, and make optional fields visibly optional. Do not require a professional workspace model to be understood before a first-time seller can understand the task. The identity and ownership model still applies on the server.

Seller preview must render the same safe projection used for anonymous visitors, separately from any protected views. Disclosure controls should show concrete examples of what becomes visible. Whether particular information is public, registered, verified, approved or subject to an NDA requires an approved policy. A proposed NDA label must not imply a completed NDA workflow.

Do not expose publication, payment, uploading or protected access as available features until their corresponding backend behavior, failure handling and auditability exist. This documentation package and foundation package A do not implement these journeys. Later packages require their own decisions, journey research and acceptance evidence.

## Research gate

Before each major journey, create `docs/research/ux/<date>-<journey>.md` with the problem, actual Mobbin references inspected, observations, rejected patterns, accessibility considerations and product decision. A static screenshot supports visual observations only.

[Marketplace search research](../research/ux/2026-09-16-marketplace-search.md) is the only completed pass in this foundation. It does not complete research for filtering interactions, listing detail, saving, saved searches, notifications, seller onboarding, listing creation, uploads, checkout, accounts, messaging or professional workspaces. Research reduces uncertainty; it does not approve material public UX decisions.

## Internationalization contract

English is the canonical source language for product messages and domain terminology. Product message identifiers must be stable English keys, separate from rendered wording. Use one message/catalog boundary for server-rendered pages, browser components, email, notifications, validation and metadata. API errors carry stable codes and structured parameters; clients localize them rather than parsing English error text. Do not concatenate sentence fragments or embed user-facing strings throughout components.

The locale registry must accommodate all 24 official EU languages: `bg`, `hr`, `cs`, `da`, `nl`, `en`, `et`, `fi`, `fr`, `de`, `el`, `hu`, `ga`, `it`, `lv`, `lt`, `mt`, `pl`, `pt`, `ro`, `sk`, `sl`, `es`, `sv`. This is architectural coverage, not a claim that catalogs or translations exist. The [EU language inventory](https://european-union.europa.eu/principles-countries-history/languages_en) was checked on 2026-09-16. Additional European languages must be addable without changing domain identifiers.

Proposed implementation requirements:

- Use explicit locale identifiers capable of regional variants; keep language, country, currency and jurisdiction in separate fields. A user's language never changes authoritative financial amounts or governing process rules.
- Keep supported, translated, reviewed and publicly enabled locale states separate. The launch set is an open product decision. Do not advertise a locale before its public journey and required communications are reviewed.
- Support plural rules, parameter formatting, Unicode text, language-specific search behavior and localized validation. Dates and money use locale-aware formatting. Authoritative amounts remain exact and currency-qualified.
- Keep listing translations separate from the original content, with source language, source version, translator provenance and review status. A material original change makes dependent translations stale. Translation never changes disclosure rights.
- Resolve UI and listing-content fallback explicitly. Never label untranslated listing content as a completed translation or publish duplicate language pages solely because the navigation was translated.
- Apply the correct document language and language-of-parts semantics. Use logical CSS properties and accommodate text expansion; do not make the component system inherently dependent on left-to-right presentation.
- Persist UTC instants. Display commercially significant deadlines with their explicit named timezone and date; locale affects formatting, not the instant. Retain the original timezone semantics where a deadline is authored in local civil time.
- Catalog parity, placeholder compatibility and representative plural cases are release checks. Test accented Latin, Greek and Cyrillic content and long labels. No unchecked machine-generated legal or confidentiality text.

## Public representation and SEO

Server-render public marketplace pages through Next.js App Router using the public application contract. React is a presentation client; it does not decide whether a private field can be returned. Public representations must also govern title tags, descriptions, structured data, image metadata and filenames, links, sitemaps and search projections. Private API responses must never enter a shared public cache.

Propose stable identifiers with human-readable slugs; exact route and localization strategy require an ADR before public launch. Slug changes need deliberate redirect behavior and must not resurrect confidential names. A route registry should produce canonical URLs and only valid, published locale alternates. Google recommends distinct URLs for language versions and explicit links between them; do not infer language from country or force an IP-based redirect. [Google multilingual guidance](https://developers.google.com/search/docs/specialty/international/managing-multi-regional-sites).

Use an explicit indexability policy, with tests for each public page type:

| Surface | Proposed eligibility and safeguards |
| --- | --- |
| Public listing | Published state, approved public projection, permitted disclosure and eligible content/locale. Generate accurate metadata from that projection. Structured data only when its semantics match the page; do not invent ratings or present a business acquisition as an ordinary checkout product. |
| Curated discovery page | Explicitly allowlisted combination with useful reviewed content and stable URL; no automatic eligibility for every filter combination. |
| Arbitrary search/facets | Not indexable by default; normalized and bounded criteria. Separate useful user navigation from crawler discoverability. |
| Draft, account, workspace, protected document | Access controlled; absent from public indexes and sitemaps. Robots directives are not authorization. |
| Sold, withdrawn, expired, archived | Policy remains unresolved: preservation, removal, redaction and redirects affect privacy and public UX. Do not guess. |

Faceted URLs can create unbounded crawl spaces. Approve a crawl policy as well as an index policy; canonical tags alone do not reliably bound crawling. Coordinate `noindex` and crawl blocking so crawlers can read intended directives. Do not expose private criteria in crawler links. [Google faceted navigation guidance](https://developers.google.com/crawling/docs/faceted-navigation).

Sitemaps must include only eligible URLs and real content modification times. `hreflang` must refer to actual eligible counterparts, never blank or fallback-only translations. The search index is a projection; revocation and withdrawal must invalidate public rendering and discovery according to an explicit freshness contract. An external search engine's cache cannot be promised to disappear instantly.

## Accessibility and quality

Target WCAG 2.2 AA. Require semantic HTML, keyboard navigation, visible unobscured focus, programmatic labels and errors, usable target sizes, zoom/reflow, contrast, reduced motion and accessible authentication. Core controls need text alternatives and state announcements. Avoid status communicated by color alone. The reference is [WCAG 2.2](https://www.w3.org/TR/WCAG22/); this document makes no conformance claim.

For each journey, specify empty, loading, validation, permission-denied, expired-session and recoverable-error behavior. Retain user input where safe. Disabled controls must explain an unmet requirement; do not use disabled buttons as the only error explanation. Never render invented listings, verification marks, buyer counts or advertising metrics to make a page look complete.

Definition of Done includes automated accessibility checks plus manual keyboard and screen-reader checks on complete journeys. Exercise narrow layouts, long translations, reduced motion and real error states. Automated tools are evidence, not proof of complete conformance. Record findings and unresolved defects with the release evidence.

## Semantic token proposal

Candidate accent `#216B52` has sRGB relative luminance `0.1144791936`. Contrast with white is `(1.0 + 0.05) / (0.1144791936 + 0.05) = 6.3837861602:1`. Therefore white text on this green, or this green text on white, meets the 4.5:1 normal-text AA threshold. It does not meet the 7:1 normal-text AAA threshold. Black on this green is only `3.2895838728:1` and must not be the normal-sized button-label pairing. Ratios were calculated on 2026-09-16 using the WCAG sRGB linearization formula. This validates one pair, not all component states or the final palette.

Proposed semantic names, to be defined centrally in the design-token source before UI work:

| Token | Responsibility | Proposed value/status |
| --- | --- | --- |
| `color.background.canvas` | Default public-page background | White candidate; evaluate complete page states. |
| `color.background.surface` | Cards and dialogs | Neutral surface; value pending design review. |
| `color.foreground.primary` | Body and heading text | Charcoal; value pending contrast testing. |
| `color.foreground.secondary` | Supporting text | Neutral, still sufficient text contrast. |
| `color.border.subtle` | Decorative separators | Must not be the sole control boundary. |
| `color.border.control` | Input/control boundary | Test against adjacent surfaces. |
| `color.action.primary.background` | Primary action background | Candidate `#216B52`. |
| `color.action.primary.foreground` | Primary action label | Candidate `#FFFFFF`; pair tested above. |
| `color.action.primary.hover` / `active` | Interactive feedback | Distinct values to be tested before adoption. |
| `color.focus.ring` | Keyboard focus indicator | Separate semantic token; test all adjacent colors. |
| `color.status.error` / `warning` / `success` | Status semantics | Values pending; always paired with meaningful text/icon. |

Typography, spacing, line height, radii, elevation and motion also require semantic tokens. Prefer a small scale, restrained elevation and generous spacing. Do not introduce arbitrary component color values. A design-token change must trigger relevant contrast and visual checks. The accent remains a proposal until full state and layout review.

## Decisions before public implementation

Record the chosen owner and outcome in the central decision register for: launch languages/markets; public disclosure fields and location granularity; default discovery ordering and sponsored placement; URL/slug/locale structure; sold/expired content policy; and the initial research-backed page hierarchy and token palette. Launch language does not establish jurisdiction, and UI approval does not approve legal or security policy.
