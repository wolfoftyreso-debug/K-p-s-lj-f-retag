// Package config validates the explicit environment contract shared by the API
// and independently runnable worker. Errors never echo configuration values.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Process string

const (
	API    Process = "api"
	Worker Process = "worker"
)

type Config struct {
	DatabaseURL          string
	HTTPAddress          string
	StartupTimeout       time.Duration
	ShutdownTimeout      time.Duration
	ReadinessTimeout     time.Duration
	RequestTimeout       time.Duration
	ReadHeaderTimeout    time.Duration
	IdleTimeout          time.Duration
	WorkerPollInterval   time.Duration
	WorkerLeaseDuration  time.Duration
	WorkerRetryBase      time.Duration
	WorkerStatusInterval time.Duration
	WorkerMaxAttempts    int
	MaxRequestBody       int64
}

// Load accepts a lookup function to make configuration independent of process
// globals and testable without modifying the environment of parallel tests.
func Load(lookup func(string) string, process Process) (Config, error) {
	if lookup == nil || (process != API && process != Worker) {
		return Config{}, fmt.Errorf("invalid configuration source or process")
	}
	cfg := Config{DatabaseURL: lookup("DATABASE_URL"), HTTPAddress: lookup("HTTP_ADDR")}
	if err := ValidateDatabaseURL(cfg.DatabaseURL); err != nil {
		return Config{}, err
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddress)
	if err != nil {
		return Config{}, fmt.Errorf("HTTP_ADDR must contain a host and numeric port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, fmt.Errorf("HTTP_ADDR port must be between 1 and 65535")
	}
	fields := []struct {
		name         string
		value        *time.Duration
		defaultValue time.Duration
		minimum      time.Duration
		maximum      time.Duration
	}{
		{"STARTUP_TIMEOUT", &cfg.StartupTimeout, 10 * time.Second, time.Second, time.Minute},
		{"SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout, 15 * time.Second, time.Millisecond, time.Minute},
		{"READINESS_TIMEOUT", &cfg.ReadinessTimeout, time.Second, time.Millisecond, 10 * time.Second},
		{"REQUEST_TIMEOUT", &cfg.RequestTimeout, 10 * time.Second, time.Millisecond, time.Minute},
		{"READ_HEADER_TIMEOUT", &cfg.ReadHeaderTimeout, 5 * time.Second, time.Millisecond, 30 * time.Second},
		{"IDLE_TIMEOUT", &cfg.IdleTimeout, 60 * time.Second, time.Second, 5 * time.Minute},
		{"WORKER_POLL_INTERVAL", &cfg.WorkerPollInterval, time.Second, 10 * time.Millisecond, time.Minute},
		{"WORKER_LEASE_DURATION", &cfg.WorkerLeaseDuration, 30 * time.Second, time.Second, 5 * time.Minute},
		{"WORKER_RETRY_BASE", &cfg.WorkerRetryBase, time.Second, 10 * time.Millisecond, time.Minute},
		{"WORKER_STATUS_INTERVAL", &cfg.WorkerStatusInterval, 30 * time.Second, time.Second, 5 * time.Minute},
	}
	for _, field := range fields {
		value := field.defaultValue
		if raw := lookup(field.name); raw != "" {
			value, err = time.ParseDuration(raw)
			if err != nil || value < field.minimum || value > field.maximum {
				return Config{}, fmt.Errorf("%s must be a duration between %s and %s", field.name, field.minimum, field.maximum)
			}
		}
		*field.value = value
	}
	if cfg.ReadinessTimeout > cfg.RequestTimeout {
		return Config{}, fmt.Errorf("READINESS_TIMEOUT must not exceed REQUEST_TIMEOUT")
	}
	cfg.WorkerMaxAttempts = 5
	if raw := lookup("WORKER_MAX_ATTEMPTS"); raw != "" {
		cfg.WorkerMaxAttempts, err = strconv.Atoi(raw)
		if err != nil || cfg.WorkerMaxAttempts < 1 || cfg.WorkerMaxAttempts > 20 {
			return Config{}, fmt.Errorf("WORKER_MAX_ATTEMPTS must be an integer between 1 and 20")
		}
	}
	cfg.MaxRequestBody = 16 * 1024
	if raw := lookup("MAX_REQUEST_BODY_BYTES"); raw != "" {
		cfg.MaxRequestBody, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cfg.MaxRequestBody < 1 || cfg.MaxRequestBody > 1024*1024 {
			return Config{}, fmt.Errorf("MAX_REQUEST_BODY_BYTES must be an integer between 1 and 1048576")
		}
	}
	return cfg, nil
}

// ValidateDatabaseURL requires an explicit URI host, user, database and TLS
// policy. Migration commands share this validation while retaining a separate
// privileged database credential. Remaining pgx credential/port defaults are
// not replaced by this URI validator.
func ValidateDatabaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || parsed.User == nil || parsed.User.Username() == "" || strings.Trim(parsed.Path, "/") == "" || parsed.Fragment != "" {
		return fmt.Errorf("DATABASE_URL must be a PostgreSQL URI with explicit host, user and database")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query["sslmode"]) != 1 {
		return fmt.Errorf("DATABASE_URL must declare exactly one sslmode")
	}
	// pgx accepts identity overrides in URI query parameters. They could make
	// the actual host differ from the one whose loopback/TLS policy we checked.
	for _, key := range []string{"host", "port", "user", "password", "dbname", "database", "service", "servicefile"} {
		if _, exists := query[key]; exists {
			return fmt.Errorf("DATABASE_URL connection identity must be specified only in the URI authority and path")
		}
	}
	mode := query.Get("sslmode")
	ip := net.ParseIP(parsed.Hostname())
	loopback := strings.EqualFold(parsed.Hostname(), "localhost") || (ip != nil && ip.IsLoopback())
	if mode != "verify-full" && !(mode == "disable" && loopback) {
		return fmt.Errorf("DATABASE_URL requires sslmode=verify-full; explicit disable is allowed only on loopback for local tests")
	}
	return nil
}
