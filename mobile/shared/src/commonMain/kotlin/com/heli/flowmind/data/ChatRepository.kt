package com.heli.flowmind.data

import com.heli.flowmind.model.Message

/**
 * 与后端会话接口交互的抽象。
 * ViewModel 只依赖此接口，不关心是 mock 还是真实 HTTP。
 */
interface ChatRepository {
    /**
     * 发送当前会话历史，返回 assistant 的回复文本。
     * 失败时抛 [ChatException]。
     */
    suspend fun sendChat(history: List<Message>): String
}

// ---- 网络层 DTO（与后端 FastAPI 约定的 JSON 结构一一对应）----
// 后端起来后，序列化/反序列化就用这些，避免在 ViewModel 里拼 JSON。
data class ChatRequest(
    val messages: List<ChatMessageDto>
)

data class ChatMessageDto(
    val role: String,      // "user" / "assistant"
    val content: String
) {
    companion object {
        fun fromMessage(m: Message) = ChatMessageDto(
            role = if (m.role == com.heli.flowmind.model.MessageRole.USER) "user" else "assistant",
            content = m.content,
        )
    }
}

data class ChatResponse(
    val reply: String
)

class ChatException(message: String, cause: Throwable? = null) : Exception(message, cause)