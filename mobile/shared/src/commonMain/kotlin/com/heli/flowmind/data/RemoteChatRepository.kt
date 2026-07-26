package com.heli.flowmind.data

import com.heli.flowmind.model.Message
// import io.ktor.client.*        // 后端就绪后启用 ktor
// import io.ktor.client.request.*
// import io.ktor.client.statement.*

/**
 * 真实网络实现。后端 FastAPI 起来后，在此处接入 HTTP。
 *
 * 建议：
 * 1. 在 shared/build.gradle.kts 的 commonMain 依赖里加
 *    ktor-client-core + 各平台 engine（android/ios/js 各一个）。
 * 2. baseUrl 走 BuildConfig/常量，开发期指向本机 uvicorn。
 * 3. 错误统一包装成 ChatException 抛给 ViewModel。
 */
class RemoteChatRepository(
    private val baseUrl: String = "http://127.0.0.1:8090",
) : ChatRepository {
    override suspend fun sendChat(history: List<Message>): String {
        val dto = ChatRequest(messages = history.map { ChatMessageDto.fromMessage(it) })
        // TODO: 用 ktor 发 POST "$baseUrl/chat"，body = dto 序列化，
        //       解析 ChatResponse.reply 返回。
        TODO("后端 FastAPI 实现后接入 ktor")
    }
}