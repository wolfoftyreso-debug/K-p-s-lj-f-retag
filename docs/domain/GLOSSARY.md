# Canonical domain glossary

Status: Phase 0 terminology baseline derived from the master directive. This document defines vocabulary; it does not approve a database schema, legal interpretation, workflow, or access policy.

Canonical English terms apply to code, storage, API contracts, event names, identifiers, URLs, and telemetry. Localized interface labels may differ, but must map to these concepts. Language, country, currency, and jurisdiction are independent attributes.

## People, accounts, and authority

| Term | Definition and boundary |
| --- | --- |
| User | An application account representing a person. A User is independent of the authentication provider's subject identifier. An authenticated User is not necessarily identity-verified or authorized to sell. Account linking and exceptional non-person identities require a separate decision. |
| Organization | A party represented in the platform, such as a brokerage, law firm, operating company, or corporate group. It is not synonymous with a Business, Listing, or tenant boundary. How individuals participate without an Organization remains an owner decision. |
| Workspace | The proposed tenant and operational access boundary for a set of protected resources. A professional may participate in several Workspaces, including separate client engagements. Organization affiliation alone does not grant access to a Workspace. The client-engagement ownership model requires approval. |
| Membership | An explicit association between a User and a Workspace or Organization, with scope and lifecycle. Workspace access depends on the relevant active membership and permissions, not merely on knowing an identifier. Organization and Workspace memberships must be distinguishable. |
| Role | A named grouping of Permissions assigned in an explicit scope. The actual role catalogue is not yet approved; a role name must never imply unrestricted cross-workspace access. |
| Permission | A defined authorization capability evaluated by the application against actor, action, resource, scope, and relevant conditions. Possession of a Permission does not automatically satisfy a SellerMandate or disclosure requirement. |
| Seller | The party offering a SaleSubject through a Listing, either directly or through a representative. The represented party, acting User, managing Workspace, and relevant SellerMandate are distinct. A seller is a participation in a sale, not necessarily a permanent account role. |
| SellerMandate | A first-class record of a claimed and, where established, evidenced authority to act for a represented party in relation to a Business, SaleSubject, or Listing. Scope, validity, evidence, verifier, revocation, and limitations must be representable. Identity verification alone does not establish a mandate. Legal sufficiency and approval policy remain unresolved. |
| Buyer | A person or represented party seeking to acquire a SaleSubject. A visitor can browse without being authenticated. A User may participate as both Buyer and Seller in different contexts. |
| Verification | A scoped, time-bound assessment of a specific claim using an explicit method and outcome. Identity, organization existence, financial claims, and authority to sell are separate verification subjects. A generic badge must not imply that every claim was checked. |
| VerificationEvidence | A protected record or reference supporting a particular Verification. It has provenance, ownership, permitted use, access control, and retention requirements. Public verification claims do not expose the underlying evidence. |

## Businesses, listings, and sale dimensions

| Term | Definition and boundary |
| --- | --- |
| Business | An economic activity or operating undertaking described in the marketplace. It may span or occupy part of one or more legal entities; it is not itself necessarily a legal entity. A Business record is distinct from a particular advertising publication. |
| LegalEntity | A legally constituted entity, where relevant to the sale. Its jurisdiction and identifiers must be explicit. The platform Organization may represent a LegalEntity, but the two concepts are not interchangeable. No cross-workspace entity deduplication is assumed. |
| Listing | A managed marketplace offer describing a Business or other permitted SaleSubject, its sale dimensions, commercial information, disclosure, and publication lifecycle. A Listing is not the Business itself and does not transfer legal ownership. |
| SaleSubject | **What is offered:** a legal entity/company, complete operating business, business division, or selected assets/asset package. These are conceptual categories from the directive, not a claim that every combination is legally valid in every jurisdiction. Marketplace eligibility for standalone assets remains an owner decision. |
| TransferStructure | **How a transfer is intended to occur:** share sale or asset/business transfer. A proposed structure does not establish its legal validity or completion. |
| SaleContext | **Why, or in what setting, the sale occurs:** ordinary sale, succession, restructuring, insolvency/bankruptcy, or another explicitly modelled context. Context is independent of SaleSubject, TransferStructure, and SaleMethod. |
| SaleMethod | **How a Buyer is selected:** asking price, negotiation, invitation for offers, time-limited bidding process, or auction only after a real auction capability exists. Whether a Listing permits multiple methods or prices alongside negotiation remains unresolved. |
| ListingAsset | A structured reference describing an asset or asset category included in or excluded from the advertised subject. It does not prove title or constitute a transfer instrument. Media files are not ListingAssets merely because they are files. |
| ListingFinancial | A structured financial fact or seller assertion associated with a Listing and version. It identifies metric, value and unit/currency, period, definition where applicable, source, and verification state. Revenue and operating result must not lose their provenance or become unqualified headline numbers. |
| ListingVersion | An immutable snapshot of defined Listing content after a versioning event. It supports review, amendment history, public projection derivation, and process evidence. The exact content boundary and version-creation triggers require approval. |
| ListingMedia | A photo, video, or other presentation asset associated with a Listing. Original uploads and approved public derivatives have separate access and sanitization requirements. Media must not reveal protected identity through embedded metadata or names. |
| ListingDocument | A versioned association between a Listing and a protected document, including classification, ownership, processing state, and access policy. A document association does not make the file publicly accessible. |
| DisclosureLevel | A policy concept describing required conditions for access to specified information: Public, Registered, Verified, Approved, or NDARequired in the directive's examples. The exact combination and scope of conditions require approval. Unsupported workflows must deny access rather than pretend a requirement was fulfilled. |
| PublicListingProjection | A deliberately minimized, allowlisted representation eligible for anonymous access. It is separately derived from approved Listing content and disclosure decisions. It is not a serialization of the protected Listing with a few fields removed. |

