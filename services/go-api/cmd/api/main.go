package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/httpserver"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration failed", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(cfg.Log.Level)}))
	apiServer := httpserver.New(cfg.HTTP, logger, httpserver.Dependencies{})

	serverErrors := make(chan error, 1)
	go func() { serverErrors <- apiServer.Run() }()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdownSignal)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case signal := <-shutdownSignal:
		logger.Info("shutdown signal received", "signal", signal.String())
		ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := apiServer.Shutdown(ctx); err != nil {
			logger.Error("HTTP server graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("HTTP server stopped")
	}
}

func parseLogLevel(value string) slog.Level {
	switch value {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
