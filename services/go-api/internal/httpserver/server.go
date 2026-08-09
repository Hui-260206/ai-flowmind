package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

// DependencyCheck verifies that one external dependency is available.
// Concrete clients are injected by later integration stages, keeping the
// readiness endpoint independent from their implementations.
type DependencyCheck func(context.Context) error

// Dependencies is the boundary for components used by HTTP routes. A nil
// check means that the dependency has not been configured yet and is reported
// as not ready.
type Dependencies struct {
	MySQL      DependencyCheck
	Redis      DependencyCheck
	PythonGRPC DependencyCheck
}

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

func New(cfg config.HTTPConfig, logger *slog.Logger, dependencies Dependencies) *Server {
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
	router.GET("/readyz", readyz(dependencies))
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

type readinessCheck struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type readinessResponse struct {
	Status string                    `json:"status"`
	Checks map[string]readinessCheck `json:"checks"`
}

// readyz returns whether all required external dependencies are available.
// Checks run concurrently so a slow dependency does not delay checks that can
// complete independently. The response is assembled in a fixed set of names
// so callers can identify the failing dependency without parsing an error.
func readyz(dependencies Dependencies) gin.HandlerFunc {
	checks := []struct {
		name  string
		check DependencyCheck
	}{
		{name: "mysql", check: dependencies.MySQL},
		{name: "redis", check: dependencies.Redis},
		{name: "python_grpc", check: dependencies.PythonGRPC},
	}

	return func(c *gin.Context) {
		results := make(map[string]readinessCheck, len(checks))
		var waitGroup sync.WaitGroup
		var mutex sync.Mutex

		for _, dependency := range checks {
			dependency := dependency
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				result := readinessCheck{Status: "ok"}
				if dependency.check == nil {
					result.Status = "failed"
					result.Error = "dependency check is not configured"
				} else if err := dependency.check(c.Request.Context()); err != nil {
					result.Status = "failed"
					result.Error = err.Error()
				}
				mutex.Lock()
				results[dependency.name] = result
				mutex.Unlock()
			}()
		}
		waitGroup.Wait()

		response := readinessResponse{Status: "ready", Checks: results}
		statusCode := http.StatusOK
		for _, result := range results {
			if result.Status != "ok" {
				response.Status = "not_ready"
				statusCode = http.StatusServiceUnavailable
				break
			}
		}
		c.JSON(statusCode, response)
	}
}
