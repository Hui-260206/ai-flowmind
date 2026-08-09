package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "request_id"

// RequestID propagates a caller-provided request ID or creates a fresh one.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if !validRequestID(requestID) {
			requestID = newRequestID()
		}
		c.Set(requestIDKey, requestID)
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
		// crypto/rand failure is exceptionally rare; a non-empty ID still makes
		// the request traceable in the same process.
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

// Errors converts unhandled Gin errors into the API's base error envelope.
func Errors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		ErrorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
