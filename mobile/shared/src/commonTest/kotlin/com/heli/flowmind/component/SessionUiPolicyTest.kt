package com.heli.flowmind.component

import com.heli.flowmind.model.Message
import com.heli.flowmind.model.MessageRole
import com.heli.flowmind.model.MessageStatus
import com.heli.flowmind.state.LoadStatus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue

class SessionUiPolicyTest {
    @Test
    fun `页面状态区分初始化历史空态和消息态`() {
        assertEquals(SessionContentMode.INITIAL_LOADING, mode(LoadStatus.IDLE, LoadStatus.IDLE))
        assertEquals(SessionContentMode.INITIAL_ERROR, mode(LoadStatus.ERROR, LoadStatus.IDLE))
        assertEquals(SessionContentMode.HISTORY_LOADING, mode(LoadStatus.READY, LoadStatus.LOADING))
        assertEquals(SessionContentMode.HISTORY_ERROR, mode(LoadStatus.READY, LoadStatus.ERROR))
        assertEquals(SessionContentMode.EMPTY, mode(LoadStatus.READY, LoadStatus.READY))
        assertEquals(SessionContentMode.MESSAGES, mode(LoadStatus.READY, LoadStatus.READY, true))
    }

    @Test
    fun `会话或尾部消息状态变化会更新自动滚动触发键`() {
        val loading = message("assistant-pending", MessageStatus.SENDING)
        val completed = loading.copy(id = "assistant-persisted", status = MessageStatus.SENT)
        val failed = loading.copy(status = MessageStatus.ERROR)

        val first = messageListTailState("s1", listOf(loading))
        assertNotEquals(first, messageListTailState("s2", listOf(loading)))
        assertNotEquals(first, messageListTailState("s1", listOf(completed)))
        assertNotEquals(first, messageListTailState("s1", listOf(failed)))
    }

    @Test
    fun `只有助手失败消息展示重试入口`() {
        assertTrue(message("a1", MessageStatus.ERROR).isRetryableAssistantFailure())
        assertFalse(message("a2", MessageStatus.SENDING).isRetryableAssistantFailure())
        assertFalse(
            Message("u1", role = MessageRole.USER, content = "失败", status = MessageStatus.ERROR)
                .isRetryableAssistantFailure()
        )
    }

    @Test
    fun `输入长度上限与服务端契约一致`() {
        assertEquals(32_000, SESSION_MESSAGE_MAX_LENGTH)
    }

    private fun mode(
        bootstrap: LoadStatus,
        history: LoadStatus,
        hasMessages: Boolean = false,
    ) = resolveSessionContentMode(bootstrap, history, hasMessages)

    private fun message(id: String, status: MessageStatus) =
        Message(id, role = MessageRole.ASSISTANT, content = "长文本保持完整展示", status = status)
}
