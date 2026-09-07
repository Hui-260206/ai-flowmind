package com.heli.flowmind.data

/**
 * 移动端会话边界。调用方不感知 Go 之外的服务、HTTP 实现或 JSON 格式。
 */
interface ChatRepository {
    suspend fun createSession(): ChatSession
    suspend fun listSessions(): List<ChatSession>
    suspend fun getMessages(sessionId: String): List<ChatMessage>
    suspend fun sendMessage(sessionId: String, content: String, clientMessageId: String): SendMessageResult
    suspend fun deleteSession(sessionId: String)
}

data class ChatSession(
    val id: String,
    val title: String,
    val modelProfile: String,
    val createdAt: String,
    val updatedAt: String,
)

/** The persisted representation returned by the Go API, not a transient UI bubble. */
data class ChatMessage(
    val id: String,
    val role: String,
    val content: String,
    val status: String,
    val clientMessageId: String? = null,
    val modelName: String? = null,
    val promptTokens: Int? = null,
    val completionTokens: Int? = null,
    val createdAt: String,
)

data class SendMessageResult(
    val requestId: String,
    val userMessage: ChatMessage,
    val assistantMessage: ChatMessage,
)

data class ChatApiError(
    val code: String,
    val message: String,
    val requestId: String? = null,
)

/** Stable error surface consumed by the ViewModel. */
class ChatException(
    val code: String,
    override val message: String,
    val httpStatus: Int? = null,
    val requestId: String? = null,
    cause: Throwable? = null,
) : Exception(message, cause) {
    companion object {
        const val TRANSPORT_ERROR = "TRANSPORT_ERROR"
        const val MALFORMED_RESPONSE = "MALFORMED_RESPONSE"
        const val INVALID_CLIENT_CONFIGURATION = "INVALID_CLIENT_CONFIGURATION"
    }
}
