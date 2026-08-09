package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/health"
	"ai-flowmind/services/go-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	MySQL      health.Check
	Redis      health.Check
	PythonGRPC health.Check
}
type Server struct{ httpServer *http.Server }

func New(cfg config.HTTPConfig, logger *slog.Logger, dependencies Dependencies) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Recovery(logger), middleware.Errors(), middleware.RequestLogger(logger))
	checker := health.NewChecker(health.Dependency{Name: "mysql", Check: dependencies.MySQL}, health.Dependency{Name: "redis", Check: dependencies.Redis}, health.Dependency{Name: "python_grpc", Check: dependencies.PythonGRPC})
	router.GET("/healthz", health.Healthz)
	router.GET("/readyz", checker.Handler())
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
