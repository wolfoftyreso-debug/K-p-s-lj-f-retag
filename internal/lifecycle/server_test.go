package lifecycle

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func listenerForTest(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func TestGracefulShutdownDrainsActiveRequest(t *testing.T) {
	listener := listenerForTest(t)
	type contextKey struct{}
	entered, release, draining := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(contextKey{}) != "synthetic-context" {
			t.Error("application context values were lost")
		}
		close(entered)
		select {
		case <-r.Context().Done():
			t.Error("active request canceled before drain")
		case <-release:
		}
		w.WriteHeader(http.StatusNoContent)
	})}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "synthetic-context"))
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- Serve(ctx, server, listener, time.Second, func() { close(draining) }) }()
	result := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			_, readError := io.Copy(io.Discard, response.Body)
			err = errors.Join(readError, response.Body.Close())
		}
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	<-draining
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}

func TestShutdownDeadlineCancelsRemainingRequest(t *testing.T) {
	listener := listenerForTest(t)
	entered, cancelled := make(chan struct{}), make(chan struct{})
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(cancelled)
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- Serve(ctx, server, listener, 20*time.Millisecond, func() {}) }()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		client := &http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	if err := <-stopped; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected drain deadline, got %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("remaining request was not canceled")
	}
	<-requestDone
}

func TestListenerFailurePropagates(t *testing.T) {
	listener := listenerForTest(t)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	err := Serve(context.Background(), &http.Server{ReadHeaderTimeout: time.Second}, listener, time.Second, func() {})
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected listener error, got %v", err)
	}
}

func TestInvalidServerDependenciesRejected(t *testing.T) {
	if err := Serve(context.Background(), nil, nil, 0, nil); err == nil {
		t.Fatal("invalid server accepted")
	}
}
