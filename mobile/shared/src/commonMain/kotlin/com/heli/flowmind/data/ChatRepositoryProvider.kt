package com.heli.flowmind.data

object ChatRepositoryProvider {
    // Phase 9 keeps the UI on mock until a page injects NetworkModule/configuration.
    val repository: ChatRepository = MockChatRepository()
}
