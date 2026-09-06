package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"unicode"

	"ai-flowmind/services/go-api/internal/requestid"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "request_id"

// RequestID 透传调用方提供的 request ID，缺失时生成一个新的。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if !validRequestID(requestID) {
			requestID = newRequestID()
		}
		c.Set(requestIDKey, requestID)
		c.Request = c.Request.WithContext(requestid.WithContext(c.Request.Context(), requestID))
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func GetRequestID(c *gin.Context) string {
	value, _ := c.Get(requestIDKey)
	requestID, _ := value.(string)
	return requestID
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		// crypto/rand 失败极其罕见；即使失败，非空 ID 仍能让请求在本进程内可追踪。
		return "request-unknown"
	}
	return hex.EncodeToString(bytes[:])
}

func ErrorResponse(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"request_id": GetRequestID(c),
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if unicode.IsSpace(char) || unicode.IsControl(char) {
			return false
		}
	}
	return true
}

// Errors 把未处理的 Gin 错误转换成 API 的基础错误包络。
func Errors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		ErrorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
