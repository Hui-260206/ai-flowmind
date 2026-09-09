package com.heli.flowmind.data

import kotlinx.coroutines.delay
import kotlinx.datetime.Clock

/** Deterministic in-memory repository for UI development and tests. */
class MockChatRepository : ChatRepository {
    private val sessions = linkedMapOf<String, ChatSession>()
    private val messagesBySession = mutableMapOf<String, MutableList<ChatMessage>>()
    private var nextSession = 1
    private var nextMessage = 1

    override suspend fun createSession(): ChatSession {
        val now = now()
        val session = ChatSession("mock-session-${nextSession++}", "新会话", "default", now, now)
        sessions[session.id] = session
        messagesBySession[session.id] = mutableListOf()
        return session
    }

    override suspend fun listSessions(): List<ChatSession> = sessions.values.sortedByDescending { it.updatedAt }

    override suspend fun getMessages(sessionId: String): List<ChatMessage> =
        messagesBySession[sessionId]?.toList() ?: throw missingSession()

    override suspend fun sendMessage(sessionId: String, content: String, clientMessageId: String): SendMessageResult {
        require(content.isNotBlank()) { "content must not be blank" }
        require(clientMessageId.isNotBlank()) { "clientMessageId must not be blank" }
        val history = messagesBySession[sessionId] ?: throw missingSession()
        delay(80)
        val existing = history.firstOrNull { it.clientMessageId == clientMessageId }
        if (existing != null) {
            val assistant = history.firstOrNull { it.id == "${existing.id}-assistant" }
                ?: throw ChatException(ChatException.MALFORMED_RESPONSE, "mock reply is missing")
            return SendMessageResult("mock-replay-$clientMessageId", existing, assistant)
        }
        val now = now()
        val user = ChatMessage("mock-message-${nextMessage++}", "user", content.trim(), "completed", clientMessageId, createdAt = now)
        val assistant = ChatMessage("${user.id}-assistant", "assistant", "你说的是：${user.content}", "completed", createdAt = now)
        history += user
        history += assistant
        sessions[sessionId]?.let { session ->
            val title = if (session.title == "新会话") {
                if (user.content.length <= 32) user.content else user.content.take(32) + "…"
            } else {
                session.title
            }
            sessions[sessionId] = session.copy(title = title, updatedAt = now)
        }
        return SendMessageResult("mock-$clientMessageId", user, assistant)
    }

    override suspend fun deleteSession(sessionId: String) {
        if (sessions.remove(sessionId) == null) throw missingSession()
        messagesBySession.remove(sessionId)
    }

    private fun missingSession() = ChatException("SESSION_NOT_FOUND", "session not found", httpStatus = 404)
    private fun now(): String = Clock.System.now().toString()
}
