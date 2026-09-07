package com.heli.flowmind.data

import com.tencent.kuikly.core.module.SharedPreferencesModule
import kotlin.random.Random

data class ChatApiConfig(
    val baseUrl: String,
    val timeoutSeconds: Int = 30,
    val production: Boolean = false,
) {
    init {
        require(timeoutSeconds > 0) { "timeoutSeconds must be positive" }
        require(baseUrl.startsWith("http://") || baseUrl.startsWith("https://")) { "baseUrl must use http or https" }
        require(!production || baseUrl.startsWith("https://")) { "production baseUrl must use HTTPS" }
    }

    fun url(path: String): String = "${baseUrl.trimEnd('/')}/${path.trimStart('/')}"
}

object ChatApiEndpoints {
    fun androidEmulator(port: Int = 8080) = development("http://10.0.2.2:$port")
    fun iosSimulator(port: Int = 8080) = development("http://127.0.0.1:$port")
    fun openHarmonyDevice(baseUrl: String) = development(baseUrl)
    fun physicalDevice(baseUrl: String) = development(baseUrl)
    fun production(baseUrl: String) = ChatApiConfig(baseUrl, production = true)
    fun development(baseUrl: String) = ChatApiConfig(baseUrl)
}

interface ClientIdentity { fun clientId(): String }

interface KeyValueStore {
    fun getString(key: String): String
    fun putString(key: String, value: String)
}

class SharedPreferencesKeyValueStore(
    private val preferences: SharedPreferencesModule,
) : KeyValueStore {
    override fun getString(key: String): String = preferences.getString(key)
    override fun putString(key: String, value: String) = preferences.setString(key, value)
}

class SharedPreferencesClientIdentity(
    preferences: SharedPreferencesModule,
    newId: () -> String = ::randomIdentifier,
) : StoredClientIdentity(SharedPreferencesKeyValueStore(preferences), newId)

open class StoredClientIdentity(
    private val store: KeyValueStore,
    private val newId: () -> String = ::randomIdentifier,
) : ClientIdentity {
    override fun clientId(): String {
        val existing = store.getString(CLIENT_ID_KEY).trim().lowercase()
        if (isUuid(existing)) return existing

        // The Go API validates X-Client-ID as a UUID. Regenerate old or malformed
        // stored values instead of repeatedly sending a request the server rejects.
        val generated = newId().lowercase()
        require(isUuid(generated)) { "generated client ID must be a UUID" }
        store.putString(CLIENT_ID_KEY, generated)
        return generated
    }

    private companion object { const val CLIENT_ID_KEY = "flowmind.anonymous_client_id" }
}

interface RequestIdGenerator { fun newRequestId(): String }
object RandomRequestIdGenerator : RequestIdGenerator { override fun newRequestId(): String = randomIdentifier() }

private fun randomIdentifier(): String = buildString(36) {
    append(randomHex(8))
    append('-')
    append(randomHex(4))
    append("-4") // UUID version 4
    append(randomHex(3))
    append('-')
    append("89ab"[Random.nextInt(4)]) // UUID RFC 4122 variant
    append(randomHex(3))
    append('-')
    append(randomHex(12))
}

private fun randomHex(length: Int): String = buildString(length) {
    repeat(length) { append("0123456789abcdef"[Random.nextInt(16)]) }
}

private fun isUuid(value: String): Boolean = value.length == 36 && value.indices.all { index ->
    val character = value[index]
    if (index == 8 || index == 13 || index == 18 || index == 23) character == '-'
    else character in "0123456789abcdef"
}