No field named `sale_type` may substitute for the four distinct sale dimensions above. Concept names do not by themselves finalize enum wire values or database types.

## Discovery, communication, and notifications

| Term | Definition and boundary |
| --- | --- |
| SearchCriteria | A normalized and versioned expression of a discovery request, containing validated filter values and explicit sort semantics. It is independent of frontend URLs and search-engine syntax. |
| SearchPort | An application-owned contract for querying eligible public discovery projections. PostgreSQL and a future OpenSearch adapter may implement it. Neither adapter becomes authoritative for Listing state, permission, or protected content. |
| SavedListing | A User's explicit reference to a Listing for later retrieval. Saving does not grant access to protected information or guarantee that the Listing remains available. |
| SavedSearch | A first-class entity containing normalized SearchCriteria, an owner, a name, and notification preferences/status. It can be edited or deleted and notifications can be paused. Criteria are not stored solely as an opaque frontend URL. |
| Inquiry | An initial expression of interest or contact request concerning a Listing. Submission, delivery, visibility, and spam controls require explicit policy. It does not constitute an Offer or evidence of a binding commitment. |
| Conversation | An access-controlled communication context involving explicitly authorized participants, potentially initiated by an Inquiry. Access to a Listing does not automatically grant access to its Conversations. |
| Notification | A record or delivery intention concerning a platform event for a permitted recipient and channel. Notification eligibility, locale, preference, deduplication, and delivery status are distinct from the underlying business event. |

## Commercial services and process evidence

| Term | Definition and boundary |
| --- | --- |
| PlatformPayment | A payment to this platform for a platform service, such as publication, promotion, or subscription. It is not the consideration paid to acquire a Business; business purchase settlement and escrow are outside the initial scope. |
| Entitlement | A recorded right to use a platform service, granted under an explicit commercial policy. A payment-provider event may be evidence for a grant but must not create duplicate grants when delivered repeatedly. Refund and revocation consequences require policy decisions. |
| Promotion | A purchased or otherwise explicitly authorized increase in labelled visibility for a Listing, connected to campaign, placement, schedule, package/budget, delivery, and status. It is separate from organic relevance. |
| Advertisement | A labelled advertising unit or placement with its own advertiser, content, eligibility, and delivery controls. Not every Advertisement is a Listing Promotion. Measurement must distinguish observed events from estimates. |
| InsolvencyCase | A record of a declared insolvency-related process, jurisdictional context, appointed professional, authority evidence, relevant subjects, and factual process events. It does not establish a legal conclusion or imply wrongdoing. |
| OfferProcess | A defined process for soliciting and considering Offers, with applicable rules, explicit deadlines and timezone semantics, amendments, information releases, interested parties, and closure evidence. A time-limited OfferProcess is not automatically an auction. |
| Offer | A recorded submission by an identified participant within an OfferProcess, with version and timing evidence. Binding effect, permitted changes/withdrawal, confidentiality, and admissibility are unresolved legal/product policy, not inferred from the word “Offer.” |
| AuditEvent | A deliberately structured, durable record of who acted, what action was attempted/performed, on what subject, when, in what context, and with what result. It is distinct from an application log or a domain integration event and must not contain uncontrolled secrets or document contents. |
| DomainEvent | A named occurrence in the application domain used to communicate a completed change, such as `ListingPublished`. Events triggering asynchronous or external effects use the transactional outbox. Their payloads are minimized and versioned. |
| OutboxRecord | A transactionally stored dispatch record for a DomainEvent committed with the authoritative state change. Delivery may be repeated; consumers must tolerate repeats. It is not proof that an external side effect has completed. |

## Usage rules

- Use **Business** for the economic operation, **LegalEntity** for the legal entity, **Listing** for the marketplace publication, **Organization** for a represented platform party, and **Seller** for a party's sale participation.
- Use **Workspace** for the proposed protected-resource access boundary. Business ownership, file ownership, tenant ownership, and authority to act are different relationships.
- Use a specific verification claim, scope, date, and outcome wherever trust is communicated; avoid an unexplained “verified business” label.
- Use **SaleSubject**, **TransferStructure**, **SaleContext**, and **SaleMethod** independently in contracts and analysis.
- Use “proposed,” “claimed,” “recorded,” or “verified for this claim” when those qualifiers are material. Do not turn an assertion into a proven legal fact through naming.
- New terminology changes must update this glossary and the affected contracts together. Renaming an established term requires a compatibility/migration decision; translation changes do not rename canonical domain concepts.
