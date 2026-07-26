package com.heli.flowmind.data

object ChatRepositoryProvider {
    // 一处开关：后端没好之前用 mock，做好后改成 RemoteChatRepository()
    val repository: ChatRepository = MockChatRepository()
}