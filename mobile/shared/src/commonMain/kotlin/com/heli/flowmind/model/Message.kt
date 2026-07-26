package com.heli.flowmind.model

data class Message(
    val id: String,
    val role: MessageRole,
    val content: String,
    val status: MessageStatus = MessageStatus.SENT,
    val timestamp: Long = 0L
)

enum class MessageRole { USER, ASSISTANT }

enum class MessageStatus { SENDING, SENT, ERROR }
