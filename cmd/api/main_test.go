package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestMalformedConfigurationFailsBeforeStartup(t *testing.T) {
	err := run(context.Background(), func(string) string { return "" }, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err == nil || err.Error() != "configuration_invalid" {
		t.Fatalf("expected deterministic configuration failure, got %v", err)
	}
}
