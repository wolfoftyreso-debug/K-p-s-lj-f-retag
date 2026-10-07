package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/listings"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/store"
)

// ListingDraftStore resolves the current session and scoped permissions again
// inside each authoritative transaction. Client context is never authority.
type ListingDraftStore interface {
	CreateListingDraftSession(context.Context, []byte, listings.CreateDraftCommand) (listings.CreatedDraft, error)
	CreatePersonalListingDraftSession(context.Context, []byte, listings.CreateDraftCommand) (listings.CreatedDraft, error)
	ReadListingDraftSession(context.Context, []byte, string, string) (listings.Draft, error)
	UpdateListingDraftSession(context.Context, []byte, listings.UpdateDraftCommand) (listings.Draft, error)
	PreviewListingDraftSession(context.Context, []byte, string, string) (listings.PrivatePreview, error)
}

var draftFieldKeys = []string{
	"private_label", "activity_description", "country", "region", "title", "description",
	"sale_subject", "transfer_structure", "sale_context", "sale_method",
}

func (h *IdentityHandler) listingDraft(w http.ResponseWriter, r *http.Request, hash []byte) bool {
	personal := r.URL.Path == "/api/v1/personal-listing-drafts"
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	workspace := len(parts) >= 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "workspaces" && parts[4] == "listing-drafts"
	if !personal && !workspace {
		return false
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		h.failure(w, r, store.ErrInvalid)
		return true
	}
	var workspaceID, draftID string
	if workspace {
		workspaceID = parts[3]
		if len(workspaceID) != 36 || len(parts) > 7 || (len(parts) == 7 && parts[6] != "preview") {
			WriteError(w, r, http.StatusNotFound, "not_found")
			return true
		}
		if len(parts) >= 6 {
			draftID = parts[5]
			if len(draftID) != 36 {
				WriteError(w, r, http.StatusNotFound, "not_found")
				return true
			}
		}
	}
	if personal || len(parts) == 5 {
		if !method(w, r, http.MethodPost) {
			return true
		}
		var body struct {
			CommandID string `json:"command_id"`
			listings.Fields
		}
		keys := append([]string{"command_id"}, draftFieldKeys...)
		if err := decodeCommand(r, &body, keys...); err != nil {
			h.failure(w, r, err)
			return true
		}
		command := listings.CreateDraftCommand{WorkspaceID: workspaceID, CommandID: body.CommandID, Fields: body.Fields,
			RequestID: RequestID(r.Context()), CorrelationID: RequestID(r.Context())}
		var created listings.CreatedDraft
		var err error
		if personal {
			created, err = h.store.CreatePersonalListingDraftSession(r.Context(), hash, command)
		} else {
			created, err = h.store.CreateListingDraftSession(r.Context(), hash, command)
		}
		if err != nil {
			h.draftFailure(w, r, err)
			return true
		}
		status := http.StatusCreated
		if created.Replayed {
			status = http.StatusOK
		}
		writeJSON(w, status, created)
		return true
	}
	if len(parts) == 7 {
		if !method(w, r, http.MethodGet) {
			return true
		}
		preview, err := h.store.PreviewListingDraftSession(r.Context(), hash, workspaceID, draftID)
		if err != nil {
			h.failure(w, r, err)
			return true
		}
		writeJSON(w, http.StatusOK, preview)
		return true
	}
	var draft listings.Draft
	var err error
	switch r.Method {
	case http.MethodGet:
		draft, err = h.store.ReadListingDraftSession(r.Context(), hash, workspaceID, draftID)
	case http.MethodPut:
		var body struct {
			ExpectedBusinessVersion int64 `json:"expected_business_version"`
			ExpectedListingVersion  int64 `json:"expected_listing_version"`
			listings.Fields
		}
		keys := append([]string{"expected_business_version", "expected_listing_version"}, draftFieldKeys...)
		if err = decodeCommand(r, &body, keys...); err != nil {
			h.failure(w, r, err)
			return true
		}
		draft, err = h.store.UpdateListingDraftSession(r.Context(), hash, listings.UpdateDraftCommand{
			WorkspaceID: workspaceID, DraftID: draftID, ExpectedBusinessVersion: body.ExpectedBusinessVersion,
			ExpectedListingVersion: body.ExpectedListingVersion, Fields: body.Fields,
			RequestID: RequestID(r.Context()), CorrelationID: RequestID(r.Context()),
		})
	default:
		w.Header().Set("Allow", "GET, PUT")
		WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed")
		return true
	}
	if err != nil {
		h.failure(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, draft)
	return true
}

func (h *IdentityHandler) draftFailure(w http.ResponseWriter, r *http.Request, err error) {
	// Operational/COMMIT uncertainty takes precedence over a joined conflict.
	if errors.Is(err, store.ErrConflict) && !errors.Is(err, store.ErrUnavailable) {
		h.logger.WarnContext(r.Context(), "draft_request_failed", "code", "command_conflict", "request_id", RequestID(r.Context()))
		WriteError(w, r, http.StatusConflict, "command_conflict")
		return
	}
	h.failure(w, r, err)
}
