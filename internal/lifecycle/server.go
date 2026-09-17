// Package lifecycle coordinates graceful HTTP shutdown without canceling active
// requests at the start of the drain period.
package lifecycle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// Serve owns listener and server. On cancellation it withdraws readiness, stops
// accepting connections, drains active requests, and finally cancels them if the
// deadline expires. Callers must not call Serve or Shutdown on the same server.
func Serve(ctx context.Context, server *http.Server, listener net.Listener, grace time.Duration, beginShutdown func()) error {
	if ctx == nil || server == nil || listener == nil || grace <= 0 || beginShutdown == nil {
		return errors.New("invalid HTTP lifecycle dependencies")
	}
	// Keep application context values while separating the drain signal from
	// cancellation of already accepted HTTP requests.
	requestContext, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRequests()
	server.BaseContext = func(net.Listener) context.Context { return requestContext }
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		beginShutdown()
		cancelRequests()
		return errors.Join(err, server.Close())
	case <-ctx.Done():
		beginShutdown()
		shutdownContext, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		shutdownError := server.Shutdown(shutdownContext)
		if shutdownError != nil {
			cancelRequests()
			closeError := server.Close()
			serveError := <-result
			if errors.Is(serveError, http.ErrServerClosed) {
				serveError = nil
			}
			return errors.Join(shutdownError, closeError, serveError)
		}
		err := <-result
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
