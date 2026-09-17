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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "api")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv, logger); err != nil {
		logger.Error("process_failed", "code", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, lookup func(string) string, logger *slog.Logger) error {
	cfg, err := config.Load(lookup, config.API)
	if err != nil {
		logger.Error("configuration_invalid", "reason", err.Error())
		return errors.New("configuration_invalid")
	}
	startup, cancelStartup := context.WithTimeout(ctx, cfg.StartupTimeout)
	database, err := store.Open(startup, cfg.DatabaseURL, "api")
	cancelStartup()
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("process_stopped", "phase", "startup")
			return nil
		}
		return errors.New("database_startup_failed")
	}
	defer database.Close()
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
	logger.Info("process_started", "authentication", "unavailable")
	err = lifecycle.Serve(ctx, server, listener, cfg.ShutdownTimeout, func() {
		handler.BeginShutdown()
		logger.Info("process_draining")
	})
	if err != nil {
		return errors.New("http_shutdown_or_serve_failed")
	}
	logger.Info("process_stopped")
	return nil
}
