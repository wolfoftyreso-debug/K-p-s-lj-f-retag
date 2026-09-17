package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testOptions(log io.Writer) Options {
	return Options{
		Ready: func(context.Context) error { return nil }, Logger: slog.New(slog.NewJSONHandler(log, nil)),
		ReadinessTimeout: 10 * time.Millisecond, RequestTimeout: 100 * time.Millisecond, MaxRequestBody: 32,
	}
}

func TestHealthChecksDependencyOnlyForReadiness(t *testing.T) {
	var calls atomic.Int32
	options := testOptions(io.Discard)
	options.Ready = func(context.Context) error { calls.Add(1); return errors.New("database secret-canary") }
	handler, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want int
	}{{"/health/live", 200}, {"/health/ready", 503}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != tc.want || strings.Contains(response.Body.String(), "secret-canary") {
			t.Fatalf("health response %d %s", response.Code, response.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("dependency called %d times", calls.Load())
	}
}

func TestReadinessHealthyAndDraining(t *testing.T) {
	handler, _ := New(testOptions(io.Discard))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != 200 {
		t.Fatal(response.Code)
	}
	handler.BeginShutdown()
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != 503 {
		t.Fatal(response.Code)
	}
}

func TestReadinessReceivesCancellation(t *testing.T) {
	options := testOptions(io.Discard)
	checked := make(chan error, 1)
	options.Ready = func(ctx context.Context) error { <-ctx.Done(); checked <- ctx.Err(); return ctx.Err() }
	handler, _ := New(options)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != 503 || !errors.Is(<-checked, context.DeadlineExceeded) {
		t.Fatal("readiness did not respect its deadline")
	}
}

func TestNormalCompositionFailsClosedAndMinimizesLogs(t *testing.T) {
	var logs bytes.Buffer
	handler, _ := New(testOptions(&logs))
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodPost} {
		request := httptest.NewRequest(method, "/api/v1/workspaces/secret-canary?token=secret-canary", strings.NewReader("secret-canary"))
		for _, name := range []string{"X-User-ID", "X-Actor-ID", "X-Workspace-ID", "X-Role", "X-Request-ID", "Authorization", "Cookie"} {
			request.Header.Set(name, "secret-canary")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 401 || !strings.Contains(response.Body.String(), "authentication_required") {
			t.Fatalf("protected request did not deny: %d %s", response.Code, response.Body.String())
		}
		if response.Header().Get("X-Request-ID") == "secret-canary" || len(response.Header().Get("X-Request-ID")) != 36 {
			t.Fatal("client request ID trusted")
		}
		if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("security headers absent")
		}
	}
	if strings.Contains(logs.String(), "secret-canary") {
		t.Fatal("request data leaked into logs")
	}
}

func TestUnknownRouteAndMethodHaveStableErrors(t *testing.T) {
	handler, _ := New(testOptions(io.Discard))
	for _, tc := range []struct {
		method, path, code string
		status             int
	}{{"GET", "/missing", "not_found", 404}, {"POST", "/health/live", "method_not_allowed", 405}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.code) {
			t.Fatalf("unexpected response %d %s", response.Code, response.Body.String())
		}
	}
}

func TestRequestBodyLimitAndRequestIDContext(t *testing.T) {
	options := testOptions(io.Discard)
	options.Protected = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(RequestID(r.Context())) != 36 {
			t.Error("missing request identifier in context")
		}
		_, err := io.ReadAll(r.Body)
		var tooLarge *http.MaxBytesError
		if !errors.As(err, &tooLarge) {
			t.Errorf("expected body limit error, got %v", err)
		}
		WriteError(w, r, http.StatusRequestEntityTooLarge, "body_too_large")
	})
	handler, _ := New(options)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", strings.NewReader(strings.Repeat("a", 33))))
	if response.Code != 413 {
		t.Fatal(response.Code)
	}
}

func TestRequestTimeoutCancelsHandler(t *testing.T) {
	options := testOptions(io.Discard)
	options.RequestTimeout = 15 * time.Millisecond
	cancelled := make(chan error, 1)
	options.Protected = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		cancelled <- r.Context().Err()
	})
	handler, _ := New(options)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil))
	if response.Code != 503 || !errors.Is(<-cancelled, context.DeadlineExceeded) {
		t.Fatal("request did not time out safely")
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatal("timeout response lost JSON content type")
	}
}

func TestPanicDoesNotLeakPayloadOrPartialResponse(t *testing.T) {
	var logs bytes.Buffer
	options := testOptions(&logs)
	options.Protected = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secret-canary"))
		panic("secret-canary")
	})
	handler, _ := New(options)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil))
	if response.Code != 500 || strings.Contains(response.Body.String()+logs.String(), "secret-canary") {
		t.Fatalf("unsafe panic response %d %s", response.Code, response.Body.String())
	}
}

func TestMissingDependenciesRejected(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("empty options accepted")
	}
}

func TestConnectionWriteFailureIsObservableWithoutPayload(t *testing.T) {
	var logs bytes.Buffer
	handler, _ := New(testOptions(&logs))
	handler.ServeHTTP(&failingWriter{headers: make(http.Header)}, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if !strings.Contains(logs.String(), "response_write_failed") || strings.Contains(logs.String(), "secret-canary") {
		t.Fatal("connection write failure was not observed safely")
	}
}

func TestTransportLogDoesNotLeakRawServerMessages(t *testing.T) {
	var logs bytes.Buffer
	ServerErrorLog(slog.New(slog.NewJSONHandler(&logs, nil))).Print("secret-canary")
	if !strings.Contains(logs.String(), "http_transport_failed") || strings.Contains(logs.String(), "secret-canary") {
		t.Fatal("transport error was not logged safely")
	}
}

type failingWriter struct{ headers http.Header }

func (w *failingWriter) Header() http.Header       { return w.headers }
func (w *failingWriter) WriteHeader(int)           {}
func (w *failingWriter) Write([]byte) (int, error) { return 0, errors.New("secret-canary") }
