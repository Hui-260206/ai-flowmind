package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"ai-flowmind/services/go-api/internal/chat"
	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/health"
	"ai-flowmind/services/go-api/internal/metrics"
	"ai-flowmind/services/go-api/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Dependencies struct {
	MySQL      health.Check
	Redis      health.Check
	PythonGRPC health.Check
	// RequirePythonGRPC 控制 Python gRPC 是否为服务就绪的必要依赖。
	// 阶段 3 使用进程内 FakeCompleter，因此组合根保持 false；阶段 5 的 gRPC
	// 完成器接入后应设为 true。
	RequirePythonGRPC bool
	Chat              *chat.Service
	Metrics           *metrics.Collector
	MetricsRegistry   *prometheus.Registry
}
type Server struct{ httpServer *http.Server }

func New(cfg config.HTTPConfig, logger *slog.Logger, dependencies Dependencies) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Recovery(logger), middleware.Errors(), middleware.RequestLogger(logger))
	readinessDependencies := []health.Dependency{
		{Name: "mysql", Check: dependencies.MySQL},
		{Name: "redis", Check: dependencies.Redis},
	}
	if dependencies.RequirePythonGRPC {
		readinessDependencies = append(readinessDependencies, health.Dependency{Name: "python_grpc", Check: dependencies.PythonGRPC})
	}
	checker := health.NewChecker(readinessDependencies...)
	router.GET("/healthz", health.Healthz)
	router.GET("/readyz", checker.Handler())
	registry := dependencies.MetricsRegistry
	if registry == nil {
		registry = prometheus.NewRegistry()
	}
	collector := dependencies.Metrics
	if collector == nil {
		collector = metrics.New(registry)
	}
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))
	if dependencies.Chat != nil {
		handler := newChatHandler(dependencies.Chat, collector)
		sessions := router.Group("/api/v1/sessions")
		sessions.POST("", handler.createSession)
		sessions.GET("", handler.listSessions)
		sessions.DELETE("/:session_id", handler.deleteSession)
		sessions.GET("/:session_id/messages", handler.listMessages)
		sessions.POST("/:session_id/messages", handler.sendMessage)
	}
	router.NoRoute(func(c *gin.Context) { middleware.ErrorResponse(c, http.StatusNotFound, "NOT_FOUND", "route not found") })
	return &Server{httpServer: &http.Server{Addr: cfg.Addr, Handler: router, ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout}}
}
func (s *Server) Handler() http.Handler { return s.httpServer.Handler }
func (s *Server) Run() error {
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (s *Server) Shutdown(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
