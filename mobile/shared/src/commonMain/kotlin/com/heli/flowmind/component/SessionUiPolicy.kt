package com.heli.flowmind.component

import com.heli.flowmind.model.Message
import com.heli.flowmind.model.MessageRole
import com.heli.flowmind.model.MessageStatus
import com.heli.flowmind.state.LoadStatus

const val SESSION_MESSAGE_MAX_LENGTH = 32_000

internal enum class SessionContentMode {
    INITIAL_LOADING,
    INITIAL_ERROR,
    HISTORY_LOADING,
    HISTORY_ERROR,
    EMPTY,
    MESSAGES,
}

internal fun resolveSessionContentMode(
    bootstrapStatus: LoadStatus,
    historyStatus: LoadStatus,
    hasMessages: Boolean,
): SessionContentMode = when {
    bootstrapStatus == LoadStatus.LOADING || bootstrapStatus == LoadStatus.IDLE ->
        SessionContentMode.INITIAL_LOADING

    bootstrapStatus == LoadStatus.ERROR -> SessionContentMode.INITIAL_ERROR
    historyStatus == LoadStatus.LOADING -> SessionContentMode.HISTORY_LOADING
    historyStatus == LoadStatus.ERROR -> SessionContentMode.HISTORY_ERROR
    !hasMessages -> SessionContentMode.EMPTY
    else -> SessionContentMode.MESSAGES
}

internal data class MessageListTailState(
    val sessionId: String?,
    val messageId: String?,
    val status: MessageStatus?,
)

internal fun messageListTailState(sessionId: String?, messages: List<Message>): MessageListTailState {
    val tail = messages.lastOrNull()
    return MessageListTailState(sessionId, tail?.id, tail?.status)
}

internal fun Message.isRetryableAssistantFailure(): Boolean =
    role == MessageRole.ASSISTANT && status == MessageStatus.ERROR
