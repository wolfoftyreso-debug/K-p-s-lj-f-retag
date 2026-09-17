package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/config"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/httpapi"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/lifecycle"
	"github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "worker")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv, logger); err != nil {
		logger.Error("process_failed", "code", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, lookup func(string) string, logger *slog.Logger) error {
	cfg, err := config.Load(lookup, config.Worker)
	if err != nil {
		logger.Error("configuration_invalid", "reason", err.Error())
		return errors.New("configuration_invalid")
	}
	startup, cancelStartup := context.WithTimeout(ctx, cfg.StartupTimeout)
	database, err := store.Open(startup, cfg.DatabaseURL, "worker")
	cancelStartup()
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("process_stopped", "phase", "startup")
			return nil
		}
		return errors.New("database_startup_failed")
	}
	defer database.Close()
	processor, err := store.NewProcessor(database, store.WorkerOptions{LeaseDuration: cfg.WorkerLeaseDuration, RetryBase: cfg.WorkerRetryBase, MaxAttempts: cfg.WorkerMaxAttempts})
	if err != nil {
		return errors.New("worker_configuration_invalid")
	}
	handler, err := httpapi.New(httpapi.Options{Ready: database.Ping, Logger: logger, ReadinessTimeout: cfg.ReadinessTimeout, RequestTimeout: cfg.RequestTimeout, MaxRequestBody: cfg.MaxRequestBody})
	if err != nil {
		return errors.New("http_configuration_invalid")
	}
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.RequestTimeout,
		WriteTimeout: cfg.RequestTimeout + time.Second, IdleTimeout: cfg.IdleTimeout, MaxHeaderBytes: 8192,
		ErrorLog: httpapi.ServerErrorLog(logger),
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTPAddress)
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("process_stopped", "phase", "startup")
			return nil
		}
		return errors.New("http_listen_failed")
	}
	processContext, cancelProcess := context.WithCancel(ctx)
	defer cancelProcess()
	workerDone := make(chan error, 1)
	observerDone := make(chan error, 1)
	go func() {
		workerDone <- lifecycle.RunWorker(processContext, processor.ProcessNext, cfg.WorkerPollInterval, logger)
		cancelProcess()
	}()
	go func() {
		observerDone <- lifecycle.ObserveWorker(processContext, func(ctx context.Context) error {
			statusContext, cancel := context.WithTimeout(ctx, cfg.ReadinessTimeout)
			defer cancel()
			stats, err := database.QueueStats(statusContext)
			if err != nil {
				return err
			}
			logger.InfoContext(ctx, "worker_queue_status", "pending", stats.Pending, "processing", stats.Processing, "delivered", stats.Delivered, "dead", stats.Dead, "oldest_pending_seconds", stats.OldestPendingSeconds)
			if stats.Dead > 0 {
				logger.ErrorContext(ctx, "worker_dead_letters_present", "code", "dead_letters_require_review", "count", stats.Dead)
			}
			return nil
		}, cfg.WorkerStatusInterval, logger)
		cancelProcess()
	}()
	logger.Info("process_started", "delivery", "at_least_once")
	serverError := lifecycle.Serve(processContext, server, listener, cfg.ShutdownTimeout, func() {
		handler.BeginShutdown()
		logger.Info("process_draining")
	})
	cancelProcess()
	// Store operations inherit processContext and must return on cancellation;
	// wait before closing the pool so an interrupted transaction can roll back.
	workerError := <-workerDone
	observerError := <-observerDone
	if serverError != nil || workerError != nil || observerError != nil {
		return errors.New("worker_shutdown_or_serve_failed")
	}
	logger.Info("process_stopped")
	return nil
}
