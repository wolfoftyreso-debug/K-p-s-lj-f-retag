package httpapi

import (
	"log"
	"log/slog"
)

// ServerErrorLog records that the HTTP transport failed without echoing
// untrusted request material, remote addresses or panic payloads to telemetry.
func ServerErrorLog(logger *slog.Logger) *log.Logger {
	return log.New(transportLog{logger: logger}, "", 0)
}

type transportLog struct{ logger *slog.Logger }

func (l transportLog) Write(message []byte) (int, error) {
	l.logger.Error("http_transport_failed", "code", "transport_error")
	return len(message), nil
}
