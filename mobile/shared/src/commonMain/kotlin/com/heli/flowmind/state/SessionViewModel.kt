package com.heli.flowmind.state

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.setValue
import com.heli.flowmind.model.Message
import com.heli.flowmind.model.MessageRole
import com.heli.flowmind.model.MessageStatus
import kotlinx.datetime.Clock

class SessionViewModel {
    var inputText by mutableStateOf("")
        private set

    val messages = mutableStateListOf<Message>()

    var isLoading by mutableStateOf(false)
        private set

    fun onInputChange(text: String) {
        inputText = text
    }

    fun sendMessage() {
        val text = inputText.trim()
        if (text.isEmpty()) return

        val now = Clock.System.now().toEpochMilliseconds()
        val userMsg = Message(
            id = now.toString(),
            role = MessageRole.USER,
            content = text,
            status = MessageStatus.SENT,
            timestamp = now
        )
        messages.add(userMsg)
        inputText = ""
        isLoading = true

        // TODO: 调用 AI API，收到响应后添加 assistant Message
    }
}
