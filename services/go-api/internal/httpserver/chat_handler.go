package httpserver

import (
	"errors"
	"net/http"
	"time"

	"ai-flowmind/services/go-api/internal/chat"
	"ai-flowmind/services/go-api/internal/middleware"
	"ai-flowmind/services/go-api/internal/model"

	"github.com/gin-gonic/gin"
)

const clientIDHeader = "X-Client-ID"

type chatHandler struct{ service *chat.Service }

type sessionResponse struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	ModelProfile string    `json:"model_profile"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type messageResponse struct {
	ID               string    `json:"id"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Status           string    `json:"status"`
	ClientMessageID  *string   `json:"client_message_id,omitempty"`
	ModelName        *string   `json:"model_name,omitempty"`
	PromptTokens     *int      `json:"prompt_tokens,omitempty"`
	CompletionTokens *int      `json:"completion_tokens,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type sendMessageRequest struct {
	Content         string `json:"content"`
	ClientMessageID string `json:"client_message_id"`
	ModelProfile    string `json:"model_profile"`
}

func newChatHandler(service *chat.Service) *chatHandler { return &chatHandler{service: service} }

func (h *chatHandler) createSession(c *gin.Context) {
	result, err := h.service.CreateSession(c.Request.Context(), chat.CreateSessionInput{ClientID: c.GetHeader(clientIDHeader)})
	if err != nil {
		writeChatError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toSessionResponse(result.Session))
}

func (h *chatHandler) listSessions(c *gin.Context) {
	result, err := h.service.ListSessions(c.Request.Context(), chat.ListSessionsInput{ClientID: c.GetHeader(clientIDHeader)})
	if err != nil {
		writeChatError(c, err)
		return
	}
	items := make([]sessionResponse, 0, len(result.Sessions))
	for _, session := range result.Sessions {
		items = append(items, toSessionResponse(session))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *chatHandler) deleteSession(c *gin.Context) {
	err := h.service.DeleteSession(c.Request.Context(), chat.SessionInput{
		ClientID: c.GetHeader(clientIDHeader), SessionID: c.Param("session_id"),
	})
	if err != nil {
		writeChatError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *chatHandler) listMessages(c *gin.Context) {
	result, err := h.service.ListMessages(c.Request.Context(), chat.SessionInput{
		ClientID: c.GetHeader(clientIDHeader), SessionID: c.Param("session_id"),
	})
	if err != nil {
		writeChatError(c, err)
		return
	}
	items := make([]messageResponse, 0, len(result.Messages))
	for _, message := range result.Messages {
		items = append(items, toMessageResponse(message))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *chatHandler) sendMessage(c *gin.Context) {
	var request sendMessageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid JSON request body")
		return
	}
	result, err := h.service.SendMessage(c.Request.Context(), chat.SendMessageInput{
		ClientID:        c.GetHeader(clientIDHeader),
		SessionID:       c.Param("session_id"),
		Content:         request.Content,
		ClientMessageID: request.ClientMessageID,
		ModelProfile:    request.ModelProfile,
	})
	if err != nil {
		writeChatError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"request_id":        middleware.GetRequestID(c),
		"user_message":      toMessageResponse(result.UserMessage),
		"assistant_message": toMessageResponse(result.AssistantMessage),
	})
}

func writeChatError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, chat.ErrInvalidArgument):
		middleware.ErrorResponse(c, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request argument")
	case errors.Is(err, chat.ErrSessionNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, "SESSION_NOT_FOUND", "session not found")
	case errors.Is(err, chat.ErrDuplicateRequest):
		middleware.ErrorResponse(c, http.StatusConflict, "DUPLICATE_REQUEST", "duplicate client message ID")
	case errors.Is(err, chat.ErrSessionBusy):
		middleware.ErrorResponse(c, http.StatusConflict, "SESSION_BUSY", "chat session is busy")
	case errors.Is(err, chat.ErrRateLimited):
		middleware.ErrorResponse(c, http.StatusTooManyRequests, "RATE_LIMITED", "chat request rate limit exceeded")
	case errors.Is(err, chat.ErrRedisUnavailable):
		middleware.ErrorResponse(c, http.StatusServiceUnavailable, "REDIS_UNAVAILABLE", "chat reliability service is unavailable")
	case errors.Is(err, chat.ErrAITimeout):
		middleware.ErrorResponse(c, http.StatusGatewayTimeout, "AI_TIMEOUT", "AI service timed out")
	case errors.Is(err, chat.ErrAIUnavailable):
		middleware.ErrorResponse(c, http.StatusBadGateway, "AI_UNAVAILABLE", "AI service is unavailable")
	case errors.Is(err, chat.ErrAIProvider):
		middleware.ErrorResponse(c, http.StatusBadGateway, "AI_PROVIDER_ERROR", "AI provider error")
	case errors.Is(err, chat.ErrAICompletion):
		middleware.ErrorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to complete chat message")
	default:
		middleware.ErrorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func toSessionResponse(session model.Session) sessionResponse {
	return sessionResponse{
		ID: session.ID, Title: session.Title, ModelProfile: session.ModelProfile,
		CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	}
}

func toMessageResponse(message model.Message) messageResponse {
	return messageResponse{
		ID: message.ID, Role: string(message.Role), Content: message.Content, Status: string(message.Status),
		ClientMessageID: message.ClientMessageID, ModelName: message.ModelName,
		PromptTokens: message.PromptTokens, CompletionTokens: message.CompletionTokens,
		CreatedAt: message.CreatedAt,
	}
}
