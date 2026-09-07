package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-flowmind/services/go-api/internal/chat"
	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/grpcclient"
	"ai-flowmind/services/go-api/internal/httpserver"
	"ai-flowmind/services/go-api/internal/metrics"
	"ai-flowmind/services/go-api/internal/migrate"
	"ai-flowmind/services/go-api/internal/mysql"
	redisclient "ai-flowmind/services/go-api/internal/redis"
	"ai-flowmind/services/go-api/internal/repository"

	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration failed", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(cfg.Log.Level)}))
	db, err := mysql.Open(cfg.MySQL)
	if err != nil {
		logger.Error("open mysql failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("close mysql failed", "error", err)
		}
	}()
	if err := migrate.Up(db.SQLDB()); err != nil {
		logger.Error("run migrations failed", "error", err)
		os.Exit(1)
	}
	cache, err := redisclient.Open(cfg.Redis)
	if err != nil {
		logger.Error("open redis failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			logger.Error("close redis failed", "error", err)
		}
	}()
	grpcClient, err := grpcclient.Open(cfg.GRPC)
	if err != nil {
		logger.Error("open grpc client failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := grpcClient.Close(); err != nil {
			logger.Error("close grpc client failed", "error", err)
		}
	}()
	metricsRegistry := prometheus.NewRegistry()
	chatMetrics := metrics.New(metricsRegistry)
	completer, err := chat.NewGRPCCompleter(grpcClient, chatMetrics)
	if err != nil {
		logger.Error("create grpc chat completer failed", "error", err)
		os.Exit(1)
	}
	reliability, err := redisclient.NewReliabilityAdapter(cache, cfg.Redis.IdempotencyTTL, cfg.Redis.SessionLockTTL, cfg.Redis.OwnerRateLimitPerMinute, chatMetrics)
	if err != nil {
		logger.Error("create Redis chat reliability adapter failed", "error", err)
		os.Exit(1)
	}
	chatService, err := chat.New(chat.Dependencies{
		Sessions:    repository.NewSessionRepository(db.DB()),
		Messages:    repository.NewMessageRepository(db.DB()),
		Operations:  repository.NewSendOperationRepository(db.DB()),
		Completer:   completer,
		Reliability: reliability,
		Now:         time.Now,
		NewID:       chat.NewID,
		Metrics:     chatMetrics,
	})
	if err != nil {
		logger.Error("create chat service failed", "error", err)
		os.Exit(1)
	}
	apiServer := httpserver.New(cfg.HTTP, logger, httpserver.Dependencies{
		MySQL:             db.Check,
		Redis:             cache.Check,
		PythonGRPC:        grpcClient.Check,
		RequirePythonGRPC: true,
		Chat:              chatService,
		Metrics:           chatMetrics,
		MetricsRegistry:   metricsRegistry,
	})

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
