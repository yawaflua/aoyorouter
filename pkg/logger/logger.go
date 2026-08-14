package logger

import (
	"context"
	"log/slog"
	"os"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

type ContextLoggerKey struct{}

func InitLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envDev:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envProd:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	return log
}

func GetLoggerFromCtx(ctx context.Context) *slog.Logger {
	return ctx.Value(ContextLoggerKey{}).(*slog.Logger)
}

func SetLoggerToCtx(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, ContextLoggerKey{}, log)
}
