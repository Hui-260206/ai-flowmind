package com.heli.flowmind.state

import com.heli.flowmind.data.ChatException
import com.heli.flowmind.data.KeyValueStore
import com.heli.flowmind.data.RandomRequestIdGenerator
import com.tencent.kuikly.core.module.SharedPreferencesModule

enum class LoadStatus { IDLE, LOADING, READY, ERROR }

enum class SendStatus { IDLE, SENDING, FAILED }

enum class SessionMutationStatus { IDLE, CREATING, DELETING }

enum class SessionErrorSource { BOOTSTRAP, HISTORY, SESSION_MUTATION, SEND }

data class SessionUiError(
    val source: SessionErrorSource,
    val code: String,
    val message: String,
    val requestId: String? = null,
)

data class PendingTurn(
    val sessionId: String,
    val content: String,
    val clientMessageId: String,
    val userUiId: String,
    val assistantUiId: String,
    val error: SessionUiError? = null,
)

fun interface ClientMessageIdGenerator {
    fun newClientMessageId(): String
}

object RandomClientMessageIdGenerator : ClientMessageIdGenerator {
    override fun newClientMessageId(): String = RandomRequestIdGenerator.newRequestId()
}

interface SelectedSessionStore {
    fun selectedSessionId(): String?
    fun saveSelectedSessionId(sessionId: String)
    fun clearSelectedSessionId()
}

class StoredSelectedSessionStore(
    private val store: KeyValueStore,
) : SelectedSessionStore {
    override fun selectedSessionId(): String? =
        store.getString(SELECTED_SESSION_ID_KEY).trim().takeIf { it.isNotEmpty() }

    override fun saveSelectedSessionId(sessionId: String) {
        store.putString(SELECTED_SESSION_ID_KEY, sessionId.trim())
    }

    override fun clearSelectedSessionId() {
        store.putString(SELECTED_SESSION_ID_KEY, "")
    }

    private companion object {
        const val SELECTED_SESSION_ID_KEY = "flowmind.selected_session_id"
    }
}

class SharedPreferencesSelectedSessionStore(
    preferences: SharedPreferencesModule,
) : SelectedSessionStore by StoredSelectedSessionStore(
    com.heli.flowmind.data.SharedPreferencesKeyValueStore(preferences),
)

object EmptySelectedSessionStore : SelectedSessionStore {
    override fun selectedSessionId(): String? = null
    override fun saveSelectedSessionId(sessionId: String) = Unit
    override fun clearSelectedSessionId() = Unit
}

internal fun ChatException.toSessionUiError(source: SessionErrorSource): SessionUiError =
    SessionUiError(source = source, code = code, message = message, requestId = requestId)
