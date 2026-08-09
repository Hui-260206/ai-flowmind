package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// New creates the minimal Gin-based HTTP server for the Go API.
func New() *http.Server {
	router := gin.New()
	router.GET("/healthz", healthz)

	return &http.Server{Handler: router}
}

func healthz(c *gin.Context) {
	c.String(http.StatusOK, "ok\n")
}
