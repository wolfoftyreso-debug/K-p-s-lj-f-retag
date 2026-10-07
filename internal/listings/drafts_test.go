package listings

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIncompleteDraftAndIndependentAxes(t *testing.T) {
	for _, f := range []Fields{{}, {SaleSubject: "asset_package"}, {TransferStructure: "share_sale"}, {SaleContext: "insolvency"}, {SaleMethod: "time_limited_bidding"},
		{SaleSubject: "operating_business", TransferStructure: "business_transfer", SaleContext: "succession", SaleMethod: "invitation_for_offers"}} {
		if _, err := f.Normalize(); err != nil {
			t.Fatalf("valid incomplete dimensions rejected: %v", err)
		}
	}
}

func TestNormalizeBoundsAndMalformedInput(t *testing.T) {
	for name, f := range map[string]Fields{
		"label limit": {PrivateLabel: strings.Repeat("ö", 121)}, "title limit": {Title: strings.Repeat("x", 161)},
		"activity limit": {ActivityDescription: strings.Repeat("x", 2001)}, "description limit": {Description: strings.Repeat("x", 8001)},
		"region limit": {Region: strings.Repeat("x", 121)}, "invalid utf8": {Title: string([]byte{0xff})},
		"title control": {Title: "one\ntwo"}, "embedded control": {Description: "text\x01"},
		"unicode control": {Description: "text\u0085tail"}, "invalid country": {Country: "123"},
		"line separator": {Title: "one\u2028two"}, "paragraph separator": {Title: "one\u2029two"},
		"country glyphs": {Country: "ÅÖ"}, "unknown subject": {SaleSubject: "company"},
		"unknown transfer": {TransferStructure: "asset"}, "unknown context": {SaleContext: "bankruptcy"}, "no auction engine": {SaleMethod: "auction"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.Normalize(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("malformed fields accepted: %v", err)
			}
		})
	}
	f, err := (Fields{PrivateLabel: strings.Repeat("ö", 120), Title: strings.Repeat("x", 160), Description: " first\r\nsecond\tline\rthird ", Country: " se "}).Normalize()
	if err != nil || f.Country != "SE" || f.Description != "first\nsecond\tline\nthird" {
		t.Fatalf("boundary normalization failed: %+v %v", f, err)
	}
}

func TestCommandFingerprintNormalizedFiniteFields(t *testing.T) {
	a, err := (Fields{Title: " Workshop ", Country: "se", Description: "a\r\nb"}).Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	b, err := (Fields{Title: "Workshop", Country: "SE", Description: "a\nb"}).Fingerprint()
	if err != nil || a != b {
		t.Fatal("same normalized request has different fingerprint", err)
	}
	c, err := (Fields{Title: "Different", Country: "SE", Description: "a\nb"}).Fingerprint()
	if err != nil || a == c {
		t.Fatal("changed request does not change fingerprint", err)
	}
}

func TestPrivatePreviewExplicitAllowlist(t *testing.T) {
	d := Draft{Business: Business{ID: "business-canary", WorkspaceID: "workspace-canary", PrivateLabel: "private-label-canary", Version: 3, Country: "SE"},
		Listing: ListingDraft{ID: "draft", BusinessID: "business-canary", WorkspaceID: "workspace-canary", Title: "Workshop", Version: 4, Status: "DRAFT"}}
	data, err := json.Marshal(d.Preview())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private_label", "private-label-canary", "business-canary", "workspace-canary", "owner", "session", "permission"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("preview leaked %s", forbidden)
		}
	}
	p := d.Preview()
	if p.BusinessVersion != 3 || p.ListingVersion != 4 || p.Title != "Workshop" || p.Status != "DRAFT" {
		t.Fatal("preview lost source provenance")
	}
}
