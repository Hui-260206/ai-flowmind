package com.heli.flowmind.data

import com.heli.flowmind.model.Message
import kotlinx.coroutines.delay
import com.heli.flowmind.model.MessageRole

class MockChatRepository : ChatRepository {
    override suspend fun sendChat(history: List<Message>): String {
        delay(800) // 模拟网络往返
        val last = history.lastOrNull {
            it.role == MessageRole.USER
        }?.content.orEmpty()

        // 简单 mock：回声 + 一句固定话术
        return "你说的是：$last"
    }
}