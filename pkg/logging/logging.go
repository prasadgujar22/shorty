// Package logging configures structured JSON logs on stdout via log/slog.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/prasadgujar22/shorty/pkg/buildinfo"
)

// New returns a JSON logger tagged with the service name and version, and
// installs it as the slog default. level is one of debug|info|warn|error.
func New(service, level string) *slog.Logger {
	return newWithWriter(os.Stdout, service, level)
}

func newWithWriter(w io.Writer, service, level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// Loki/Grafana conventions: "ts" and "msg" are easier to query.
			if a.Key == slog.TimeKey {
				a.Key = "ts"
			}
			return a
		},
	})
	l := slog.New(h).With("service", service, "version", buildinfo.Version)
	slog.SetDefault(l)
	return l
}
