package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Dependencies is the boundary for components used by HTTP routes. Health
// probes and business handlers can be added here without changing lifecycle
// code in cmd/api.
type Dependencies struct{}

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

func New(cfg config.HTTPConfig, logger *slog.Logger, _ Dependencies) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.Recovery(logger),
		middleware.Errors(),
		middleware.RequestLogger(logger),
	)
	router.GET("/healthz", healthz)
	router.NoRoute(func(c *gin.Context) {
		middleware.ErrorResponse(c, http.StatusNotFound, "NOT_FOUND", "route not found")
	})

	return &Server{
		logger: logger,
		httpServer: &http.Server{
			Addr:              cfg.Addr,
			Handler:           router,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
	}
}

func (s *Server) Handler() http.Handler { return s.httpServer.Handler }

func (s *Server) Run() error {
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func healthz(c *gin.Context) {
	c.String(http.StatusOK, "ok\n")
}
