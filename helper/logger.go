package helper

import (
	"log/slog"
	"os"
	"time"
)

func NewLogger(cfg *Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg != nil && cfg.Env == "development" {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// Persist timestamps in UTC
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.TimeValue(t.UTC())
				}
			}
			return a
		},
	}

	if cfg != nil && cfg.Env == "production" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

var Log *slog.Logger
