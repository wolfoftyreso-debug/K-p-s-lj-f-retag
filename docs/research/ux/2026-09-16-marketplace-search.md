# Marketplace search: targeted Mobbin research

Date: 2026-09-16
Status: research completed; product proposals require approval before public UX implementation.
Scope: the marketplace search results surface, including visible filter and save affordances. This is not research completion for the full filtering, saving, listing detail, seller, authentication, or notification journeys.

## Problem

Help an anonymous buyer find relevant businesses, understand the active search, compare public facts, and continue to a listing without unnecessary interruption. Preserve the distinction between organic discovery and paid visibility. A business purchase involves confidentiality and information asymmetry; consumer shopping patterns must be adapted rather than copied.

## Method and evidence limits

Used the connected Mobbin search tool with two targeted queries: one for web results with search, filters, listing cards and save controls; one for mobile results with a current query, filters, price and location. Visually inspected all five returned image previews on this date. Canonical Mobbin screen links are recorded below. No third-party screenshot is embedded or copied into this repository.

These are static reference screens. They do not establish keyboard behavior, screen-reader output, responsive breakpoints, performance, anonymous access, successful saving, or how controls behave after activation. Mobile references are iOS screens and inform principles only; the proposed product remains a responsive web application. No claims about the reference products' accessibility conformance are made.

## References inspected

| Reference | Direct observation | Useful principle | Limit or rejected transfer |
| --- | --- | --- | --- |
| [Faire: web search results](https://mobbin.com/screens/7c0ac947-8ea5-48e8-b61d-9f63ccc18140) | The header contains a wide search field; filter controls sit above a card grid. Each pictured card has an explicit “Add to list” control. | Keep search visible on results and place a secondary save control consistently near each item. | Retail minimum-order prices, promotional discount labels and image-dominant cards do not express business-sale comparability. |
| [Relevance AI: web marketplace results](https://mobbin.com/screens/32b88167-1a97-4fbd-8707-340cf80defb8) | A heading repeats the query; a row contains search, listing/category/price choices and sorting. Cards show short titles, descriptions and attribution. | State what the results represent and keep sorting distinct from filtering. | The persistent professional-app sidebar competes with the marketplace task and is not a public-marketplace navigation proposal. |
| [Dribbble: web results](https://mobbin.com/screens/80662ab5-1b1b-4927-a12c-ea8140f35bcf) | Search is in the header. The query appears in the heading, filters are grouped, and active filter chips sit beside a clear-filters control. | Expose applied criteria and provide an obvious reset path. | Trending topic chips, recruitment navigation and a large image grid add choices unrelated to assessing a business. |
| [eBay: iOS search results](https://mobbin.com/screens/c8aa89dc-184a-4292-b885-71aadfb86c39) | The query remains at the top; Filter, Sort and Price controls precede stacked results. A visible Sponsored label separates paid material. Results include price, short description and location information. | Preserve search context on a small screen; label paid visibility explicitly; keep comparable facts near the item. | Shipping, units sold, shop promotion and stacked merchandising rows are not appropriate business-sale signals. Do not infer sponsored-ranking mechanics from this image. |
| [Etsy: iOS search results](https://mobbin.com/screens/c8db476a-7762-4e4f-836a-4a535ea0d0bd) | Search and a filter icon sit above a two-column grid. The current sort is stated. Heart controls repeat on cards. | Make the selected sort visible and save placement consistent. | Dense photography, truncated titles and several merchandising chips obscure business facts. A heart icon alone is insufficient as our accessible control name. |

## Principles extracted

1. Search context should remain obvious: current query, active criteria and selected ordering.
2. Criteria and sorting are separate concepts; neither should secretly change the other.
3. Results should use a stable fact hierarchy. A business card needs public title, appropriate location granularity, industry and disclosed financial facts before decorative imagery.
4. Save should be a predictable secondary action. This pass supports its discoverability, not a complete save or sign-in flow.
5. Paid results need a visible, localized label and a separate placement contract. A paid position must not change the organic relevance score.
6. A narrow viewport needs fewer simultaneous controls, with the active criteria still inspectable and reversible.

## Patterns rejected for this product

- A forced account or onboarding wall before public search; anonymous browsing is part of the product directive, not a behavior verified in these screenshots.
- Dense professional dashboards around public discovery, several competing primary calls to action, and retail-style urgency.
- Cards that rely on images or generic trust badges while omitting financial period/source or confidentiality context.
- Hidden paid ranking, unlabeled sponsorship, fabricated popularity or impression counts.
- Controls identifiable only by color or unlabeled icons; horizontal chip carousels as the only way to reach an essential filter.
- Endless scrolling as a default commitment. Pagination and back-navigation behavior require an explicit design and test before implementation.

## Accessibility considerations for implementation

Provide a labeled search form, semantic results list, meaningful listing links and named buttons. Save state must be programmatically available. Keep focus visible, restore focus after a filter dialog closes, and announce result updates without moving focus on every keystroke. Preserve user-entered criteria after a recoverable error. Test keyboard use, zoom, reflow, screen readers, contrast and reduced motion; a screenshot cannot verify these. The target is [WCAG 2.2 AA](https://www.w3.org/TR/WCAG22/).

## Product proposal and decision

Carry these interaction principles into the Phase 0 specification. Propose a public search surface with a prominent labeled query field, a small set of primary filters, a secondary expanded filter control, visible applied criteria, explicit ordering, and factual listing summaries. Public results must come exclusively from the approved public listing representation. Filters may reference only publicly disclosable values; counts and ranking must not reveal protected attributes.

The exact filter order, initial sort, result card content, pagination, save interruption, locale routing and sponsorship placement are **not approved** by this record. Product and engineering must resolve them alongside the public disclosure contract before UI implementation. The broader seller and buyer journeys require their own targeted Mobbin passes.

## Follow-up acceptance evidence

- Test a first-time buyer's ability to state the active search, alter one criterion and return from a listing without losing context.
- Test no-results, loading, partial service failure and recoverable-error states.
- Verify a protected business identity does not leak through card text, images, filters, counts, metadata or HTML.
- Verify all search operations work with keyboard and mobile assistive technology.
- Validate proposed layouts with realistic business titles, unknown financial values, different currencies and long translations.
