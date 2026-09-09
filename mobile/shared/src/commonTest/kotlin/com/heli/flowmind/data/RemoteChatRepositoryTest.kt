package com.heli.flowmind.data

import com.tencent.kuikly.core.nvi.serialization.json.JSONObject
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue
import kotlin.coroutines.startCoroutine

class RemoteChatRepositoryTest {
    @Test
    fun `send maps Go response and adds identity correlation and JSON headers`() = runSuspend {
        val transport = FakeTransport(
            ChatHttpResponse(true, 200, body = json("""
                {"request_id":"server-request","user_message":${messageJson("u1", "user", "hello", "m1")},"assistant_message":${messageJson("a1", "assistant", "hi")}}
            """))
        )
        val repository = repository(transport)

        val result = repository.sendMessage("session-1", "hello", "m1")

        assertEquals("server-request", result.requestId)
        assertEquals("u1", result.userMessage.id)
        assertEquals("hi", result.assistantMessage.content)
        assertEquals(HttpMethod.POST, transport.requests.single().method)
        assertEquals("client-1", transport.requests.single().headers["X-Client-ID"])
        assertEquals("request-1", transport.requests.single().headers["X-Request-ID"])
        assertEquals("application/json", transport.requests.single().headers["Content-Type"])
        assertEquals("m1", transport.requests.single().body?.optString("client_message_id"))
    }

    @Test
    fun `Go error envelope retains code HTTP status and server request ID`() = runSuspend {
        val transport = FakeTransport(ChatHttpResponse(false, 504, body = json("""
            {"request_id":"server-timeout","error":{"code":"AI_TIMEOUT","message":"AI service timed out"}}
        """)))
        val error = assertFailsWith<ChatException> { repository(transport).createSession() }

        assertEquals("AI_TIMEOUT", error.code)
        assertEquals(504, error.httpStatus)
        assertEquals("server-timeout", error.requestId)
    }

    @Test
    fun `platform network wording maps to one shared connection message`() = runSuspend {
        val ios = assertFailsWith<ChatException> {
            repository(FakeTransport(ChatHttpResponse(false, null, errorMessage = "Could not connect to the server."))).listSessions()
        }
        val android = assertFailsWith<ChatException> {
            repository(FakeTransport(ChatHttpResponse(false, null, errorMessage = "io exception"))).listSessions()
        }

        assertEquals(ChatException.TRANSPORT_ERROR, ios.code)
        assertEquals("request-1", ios.requestId)
        assertEquals("网络连接失败，请检查网络后重试", ios.message)
        assertEquals(ios.message, android.message)
    }

    @Test
    fun `native URL error code is treated as transport failure`() = runSuspend {
        val error = assertFailsWith<ChatException> {
            repository(
                FakeTransport(
                    ChatHttpResponse(
                        success = false,
                        statusCode = -1001,
                        errorMessage = "The request timed out.",
                    ),
                ),
            ).listSessions()
        }

        assertEquals(ChatException.TRANSPORT_ERROR, error.code)
        assertEquals("网络连接失败，请检查网络后重试", error.message)
    }

    @Test
    fun `missing required success field is rejected`() = runSuspend {
        val transport = FakeTransport(ChatHttpResponse(true, 201, body = json("""
            {"id":"s1","title":"new","model_profile":"default","created_at":"now"}
        """)))
        val error = assertFailsWith<ChatException> { repository(transport).createSession() }

        assertEquals(ChatException.MALFORMED_RESPONSE, error.code)
        assertTrue(error.message.contains("updated_at"))
    }

    @Test
    fun `delete uses explicit DELETE and accepts 204 with no body`() = runSuspend {
        val transport = FakeTransport(ChatHttpResponse(true, 204))
        repository(transport).deleteSession("session-1")

        val request = transport.requests.single()
        assertEquals(HttpMethod.DELETE, request.method)
        assertEquals("http://10.0.2.2:8080/api/v1/sessions/session-1", request.url)
        assertEquals("client-1", request.headers["X-Client-ID"])
        assertEquals("request-1", request.headers["X-Request-ID"])
    }

