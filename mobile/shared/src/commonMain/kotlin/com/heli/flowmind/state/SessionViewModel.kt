package com.heli.flowmind.state

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.setValue
import com.heli.flowmind.data.ChatException
import com.heli.flowmind.data.ChatRepository
import com.heli.flowmind.data.ChatRepositoryProvider
import com.heli.flowmind.model.Message
import com.heli.flowmind.model.MessageRole
import com.heli.flowmind.model.MessageStatus
import com.tencent.kuikly.lifecycle.ViewModel
import com.tencent.kuikly.lifecycle.viewModelScope
import kotlinx.coroutines.launch
import kotlinx.datetime.Clock

class SessionViewModel(
    private val repository: ChatRepository = ChatRepositoryProvider.repository,
) : ViewModel() {
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
        if(isLoading) return
        isLoading = true

        viewModelScope.launch {
            val replyId = (now + 1).toString()
            // 先插一条 SENDING 占位（可选：让 UI 先显示"正在输入..."）
            messages.add(
                Message(
                    id = replyId,
                    role = MessageRole.ASSISTANT,
                    content = "等待模型回答......",
                    status = MessageStatus.SENDING,
                    timestamp = now,
                )
            )
            val replyIndex = messages.lastIndex

            try {
                val reply = repository.sendChat(messages.dropLast(1)) // 不带占位发过去
                messages[replyIndex] = messages[replyIndex].copy(
                    content = reply,
                    status = MessageStatus.SENT,
                )
            } catch (e: ChatException) {
                messages[replyIndex] = messages[replyIndex].copy(
                    content = "回复失败：${e.message}",
                    status = MessageStatus.ERROR,
                )
            } finally {
                isLoading = false
            }
        }
    }
}
