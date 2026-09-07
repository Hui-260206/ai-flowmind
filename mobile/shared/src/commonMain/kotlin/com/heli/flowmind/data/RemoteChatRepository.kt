package com.heli.flowmind.data

import com.tencent.kuikly.core.nvi.serialization.json.JSONArray
import com.tencent.kuikly.core.nvi.serialization.json.JSONObject

/** Real implementation of the public Go session/message REST contract. */
class RemoteChatRepository(
    private val transport: ChatHttpTransport,
    private val config: ChatApiConfig,
    private val clientIdentity: ClientIdentity,
    private val requestIdGenerator: RequestIdGenerator = RandomRequestIdGenerator,
) : ChatRepository {
    override suspend fun createSession(): ChatSession = parseSession(request(HttpMethod.POST, "/api/v1/sessions").body)

    override suspend fun listSessions(): List<ChatSession> = parseItems(request(HttpMethod.GET, "/api/v1/sessions").body) { parseSession(it) }

    override suspend fun getMessages(sessionId: String): List<ChatMessage> =
        parseItems(request(HttpMethod.GET, "/api/v1/sessions/${pathSegment(sessionId)}/messages").body) { parseMessage(it) }

    override suspend fun sendMessage(sessionId: String, content: String, clientMessageId: String): SendMessageResult {
        requireNotBlank(sessionId, "sessionId")
        requireNotBlank(content, "content")
        requireNotBlank(clientMessageId, "clientMessageId")
        val body = JSONObject().apply { put("content", content); put("client_message_id", clientMessageId) }
        val response = request(HttpMethod.POST, "/api/v1/sessions/${pathSegment(sessionId)}/messages", body).body
        return SendMessageResult(
            requestId = requiredString(response, "request_id"),
            userMessage = parseMessage(requiredObject(response, "user_message")),
            assistantMessage = parseMessage(requiredObject(response, "assistant_message")),
        )
    }

    override suspend fun deleteSession(sessionId: String) {
        requireNotBlank(sessionId, "sessionId")
        request(HttpMethod.DELETE, "/api/v1/sessions/${pathSegment(sessionId)}", expectEmptySuccess = true)
    }

    private suspend fun request(method: HttpMethod, path: String, body: JSONObject? = null, expectEmptySuccess: Boolean = false): ChatHttpResponse {
        val requestId = requestIdGenerator.newRequestId()
        val clientId = clientIdentity.clientId()
        requireNotBlank(clientId, "clientId")
        val response = transport.execute(
            ChatHttpRequest(
                method = method,
                url = config.url(path),
                body = body,
                headers = buildMap {
                    put("X-Client-ID", clientId)
                    put("X-Request-ID", requestId)
                    if (body != null) put("Content-Type", "application/json")
                },
                timeoutSeconds = config.timeoutSeconds,
            )
        )
        if (!response.success || response.statusCode !in 200..299) {
            throw toChatException(response, requestId)
        }
        if (!expectEmptySuccess && response.body == null) {
            throw ChatException(ChatException.MALFORMED_RESPONSE, "response body is missing", response.statusCode, requestId)
        }
        return response
    }

    private fun toChatException(response: ChatHttpResponse, fallbackRequestId: String): ChatException {
        val body = response.body
        val requestId = body?.optString("request_id")?.takeIf { it.isNotBlank() }
            ?: response.headers["X-Request-ID"]?.takeIf { it.isNotBlank() }
            ?: fallbackRequestId
        val error = body?.optJSONObject("error")
        return if (error != null) {
            ChatException(error.optString("code").ifBlank { ChatException.TRANSPORT_ERROR }, error.optString("message").ifBlank { "chat request failed" }, response.statusCode, requestId)
        } else {
            // NetworkModule exposes platform-native wording here (for example,
            // iOS "Could not connect to the server." and Android "io exception").
            // Do not surface that unstable implementation detail in shared UI.
            ChatException(
                code = ChatException.TRANSPORT_ERROR,
                message = if (response.statusCode == null) {
                    "网络连接失败，请检查网络后重试"
                } else {
                    "服务请求失败，请稍后重试"
                },
                httpStatus = response.statusCode,
                requestId = requestId,
            )
        }
    }

    private fun parseSession(json: JSONObject?): ChatSession = ChatSession(
        id = requiredString(json, "id"), title = requiredString(json, "title"), modelProfile = requiredString(json, "model_profile"),
        createdAt = requiredString(json, "created_at"), updatedAt = requiredString(json, "updated_at"),
    )

    private fun parseMessage(json: JSONObject): ChatMessage = ChatMessage(
        id = requiredString(json, "id"), role = requiredString(json, "role"), content = requiredString(json, "content"),
        status = requiredString(json, "status"), clientMessageId = json.optionalString("client_message_id"), modelName = json.optionalString("model_name"),
        promptTokens = json.optionalInt("prompt_tokens"), completionTokens = json.optionalInt("completion_tokens"), createdAt = requiredString(json, "created_at"),
    )

    private fun <T> parseItems(json: JSONObject?, parser: (JSONObject) -> T): List<T> {
        val items: JSONArray = requiredArray(json, "items")
        return buildList {
            for (index in 0 until items.length()) {
                val item = items.optJSONObject(index) ?: throw malformed("items[$index]")
                add(parser(item))
            }
        }
    }

    private fun requiredString(json: JSONObject?, field: String): String = json?.optString(field)?.takeIf { it.isNotBlank() } ?: throw malformed(field)
    private fun requiredObject(json: JSONObject?, field: String): JSONObject = json?.optJSONObject(field) ?: throw malformed(field)
    private fun requiredArray(json: JSONObject?, field: String): JSONArray = json?.optJSONArray(field) ?: throw malformed(field)
    private fun JSONObject.optionalString(field: String): String? = optString(field).takeIf { it.isNotBlank() }
    private fun JSONObject.optionalInt(field: String): Int? = if (has(field)) optInt(field) else null
    private fun malformed(field: String): ChatException = ChatException(ChatException.MALFORMED_RESPONSE, "response field '$field' is missing")

    private fun requireNotBlank(value: String, name: String) {
        if (value.isBlank()) throw ChatException(ChatException.INVALID_CLIENT_CONFIGURATION, "$name must not be blank")
    }

    private fun pathSegment(value: String): String {
        if (value.isBlank() || value.any { it == '/' || it == '?' || it == '#' }) throw ChatException(ChatException.INVALID_CLIENT_CONFIGURATION, "invalid session ID")
        return value
    }
}
