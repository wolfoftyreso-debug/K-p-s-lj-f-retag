package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests exercise the same run function that main wraps with OS signals.
// They require the migrated source test database and its actual restricted role.
func TestRuntimeStartsServesHealthAndStopsAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_API_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_INTEGRATION") == "1" {
			t.Fatal("required real PostgreSQL TEST_API_DATABASE_URL is not configured")
		}
		t.Skip("real PostgreSQL runtime test not run: TEST_API_DATABASE_URL is required")
	}
	address := unusedLoopbackAddress(t)
	values := map[string]string{
		"DATABASE_URL": dsn, "HTTP_ADDR": address, "SHUTDOWN_TIMEOUT": "1s",
		"WORKER_POLL_INTERVAL": "10ms", "WORKER_STATUS_INTERVAL": "1s",
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	var runError error
	go func() {
		defer close(stopped)
		runError = run(ctx, func(key string) string { return values[key] }, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("runtime did not stop after cancellation")
		}
	})
	client := &http.Client{Timeout: time.Second}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	for !ready {
		select {
		case <-stopped:
			t.Fatalf("runtime failed before readiness: %v", runError)
		case <-deadline.C:
			t.Fatal("runtime did not become ready")
		case <-ticker.C:
			response, err := client.Get("http://" + address + "/health/ready")
			if err == nil {
				ready = response.StatusCode == http.StatusOK
				_, readError := io.Copy(io.Discard, response.Body)
				closeError := response.Body.Close()
				if readError != nil || closeError != nil {
					t.Fatal("readiness response failed")
				}
			}
		}
	}
	for _, endpoint := range []struct {
		path   string
		status int
	}{{"/health/live", 200}, {"/api/v1/workspaces/guessed", 401}} {
		request, err := http.NewRequest(http.MethodGet, "http://"+address+endpoint.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Actor-ID", "synthetic-forged-actor")
		request.Header.Set("X-Workspace-ID", "synthetic-forged-workspace")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("runtime probe failed")
		}
		body, readError := io.ReadAll(io.LimitReader(response.Body, 4096))
		closeError := response.Body.Close()
		if readError != nil || closeError != nil || response.StatusCode != endpoint.status || strings.Contains(string(body), "synthetic-forged") {
			t.Fatalf("runtime probe did not enforce its contract: status=%d", response.StatusCode)
		}
	}
	cancel()
	select {
	case <-stopped:
		if runError != nil {
			t.Fatalf("graceful stop failed: %v", runError)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime cancellation did not finish")
	}
}

func TestUnavailableDatabaseFailsStartupWithoutLeakingConfiguration(t *testing.T) {
	address := unusedLoopbackAddress(t)
	values := map[string]string{
		"DATABASE_URL": "postgres://synthetic:secret-canary@" + address + "/foundation?sslmode=disable",
		"HTTP_ADDR":    unusedLoopbackAddress(t), "STARTUP_TIMEOUT": "1s",
	}
	var logs bytes.Buffer
	err := run(context.Background(), func(key string) string { return values[key] }, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err == nil || err.Error() != "database_startup_failed" || strings.Contains(logs.String(), "secret-canary") {
		t.Fatalf("database failure was not deterministic and safe: %v", err)
	}
}

func unusedLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func TestCancellationDuringStartupStopsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	values := map[string]string{
		"DATABASE_URL": "postgres://synthetic@" + unusedLoopbackAddress(t) + "/foundation?sslmode=disable",
		"HTTP_ADDR":    unusedLoopbackAddress(t),
	}
	if err := run(ctx, func(key string) string { return values[key] }, slog.New(slog.NewJSONHandler(io.Discard, nil))); err != nil {
		t.Fatalf("startup cancellation was not graceful: %v", err)
	}
}
