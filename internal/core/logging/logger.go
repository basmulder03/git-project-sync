package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Options struct {
	Level  string
	Format string
}

func New(opts Options) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	format := strings.TrimSpace(strings.ToLower(opts.Format))
	if format == "" {
		format = "json"
	}

	handlerOpts := &slog.HandlerOptions{Level: level}

	switch format {
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stdout, handlerOpts)), nil
	case "text":
		return slog.New(slog.NewTextHandler(os.Stdout, handlerOpts)), nil
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}
