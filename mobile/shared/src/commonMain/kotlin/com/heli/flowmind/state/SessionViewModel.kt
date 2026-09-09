package com.heli.flowmind.state

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import com.heli.flowmind.data.ChatException
import com.heli.flowmind.data.ChatMessage
import com.heli.flowmind.data.ChatRepository
import com.heli.flowmind.data.ChatRepositoryProvider
import com.heli.flowmind.data.ChatSession
import com.heli.flowmind.data.SendMessageResult
import com.heli.flowmind.model.Message
import com.heli.flowmind.model.MessageRole
import com.heli.flowmind.model.MessageStatus
import com.tencent.kuikly.lifecycle.ViewModel
import com.tencent.kuikly.lifecycle.viewModelScope
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.datetime.Clock
import kotlinx.datetime.Instant

class SessionViewModel(
    private val repository: ChatRepository = ChatRepositoryProvider.repository,
    private val selectedSessionStore: SelectedSessionStore = EmptySelectedSessionStore,
    private val clientMessageIdGenerator: ClientMessageIdGenerator = RandomClientMessageIdGenerator,
    private val scopeOverride: CoroutineScope? = null,
) : ViewModel() {
    var inputText by mutableStateOf("")
        private set

    val sessions = mutableStateListOf<ChatSession>()
    val messages = mutableStateListOf<Message>()

    var currentSessionId by mutableStateOf<String?>(null)
        private set

    var bootstrapStatus by mutableStateOf(LoadStatus.IDLE)
        private set

    var historyStatus by mutableStateOf(LoadStatus.IDLE)
        private set

    var sendStatus by mutableStateOf(SendStatus.IDLE)
        private set

    var sessionMutationStatus by mutableStateOf(SessionMutationStatus.IDLE)
        private set

    var error by mutableStateOf<SessionUiError?>(null)
        private set

    var pendingTurn by mutableStateOf<PendingTurn?>(null)
        private set

    private var historyGeneration = 0L
    private val pendingTurnsBySession = mutableMapOf<String, PendingTurn>()
    private val scope: CoroutineScope
        get() = scopeOverride ?: viewModelScope

    val currentSession: ChatSession?
        get() = sessions.firstOrNull { it.id == currentSessionId }

    val currentTitle: String
        get() = currentSession?.title ?: "会话"

    val isSending: Boolean
        get() = sendStatus == SendStatus.SENDING

    val canSend: Boolean
        get() = canEditInput && inputText.isNotBlank()

    val canEditInput: Boolean
        get() = bootstrapStatus == LoadStatus.READY &&
            historyStatus == LoadStatus.READY &&
            currentSessionId != null &&
            !isSending &&
            sessionMutationStatus == SessionMutationStatus.IDLE &&
            pendingTurn == null

    val canManageSessions: Boolean
        get() = bootstrapStatus == LoadStatus.READY && !isSending && sessionMutationStatus == SessionMutationStatus.IDLE

    fun onInputChange(text: String) {
        inputText = text
    }

    fun initialize() {
        if (bootstrapStatus == LoadStatus.LOADING || bootstrapStatus == LoadStatus.READY || isSending) return
        bootstrapStatus = LoadStatus.LOADING
        historyStatus = LoadStatus.IDLE
        error = null
        invalidateHistory()
        scope.launch {
            try {
                val listed = repository.listSessions()
                replaceSessions(listed)
                val selected = selectedSessionStore.selectedSessionId()
                    ?.let { remembered -> listed.firstOrNull { it.id == remembered } }
                    ?: listed.firstOrNull()
                    ?: repository.createSession().also { replaceSessions(listOf(it)) }
                selectLoadedSession(selected)
                bootstrapStatus = LoadStatus.READY
                loadHistoryInternal(selected.id)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (throwable: Throwable) {
                bootstrapStatus = LoadStatus.ERROR
                error = throwable.toUiError(SessionErrorSource.BOOTSTRAP)
            }
        }
    }

    fun retryInitialization() = initialize()

    fun retryHistory() {
        val sessionId = currentSessionId ?: return
        if (isSending) return
        scope.launch { loadHistoryInternal(sessionId) }
    }

    fun createSession() {
        if (!canManageSessions) return
        sessionMutationStatus = SessionMutationStatus.CREATING
        error = null
        scope.launch {
            try {
                val session = repository.createSession()
                sessions.add(0, session)
                selectLoadedSession(session)
                invalidateHistory()
                messages.clear()
                historyStatus = LoadStatus.READY
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (throwable: Throwable) {
                error = throwable.toUiError(SessionErrorSource.SESSION_MUTATION)
            } finally {
                sessionMutationStatus = SessionMutationStatus.IDLE
            }
        }
    }

    fun selectSession(sessionId: String) {
        if (!canManageSessions || sessionId == currentSessionId) return
        val session = sessions.firstOrNull { it.id == sessionId } ?: return
        selectLoadedSession(session)
        messages.clear()
        error = null
        scope.launch { loadHistoryInternal(session.id) }
    }

    fun deleteSession(sessionId: String) {
        if (!canManageSessions || sessions.none { it.id == sessionId }) return
        sessionMutationStatus = SessionMutationStatus.DELETING
        error = null
        scope.launch {
            try {
                repository.deleteSession(sessionId)
                pendingTurnsBySession.remove(sessionId)
                val localRemaining = sessions.filterNot { it.id == sessionId }
                val remaining = try {
                    repository.listSessions()
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (_: Throwable) {
                    localRemaining
                }
                replaceSessions(remaining)
                if (sessionId != currentSessionId) return@launch

                invalidateHistory()
                messages.clear()
                val next = remaining.firstOrNull() ?: repository.createSession().also {
                    replaceSessions(listOf(it))
                }
                selectLoadedSession(next)
                loadHistoryInternal(next.id)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (throwable: Throwable) {
                error = throwable.toUiError(SessionErrorSource.SESSION_MUTATION)
            } finally {
                sessionMutationStatus = SessionMutationStatus.IDLE
            }
        }
    }

    fun sendMessage() {
        val sessionId = currentSessionId ?: return
        val content = inputText.trim()
        if (!canSend || content.isEmpty()) return

        val clientMessageId = clientMessageIdGenerator.newClientMessageId()
        val turn = PendingTurn(
            sessionId = sessionId,
            content = content,
            clientMessageId = clientMessageId,
            userUiId = "pending-user-$clientMessageId",
            assistantUiId = "pending-assistant-$clientMessageId",
        )
        rememberPendingTurn(turn)
        appendOptimisticTurn(turn)
        inputText = ""
        performSend(turn)
    }

    fun retryPendingTurn() {
        val turn = pendingTurn ?: return
        if (isSending || sessionMutationStatus != SessionMutationStatus.IDLE || currentSessionId != turn.sessionId) return
        val assistantIndex = messages.indexOfFirst { it.id == turn.assistantUiId }
        if (assistantIndex >= 0) {
            messages[assistantIndex] = messages[assistantIndex].copy(
                content = "等待模型回答……",
                status = MessageStatus.SENDING,
                errorMessage = null,
            )
        } else {
            messages.add(
                Message(
                    id = turn.assistantUiId,
                    role = MessageRole.ASSISTANT,
                    content = "等待模型回答……",
                    status = MessageStatus.SENDING,
                )
            )
        }
        rememberPendingTurn(turn.copy(error = null))
        performSend(turn)
    }

    private fun performSend(turn: PendingTurn) {
        if (isSending) return
        sendStatus = SendStatus.SENDING
        scope.launch {
            error = null
            try {
                val result = repository.sendMessage(turn.sessionId, turn.content, turn.clientMessageId)
                if (currentSessionId == turn.sessionId) applySuccessfulSend(turn, result)
                clearPendingTurn(turn.sessionId)
                sendStatus = SendStatus.IDLE
                refreshSessionsAfterSend(turn.sessionId)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (throwable: Throwable) {
                reconcileFailedSend(turn, throwable.toUiError(SessionErrorSource.SEND))
            }
        }
    }

    private suspend fun reconcileFailedSend(turn: PendingTurn, sendError: SessionUiError) {
        if (currentSessionId != turn.sessionId) {
            rememberPendingTurn(turn.copy(error = sendError))
            sendStatus = SendStatus.FAILED
            error = sendError
            return
        }
        try {
            val history = repository.getMessages(turn.sessionId)
            if (currentSessionId != turn.sessionId) return
            val matchedIndex = history.indexOfLast { it.role == "user" && it.clientMessageId == turn.clientMessageId }
            val matchingAssistant = history.getOrNull(matchedIndex + 1)?.takeIf { it.role == "assistant" }
            replaceMessages(history.map(::toUiMessage))
            if (matchedIndex >= 0 && matchingAssistant != null) {
                clearPendingTurn(turn.sessionId)
                sendStatus = SendStatus.IDLE
                error = null
                refreshSessionsAfterSend(turn.sessionId)
                return
            }
            if (matchedIndex >= 0 && matchedIndex == history.lastIndex) {
                val persistedUser = history[matchedIndex]
                val recovered = turn.copy(userUiId = persistedUser.id, error = sendError)
                rememberPendingTurn(recovered)
                messages.add(failedAssistantMessage(recovered, sendError))
            } else {
                restoreOptimisticFailure(turn, sendError)
            }
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Throwable) {
            restoreOptimisticFailure(turn, sendError)
        }
        sendStatus = SendStatus.FAILED
        error = sendError
    }

    private fun restoreOptimisticFailure(turn: PendingTurn, sendError: SessionUiError) {
        if (messages.none { it.id == turn.userUiId }) {
            messages.add(
                Message(
                    id = turn.userUiId,
                    role = MessageRole.USER,
                    content = turn.content,
                    status = MessageStatus.SENT,
                    clientMessageId = turn.clientMessageId,
                )
            )
        }
        val failed = failedAssistantMessage(turn, sendError)
        val assistantIndex = messages.indexOfFirst { it.id == turn.assistantUiId }
        if (assistantIndex >= 0) messages[assistantIndex] = failed else messages.add(failed)
        rememberPendingTurn(turn.copy(error = sendError))
    }

    private fun appendOptimisticTurn(turn: PendingTurn) {
        val timestamp = Clock.System.now().toEpochMilliseconds()
        messages.add(
            Message(
                id = turn.userUiId,
                role = MessageRole.USER,
                content = turn.content,
                status = MessageStatus.SENT,
                timestamp = timestamp,
                clientMessageId = turn.clientMessageId,
            )
        )
        messages.add(
            Message(
                id = turn.assistantUiId,
                role = MessageRole.ASSISTANT,
                content = "等待模型回答……",
                status = MessageStatus.SENDING,
                timestamp = timestamp,
            )
        )
    }

    private fun applySuccessfulSend(turn: PendingTurn, result: SendMessageResult) {
        replaceMessage(turn.userUiId, toUiMessage(result.userMessage))
        replaceMessage(turn.assistantUiId, toUiMessage(result.assistantMessage))
    }

    private fun replaceMessage(id: String, message: Message) {
        val index = messages.indexOfFirst { it.id == id }
        if (index >= 0) messages[index] = message else messages.add(message)
    }

    private suspend fun refreshSessionsAfterSend(sessionId: String) {
        try {
            val refreshed = repository.listSessions()
            replaceSessions(refreshed)
            if (currentSessionId == sessionId) {
                refreshed.firstOrNull { it.id == sessionId }?.let(::selectLoadedSession)
            }
        } catch (_: Throwable) {
            // Message persistence succeeded. Stale session metadata is non-fatal.
        }
    }

    private suspend fun loadHistoryInternal(sessionId: String) {
        val generation = ++historyGeneration
        historyStatus = LoadStatus.LOADING
        error = null
        try {
            val history = repository.getMessages(sessionId)
            if (!isCurrentHistoryRequest(sessionId, generation)) return
            replaceMessages(history.map(::toUiMessage))
            val storedTurn = pendingTurnsBySession[sessionId]
            if (storedTurn == null) {
                recoverTrailingTurn(sessionId, history)
            } else {
                reconcileStoredTurn(storedTurn, history)
            }
            historyStatus = LoadStatus.READY
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (throwable: Throwable) {
            if (!isCurrentHistoryRequest(sessionId, generation)) return
            historyStatus = LoadStatus.ERROR
            error = throwable.toUiError(SessionErrorSource.HISTORY)
        }
    }

    private fun recoverTrailingTurn(sessionId: String, history: List<ChatMessage>) {
        val last = history.lastOrNull()
        if (last?.role != "user" || last.clientMessageId.isNullOrBlank()) {
            clearPendingTurn(sessionId)
            if (sendStatus != SendStatus.SENDING) sendStatus = SendStatus.IDLE
            return
        }
        val recoveryError = SessionUiError(
            source = SessionErrorSource.SEND,
            code = "ASSISTANT_REPLY_MISSING",
            message = "回复未完成，请重试",
        )
        val recovered = PendingTurn(
            sessionId = sessionId,
            content = last.content,
            clientMessageId = last.clientMessageId,
            userUiId = last.id,
            assistantUiId = "recovery-assistant-${last.clientMessageId}",
            error = recoveryError,
        )
        rememberPendingTurn(recovered)
        sendStatus = SendStatus.FAILED
        messages.add(failedAssistantMessage(recovered, recoveryError))
    }

    private fun reconcileStoredTurn(turn: PendingTurn, history: List<ChatMessage>) {
        val matchedIndex = history.indexOfLast { it.role == "user" && it.clientMessageId == turn.clientMessageId }
        val matchingAssistant = history.getOrNull(matchedIndex + 1)?.takeIf { it.role == "assistant" }
        if (matchedIndex >= 0 && matchingAssistant != null) {
            clearPendingTurn(turn.sessionId)
            sendStatus = SendStatus.IDLE
            return
        }

        val failure = turn.error ?: SessionUiError(
            source = SessionErrorSource.SEND,
            code = "ASSISTANT_REPLY_MISSING",
            message = "回复未完成，请重试",
        )
        val restored = if (matchedIndex == history.lastIndex && matchedIndex >= 0) {
            turn.copy(userUiId = history[matchedIndex].id, error = failure)
        } else {
            turn.copy(error = failure)
        }
        restoreOptimisticFailure(restored, failure)
        sendStatus = SendStatus.FAILED
    }

    private fun failedAssistantMessage(turn: PendingTurn, failure: SessionUiError) = Message(
        id = turn.assistantUiId,
        role = MessageRole.ASSISTANT,
        content = failure.message,
        status = MessageStatus.ERROR,
        errorMessage = failure.message,
    )

    private fun selectLoadedSession(session: ChatSession) {
        currentSessionId = session.id
        pendingTurn = pendingTurnsBySession[session.id]
        selectedSessionStore.saveSelectedSessionId(session.id)
    }

    private fun rememberPendingTurn(turn: PendingTurn) {
        pendingTurnsBySession[turn.sessionId] = turn
        if (currentSessionId == turn.sessionId) pendingTurn = turn
    }

    private fun clearPendingTurn(sessionId: String) {
        pendingTurnsBySession.remove(sessionId)
        if (currentSessionId == sessionId) pendingTurn = null
    }

    private fun replaceSessions(newSessions: List<ChatSession>) {
        sessions.clear()
        sessions.addAll(newSessions)
    }

    private fun replaceMessages(newMessages: List<Message>) {
        messages.clear()
        messages.addAll(newMessages)
    }

    private fun invalidateHistory() {
        historyGeneration++
    }

    private fun isCurrentHistoryRequest(sessionId: String, generation: Long): Boolean =
        currentSessionId == sessionId && historyGeneration == generation

    private fun toUiMessage(message: ChatMessage): Message = Message(
        id = message.id,
        serverId = message.id,
        role = when (message.role) {
            "system" -> MessageRole.SYSTEM
            "user" -> MessageRole.USER
            "tool" -> MessageRole.TOOL
            else -> MessageRole.ASSISTANT
        },
        content = message.content,
        status = when (message.status) {
            "pending" -> MessageStatus.SENDING
            "failed" -> MessageStatus.ERROR
            else -> MessageStatus.SENT
        },
        timestamp = runCatching { Instant.parse(message.createdAt).toEpochMilliseconds() }.getOrDefault(0L),
        clientMessageId = message.clientMessageId,
    )

    private fun Throwable.toUiError(source: SessionErrorSource): SessionUiError = when (this) {
        is ChatException -> toSessionUiError(source)
        else -> SessionUiError(source, "INTERNAL_ERROR", message ?: "操作失败，请稍后重试")
    }

}
