package com.heli.flowmind.model

data class Message(
    val id: String,
    val serverId: String? = null,
    val role: MessageRole,
    val content: String,
    val status: MessageStatus = MessageStatus.SENT,
    val timestamp: Long = 0L,
    val clientMessageId: String? = null,
    val errorMessage: String? = null,
)

enum class MessageRole { SYSTEM, USER, ASSISTANT, TOOL }

enum class MessageStatus { SENDING, SENT, ERROR }
