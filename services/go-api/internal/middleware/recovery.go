package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// Recovery 把 panic 转成与其他内部错误相同的对外错误包络，同时在日志里保留 request ID。
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
