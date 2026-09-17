package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestWorkerProcessesAgainAfterFailureAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	calls := 0
	err := RunWorker(ctx, func(context.Context) (bool, error) {
		calls++
		switch calls {
		case 1:
			return false, errors.New("secret-canary")
		case 2:
			return true, nil
		default:
			cancel()
			return false, context.Canceled
		}
	}, time.Millisecond, logger)
	if err != nil || calls != 3 {
		t.Fatalf("retry failed: calls=%d err=%v", calls, err)
	}
	if !strings.Contains(logs.String(), "worker_processing_failed") || strings.Contains(logs.String(), "secret-canary") {
		t.Fatal("failure not logged safely")
	}
}

func TestWorkerCancellationInterruptsIdleWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- RunWorker(ctx, func(context.Context) (bool, error) { close(called); return false, nil }, time.Hour, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	}()
	<-called
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker cancellation waited for polling interval")
	}
}

func TestWorkerPropagatesCancellationIntoProcessing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- RunWorker(ctx, func(ctx context.Context) (bool, error) {
			close(called)
			<-ctx.Done()
			return false, ctx.Err()
		}, time.Second, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	}()
	<-called
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not propagate cancellation")
	}
}

func TestInvalidWorkerDependenciesRejected(t *testing.T) {
	if err := RunWorker(context.Background(), nil, 0, nil); err == nil {
		t.Fatal("invalid worker accepted")
	}
}

func TestWorkerObserverRetriesFailuresWithoutLeakingAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var logs bytes.Buffer
	calls := 0
	err := ObserveWorker(ctx, func(context.Context) error {
		calls++
		if calls == 1 {
			return errors.New("secret-canary")
		}
		cancel()
		return nil
	}, time.Millisecond, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil || calls != 2 || !strings.Contains(logs.String(), "queue_status_unavailable") || strings.Contains(logs.String(), "secret-canary") {
		t.Fatalf("observer did not recover safely: calls=%d err=%v", calls, err)
	}
}

func TestInvalidWorkerObserverDependenciesRejected(t *testing.T) {
	if err := ObserveWorker(context.Background(), nil, 0, nil); err == nil {
		t.Fatal("invalid worker observer accepted")
	}
}