    @Test
    fun `stored identity persists and request IDs are fresh`() {
        val store = MemoryStore()
        val first = StoredClientIdentity(store) { "123e4567-e89b-42d3-a456-426614174000" }.clientId()
        val second = StoredClientIdentity(store) { "123e4567-e89b-42d3-a456-426614174001" }.clientId()

        assertEquals("123e4567-e89b-42d3-a456-426614174000", first)
        assertEquals(first, second)
        assertNotEquals(RandomRequestIdGenerator.newRequestId(), RandomRequestIdGenerator.newRequestId())
    }

    @Test
    fun `generated client identity is a UUID accepted by the Go API`() {
        val identity = StoredClientIdentity(MemoryStore()).clientId()

        assertTrue(Regex("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$").matches(identity))
    }

    @Test
    fun `legacy non UUID identity is replaced once then persisted`() {
        val store = MemoryStore().apply { putString("flowmind.anonymous_client_id", "legacy-hex-id") }
        val identity = StoredClientIdentity(store) { "123e4567-e89b-42d3-a456-426614174000" }

        assertEquals("123e4567-e89b-42d3-a456-426614174000", identity.clientId())
        assertEquals("123e4567-e89b-42d3-a456-426614174000", identity.clientId())
    }

    @Test
    fun `existing UUID preserves its anonymous owner after normalization`() {
        val store = MemoryStore().apply { putString("flowmind.anonymous_client_id", "123E4567-E89B-42D3-A456-426614174000") }

        assertEquals("123e4567-e89b-42d3-a456-426614174000", StoredClientIdentity(store).clientId())
    }

    @Test
    fun `production endpoint requires HTTPS`() {
        assertFailsWith<IllegalArgumentException> { ChatApiEndpoints.production("http://api.example.test") }
        assertEquals("https://api.example.test/api/v1/sessions", ChatApiEndpoints.production("https://api.example.test/").url("/api/v1/sessions"))
        assertEquals("http://10.0.2.2:8080", ChatApiEndpoints.androidEmulator().baseUrl)
    }

    private fun repository(transport: ChatHttpTransport) = RemoteChatRepository(
        transport = transport,
        config = ChatApiEndpoints.androidEmulator(),
        clientIdentity = object : ClientIdentity { override fun clientId() = "client-1" },
        requestIdGenerator = object : RequestIdGenerator { override fun newRequestId() = "request-1" },
    )

    private fun messageJson(id: String, role: String, content: String, clientMessageId: String? = null): String = """
        {"id":"$id","role":"$role","content":"$content","status":"completed"${clientMessageId?.let { ",\"client_message_id\":\"$it\"" } ?: ""},"created_at":"2026-09-07T00:00:00Z"}
    """.trimIndent()

    private fun json(value: String) = JSONObject(value.trimIndent())

    private class FakeTransport(private val response: ChatHttpResponse) : ChatHttpTransport {
        val requests = mutableListOf<ChatHttpRequest>()
        override suspend fun execute(request: ChatHttpRequest): ChatHttpResponse {
            requests += request
            return response
        }
    }

    private class MemoryStore : KeyValueStore {
        private val values = mutableMapOf<String, String>()
        override fun getString(key: String): String = values[key].orEmpty()
        override fun putString(key: String, value: String) { values[key] = value }
    }
}

private fun runSuspend(block: suspend () -> Unit) {
    var failure: Throwable? = null
    block.startCoroutine(object : kotlin.coroutines.Continuation<Unit> {
        override val context = kotlin.coroutines.EmptyCoroutineContext
        override fun resumeWith(result: Result<Unit>) { failure = result.exceptionOrNull() }
    })
    failure?.let { throw it }
}
