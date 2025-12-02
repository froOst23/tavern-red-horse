package utils

import (
	"log/slog"
	"os"
)

func SetupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case "debug":
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case "info":
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	case "warn":
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	case "error":
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	default:
		panic("Log level must be one of: debug, info, warn, error")
	}

	return log
}
