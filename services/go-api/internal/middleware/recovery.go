package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// Recovery converts panics into the same public error envelope as other
// internal failures, while retaining the request ID in the log.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered", "request_id", GetRequestID(c), "panic", recovered)
				if !c.Writer.Written() {
					ErrorResponse(c, 500, "INTERNAL_ERROR", "internal server error")
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
