// Package httpapi supplies bounded HTTP handling and process health endpoints.
// Authentication is deliberately absent from normal Package A composition.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Options struct {
	Ready            func(context.Context) error
	Protected        http.Handler
	Logger           *slog.Logger
	ReadinessTimeout time.Duration
	RequestTimeout   time.Duration
	MaxRequestBody   int64
}

type Handler struct {
	options  Options
	draining atomic.Bool
}

type requestIDKey struct{}

// RequestID returns the server-generated correlation identifier. Client actor,
// tenant and correlation headers are never used to establish trusted context.
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func New(options Options) (*Handler, error) {
	if options.Ready == nil || options.Logger == nil || options.ReadinessTimeout <= 0 || options.RequestTimeout <= 0 || options.ReadinessTimeout > options.RequestTimeout || options.MaxRequestBody <= 0 {
		return nil, errors.New("invalid HTTP handler dependencies or limits")
	}
	return &Handler{options: options}, nil
}

// BeginShutdown causes readiness to fail before existing requests are drained.
func (h *Handler) BeginShutdown() { h.draining.Store(true) }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		h.options.Logger.Error("request_identifier_failed", "code", "random_source_failed")
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	// RFC 4122 version 4 identifier, also accepted by the audit UUID columns.
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	id := hex.EncodeToString(bytes[:])
	id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
	r.Body = http.MaxBytesReader(w, r.Body, h.options.MaxRequestBody)
	w.Header().Set("X-Request-ID", id)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	response := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		if recover() != nil {
			h.options.Logger.ErrorContext(r.Context(), "request_handler_failed", "code", "handler_panic", "request_id", id)
			WriteError(response, r, http.StatusInternalServerError, "internal_error")
		}
		if response.writeError != nil {
			h.options.Logger.WarnContext(r.Context(), "response_write_failed", "code", "connection_write_failed", "request_id", id)
		}
		h.options.Logger.InfoContext(r.Context(), "http_request", "route", routeName(r.URL.Path), "status", response.status, "request_id", id, "duration_ms", time.Since(started).Milliseconds())
	}()
	timeoutBody := fmt.Sprintf(`{"error":{"code":"request_timeout","request_id":"%s"}}`, id)
	http.TimeoutHandler(http.HandlerFunc(h.route), h.options.RequestTimeout, timeoutBody).ServeHTTP(response, r)
}

func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	switch routeName(r.URL.Path) {
	case "health.live", "health.ready":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if r.URL.Path == "/health/live" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
			return
		}
		if h.draining.Load() {
			WriteError(w, r, http.StatusServiceUnavailable, "not_ready")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), h.options.ReadinessTimeout)
		defer cancel()
		if err := h.options.Ready(ctx); err != nil {
			h.options.Logger.WarnContext(r.Context(), "readiness_check_failed", "code", "dependency_unavailable", "request_id", RequestID(r.Context()))
			WriteError(w, r, http.StatusServiceUnavailable, "not_ready")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	case "protected":
		if h.options.Protected == nil {
			WriteError(w, r, http.StatusUnauthorized, "authentication_required")
			return
		}
		h.options.Protected.ServeHTTP(w, r)
	default:
		WriteError(w, r, http.StatusNotFound, "not_found")
	}
}

// WriteError intentionally exposes only a stable error code and generated ID.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code string) {
	writeJSON(w, status, struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}{Error: struct {
		Code      string `json:"code"`
		RequestID string `json:"request_id"`
	}{Code: code, RequestID: RequestID(r.Context())}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	// All callers pass concrete string-only response types; marshal cannot fail.
	encoded, err := json.Marshal(body)
	if err != nil {
		panic("invalid response schema")
	}
	w.WriteHeader(status)
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		// No second response can be sent after a failed write. The outer status
		// writer observes actual connection errors; TimeoutHandler owns timeout
		// cancellation when writing to its response buffer fails instead.
		return
	}
}

func routeName(path string) string {
	switch path {
	case "/health/live":
		return "health.live"
	case "/health/ready":
		return "health.ready"
	}
	if path == "/api/v1" || strings.HasPrefix(path, "/api/v1/") {
		return "protected"
	}
	return "not_found"
}

type statusWriter struct {
	http.ResponseWriter
	status     int
	writeError error
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	// net/http.TimeoutHandler otherwise changes its timeout response to HTML.
	// This transport serves only JSON, including every failure path.
	w.Header().Set("Content-Type", "application/json")
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	if err != nil {
		w.writeError = err
	}
	return n, err
}
