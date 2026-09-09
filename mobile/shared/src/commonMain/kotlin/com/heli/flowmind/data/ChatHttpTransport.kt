package com.heli.flowmind.data

import com.tencent.kuikly.core.module.NetworkModule
import com.tencent.kuikly.core.nvi.serialization.json.JSONObject
import kotlin.coroutines.resume
import kotlin.coroutines.suspendCoroutine

enum class HttpMethod { GET, POST, DELETE }

data class ChatHttpRequest(
    val method: HttpMethod,
    val url: String,
    val body: JSONObject? = null,
    val headers: Map<String, String>,
    val timeoutSeconds: Int,
)

data class ChatHttpResponse(
    val success: Boolean,
    val statusCode: Int?,
    val headers: Map<String, String> = emptyMap(),
    val body: JSONObject? = null,
    val errorMessage: String = "",
)

interface ChatHttpTransport { suspend fun execute(request: ChatHttpRequest): ChatHttpResponse }

/**
 * Adapter for Kuikly's `KRNetworkModule`. GET/POST use the same native entry as
 * DELETE; the explicit `method` field bypasses the public Boolean convenience API.
 */
class NetworkModuleChatHttpTransport(private val networkModule: NetworkModule) : ChatHttpTransport {
    override suspend fun execute(request: ChatHttpRequest): ChatHttpResponse = suspendCoroutine { continuation ->
        val params = JSONObject().apply {
            put("url", request.url)
            put("method", request.method.name)
            request.body?.let { put("param", it) }
            put("headers", JSONObject().also { headers -> request.headers.forEach { (key, value) -> headers.put(key, value) } })
            put("timeout", request.timeoutSeconds)
        }
        networkModule.toNative(false, "httpRequest", params.toString(), callback = { result ->
            if (result == null) {
                val response = ChatHttpResponse(false, null, errorMessage = "network module returned no response")
                continuation.resume(response)
                return@toNative
            }
            val dataString = result.optString("data")
            val nativeSuccess = result.optInt("success")
            val nativeStatusCode = if (result.has("statusCode")) result.optInt("statusCode") else null
            // Kuikly iOS puts NSError.code (for example, timeout = -1001) in
            // statusCode when no HTTP response was received. Expose only real
            // HTTP status codes to the repository.
            val statusCode = nativeStatusCode?.takeIf { it in 100..599 }
            val errorMessage = result.optString("errorMsg")
            val response = ChatHttpResponse(
                success = nativeSuccess == 1,
                statusCode = statusCode,
                headers = parseHeaders(result.optString("headers")),
                body = dataString.takeIf { it.isNotBlank() }?.let(::parseJson),
                errorMessage = errorMessage,
            )
            continuation.resume(response)
        })
    }

    private fun parseJson(value: String): JSONObject? = try { JSONObject(value) } catch (_: Throwable) { null }
    private fun parseHeaders(value: String): Map<String, String> {
        val json = parseJson(value) ?: return emptyMap()
        return buildMap { json.keys().forEach { key -> put(key, json.optString(key)) } }
    }
}
