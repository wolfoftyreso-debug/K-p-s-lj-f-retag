// Package listings owns private business-sale drafting rules. A draft neither
// establishes seller authority nor implies publication or commercial verification.
package listings

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrInvalid = errors.New("listing draft: invalid fields")

// Fields is a finite draft contract. Empty values mean unset. Business facts and
// listing presentation have separate persistent identities and versions.
type Fields struct {
	PrivateLabel        string `json:"private_label"`
	ActivityDescription string `json:"activity_description"`
	Country             string `json:"country"`
	Region              string `json:"region"`
	Title               string `json:"title"`
	Description         string `json:"description"`
	SaleSubject         string `json:"sale_subject"`
	TransferStructure   string `json:"transfer_structure"`
	SaleContext         string `json:"sale_context"`
	SaleMethod          string `json:"sale_method"`
}

// Normalize validates supplied values without requiring publication completeness.
// It makes CRLF/CR line endings, surrounding whitespace and country casing
// deterministic for persistence and command fingerprints. No sale combination
// is interpreted as lawful, verified or publication-ready.
func (f Fields) Normalize() (Fields, error) {
	for _, field := range []struct {
		value *string
		limit int
		lines bool
	}{
		{&f.PrivateLabel, 120, false}, {&f.ActivityDescription, 2000, true},
		{&f.Country, 2, false}, {&f.Region, 120, false},
		{&f.Title, 160, false}, {&f.Description, 8000, true},
		{&f.SaleSubject, 40, false}, {&f.TransferStructure, 40, false},
		{&f.SaleContext, 40, false}, {&f.SaleMethod, 40, false},
	} {
		if !utf8.ValidString(*field.value) {
			return Fields{}, ErrInvalid
		}
		v := *field.value
		if field.lines {
			v = strings.ReplaceAll(strings.ReplaceAll(v, "\r\n", "\n"), "\r", "\n")
		}
		v = strings.TrimSpace(v)
		if utf8.RuneCountInString(v) > field.limit {
			return Fields{}, ErrInvalid
		}
		for _, r := range v {
			if (unicode.IsControl(r) && !(field.lines && (r == '\n' || r == '\t'))) || r == '\u2028' || r == '\u2029' {
				return Fields{}, ErrInvalid
			}
		}
		*field.value = v
	}
	f.Country = strings.ToUpper(f.Country)
	if f.Country != "" && (len(f.Country) != 2 || f.Country[0] < 'A' || f.Country[0] > 'Z' || f.Country[1] < 'A' || f.Country[1] > 'Z') {
		return Fields{}, ErrInvalid
	}
	if !oneOf(f.SaleSubject, "", "legal_entity", "operating_business", "business_division", "asset_package") ||
		!oneOf(f.TransferStructure, "", "share_sale", "business_transfer") ||
		!oneOf(f.SaleContext, "", "ordinary_sale", "succession", "restructuring", "insolvency", "other") ||
		!oneOf(f.SaleMethod, "", "asking_price", "negotiation", "invitation_for_offers", "time_limited_bidding") {
		return Fields{}, ErrInvalid
	}
	return f, nil
}

func oneOf(v string, values ...string) bool {
	for _, candidate := range values {
		if v == candidate {
			return true
		}
	}
	return false
}

// Fingerprint is over the finite normalized contract, not an arbitrary request
// body. Request/correlation IDs and transport formatting are intentionally absent.
func (f Fields) Fingerprint() ([32]byte, error) {
	normalized, err := f.Normalize()
	if err != nil {
		return [32]byte{}, err
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(body), nil
}

type CreateDraftCommand struct {
	WorkspaceID, CommandID, RequestID, CorrelationID string
	Fields                                           Fields
}

// UpdateDraftCommand replaces all finite fields. Both source versions are
// required; only aggregates whose normalized fields changed gain a revision.
type UpdateDraftCommand struct {
	WorkspaceID, DraftID, RequestID, CorrelationID  string
	ExpectedBusinessVersion, ExpectedListingVersion int64
	Fields                                          Fields
}

// CreatedDraft is the immutable create outcome. Replays never combine initial
// version numbers with subsequently modified fields; GET retrieves current state.
type CreatedDraft struct {
	WorkspaceID     string `json:"workspace_id"`
	BusinessID      string `json:"business_id"`
	ListingID       string `json:"listing_id"`
	BusinessVersion int64  `json:"business_version"`
	ListingVersion  int64  `json:"listing_version"`
	Replayed        bool   `json:"replayed"`
}

type Business struct {
	ID                  string    `json:"id"`
	WorkspaceID         string    `json:"workspace_id"`
	PrivateLabel        string    `json:"private_label"`
	ActivityDescription string    `json:"activity_description"`
	Country             string    `json:"country"`
	Region              string    `json:"region"`
	Version             int64     `json:"version"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ListingDraft struct {
	ID                string    `json:"id"`
	WorkspaceID       string    `json:"workspace_id"`
	BusinessID        string    `json:"business_id"`
	Status            string    `json:"status"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	SaleSubject       string    `json:"sale_subject"`
	TransferStructure string    `json:"transfer_structure"`
	SaleContext       string    `json:"sale_context"`
	SaleMethod        string    `json:"sale_method"`
	Version           int64     `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Draft struct {
	Business Business     `json:"business"`
	Listing  ListingDraft `json:"listing"`
}

// PrivatePreview is an explicit protected allowlist, never a public listing.
// Free text may itself identify a business and is not certified anonymous.
type PrivatePreview struct {
	ListingID           string `json:"listing_id"`
	BusinessVersion     int64  `json:"business_version"`
	ListingVersion      int64  `json:"listing_version"`
	Status              string `json:"status"`
	ActivityDescription string `json:"activity_description"`
	Country             string `json:"country"`
	Region              string `json:"region"`
	Title               string `json:"title"`
	Description         string `json:"description"`
	SaleSubject         string `json:"sale_subject"`
	TransferStructure   string `json:"transfer_structure"`
	SaleContext         string `json:"sale_context"`
	SaleMethod          string `json:"sale_method"`
}

func (d Draft) Preview() PrivatePreview {
	return PrivatePreview{ListingID: d.Listing.ID, BusinessVersion: d.Business.Version, ListingVersion: d.Listing.Version,
		Status: d.Listing.Status, ActivityDescription: d.Business.ActivityDescription, Country: d.Business.Country, Region: d.Business.Region,
		Title: d.Listing.Title, Description: d.Listing.Description, SaleSubject: d.Listing.SaleSubject,
		TransferStructure: d.Listing.TransferStructure, SaleContext: d.Listing.SaleContext, SaleMethod: d.Listing.SaleMethod}
}
