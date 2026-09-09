package com.heli.flowmind.state

import com.heli.flowmind.data.ChatException
import com.heli.flowmind.data.ChatMessage
import com.heli.flowmind.data.ChatRepository
import com.heli.flowmind.data.ChatSession
import com.heli.flowmind.data.SendMessageResult
import com.heli.flowmind.model.MessageStatus
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class)
class SessionViewModelTest {
    @Test
    fun `空列表初始化只创建一个空会话`() = runTest {
        val repository = FakeChatRepository()
        val store = MemorySelectedSessionStore()
        val viewModel = viewModel(repository, store)

        viewModel.initialize()
        advanceUntilIdle()

        assertEquals(1, repository.createCalls)
        assertEquals("session-1", viewModel.currentSessionId)
        assertEquals("session-1", store.value)
        assertEquals(LoadStatus.READY, viewModel.bootstrapStatus)
        assertEquals(LoadStatus.READY, viewModel.historyStatus)
        assertTrue(viewModel.messages.isEmpty())
    }

    @Test
    fun `重复初始化只启动一次列表和创建流程`() = runTest {
        val repository = FakeChatRepository()
        val viewModel = viewModel(repository)

        viewModel.initialize()
        viewModel.initialize()
        advanceUntilIdle()

        assertEquals(1, repository.listCalls)
        assertEquals(1, repository.createCalls)
    }

    @Test
    fun `恢复有效选择并在选择过期时回退到首个会话`() = runTest {
        val first = session("first")
        val second = session("second")
        val repository = FakeChatRepository(initialSessions = listOf(first, second))
        val validStore = MemorySelectedSessionStore("second")
        val valid = viewModel(repository, validStore)

        valid.initialize()
        advanceUntilIdle()

        assertEquals("second", valid.currentSessionId)
        assertEquals(0, repository.createCalls)

        val staleStore = MemorySelectedSessionStore("missing")
        val stale = viewModel(repository, staleStore)
        stale.initialize()
        advanceUntilIdle()

        assertEquals("first", stale.currentSessionId)
        assertEquals("first", staleStore.value)
    }

    @Test
    fun `列表失败不会创建替代会话`() = runTest {
        val repository = FakeChatRepository().apply { listFailure = failure("TRANSPORT_ERROR") }
        val viewModel = viewModel(repository)

        viewModel.initialize()
        advanceUntilIdle()

        assertEquals(LoadStatus.ERROR, viewModel.bootstrapStatus)
        assertEquals(0, repository.createCalls)
        assertNull(viewModel.currentSessionId)
    }

    @Test
    fun `历史失败保留选择并允许重试`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"))).apply {
            historyFailures["s1"] = failure("TRANSPORT_ERROR")
        }
        val viewModel = viewModel(repository)

        viewModel.initialize()
        advanceUntilIdle()

        assertEquals("s1", viewModel.currentSessionId)
        assertEquals(LoadStatus.ERROR, viewModel.historyStatus)
        assertEquals(1, viewModel.sessions.size)

        repository.historyFailures.clear()
        repository.messages["s1"] = mutableListOf(message("u1", "user", "旧消息", "m-old"))
        viewModel.retryHistory()
        advanceUntilIdle()

        assertEquals(LoadStatus.READY, viewModel.historyStatus)
        assertEquals("旧消息", viewModel.messages.first().content)
    }

    @Test
    fun `连续两轮使用同一会话和不同客户端消息ID`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1")))
        val ids = ArrayDeque(listOf("turn-1", "turn-2"))
        val viewModel = viewModel(repository, generator = ClientMessageIdGenerator { ids.removeFirst() })
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.onInputChange("第一轮")
        viewModel.sendMessage()
        advanceUntilIdle()
        viewModel.onInputChange("第二轮")
        viewModel.sendMessage()
        advanceUntilIdle()

        assertEquals(listOf("s1", "s1"), repository.sendCalls.map { it.sessionId })
        assertEquals(listOf("turn-1", "turn-2"), repository.sendCalls.map { it.clientMessageId })
        assertEquals(4, viewModel.messages.size)
        assertEquals(0, repository.createCalls)
    }

    @Test
    fun `空白输入不会发送也不会添加乐观消息`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1")))
        val viewModel = viewModel(repository)
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.onInputChange("   \n  ")
        viewModel.sendMessage()
        advanceUntilIdle()

        assertTrue(repository.sendCalls.isEmpty())
        assertTrue(viewModel.messages.isEmpty())
        assertNull(viewModel.pendingTurn)
    }

    @Test
    fun `活动发送期间重复点击和会话变更都被阻止`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"), session("s2")))
        val gate = CompletableDeferred<Unit>()
        repository.sendGate = gate
        val viewModel = viewModel(repository)
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.onInputChange("只发送一次")
        viewModel.sendMessage()
        viewModel.sendMessage()
        viewModel.selectSession("s2")
        viewModel.createSession()
        viewModel.deleteSession("s2")
        runCurrent()

        assertEquals(1, repository.sendCalls.size)
        assertEquals("s1", viewModel.currentSessionId)
        assertEquals(2, viewModel.messages.size)
        assertEquals(0, repository.deleteCalls.size)

        gate.complete(Unit)
        advanceUntilIdle()
    }

    @Test
    fun `旧会话的延迟历史不会覆盖新会话`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"), session("s2")))
        val s1History = CompletableDeferred<List<ChatMessage>>()
        repository.historyGates["s1"] = s1History
        repository.messages["s2"] = mutableListOf(message("u2", "user", "会话二", "m2"), message("a2", "assistant", "回复二"))
        val viewModel = viewModel(repository)

        viewModel.initialize()
        runCurrent()
        viewModel.selectSession("s2")
        advanceUntilIdle()
        assertEquals("会话二", viewModel.messages.first().content)

        s1History.complete(listOf(message("u1", "user", "过期消息", "m1")))
        advanceUntilIdle()

        assertEquals("s2", viewModel.currentSessionId)
        assertEquals("会话二", viewModel.messages.first().content)
    }

    @Test
    fun `显式新建会话后可切回已有会话`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"))).apply {
            messages["s1"] = mutableListOf(message("u1", "user", "原会话", "old-turn"))
        }
        val store = MemorySelectedSessionStore()
        val viewModel = viewModel(repository, store)
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.createSession()
        advanceUntilIdle()

        assertEquals("session-2", viewModel.currentSessionId)
        assertEquals("session-2", store.value)
        assertTrue(viewModel.messages.isEmpty())

        viewModel.selectSession("s1")
        advanceUntilIdle()

        assertEquals("s1", viewModel.currentSessionId)
        assertEquals("s1", store.value)
        assertEquals("原会话", viewModel.messages.first().content)
    }

    @Test
    fun `重复新建点击只创建一个会话`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1")))
        val viewModel = viewModel(repository)
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.createSession()
        viewModel.createSession()
        advanceUntilIdle()

        assertEquals(1, repository.createCalls)
        assertEquals(2, viewModel.sessions.size)
    }

    @Test
    fun `会话变更开始后阻止发送旧会话消息`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1")))
        val viewModel = viewModel(repository)
        viewModel.initialize()
        advanceUntilIdle()
        viewModel.onInputChange("不应发送")

        viewModel.createSession()
        viewModel.sendMessage()
        advanceUntilIdle()

        assertTrue(repository.sendCalls.isEmpty())
        assertEquals("session-2", viewModel.currentSessionId)
    }

    @Test
    fun `失败重试复用客户端消息ID且不重复用户气泡`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"))).apply {
            sendFailures += failure("AI_TIMEOUT")
        }
        val viewModel = viewModel(repository, generator = ClientMessageIdGenerator { "stable-turn" })
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.onInputChange("请重试")
        viewModel.sendMessage()
        advanceUntilIdle()

        assertEquals(SendStatus.FAILED, viewModel.sendStatus)
        assertEquals(2, viewModel.messages.size)
        assertFalse(viewModel.canSend)
        assertFalse(viewModel.canEditInput)

        viewModel.retryPendingTurn()
        advanceUntilIdle()

        assertEquals(listOf("stable-turn", "stable-turn"), repository.sendCalls.map { it.clientMessageId })
        assertEquals(2, viewModel.messages.size)
        assertTrue(viewModel.messages.all { it.status == MessageStatus.SENT })
        assertNull(viewModel.pendingTurn)
    }

    @Test
    fun `发送报错但服务端已完成时通过历史对账恢复成功`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"))).apply {
            afterFailedSend = { call ->
                messages[call.sessionId] = mutableListOf(
                    message("u1", "user", call.content, call.clientMessageId),
                    message("a1", "assistant", "实际上已完成"),
                )
            }
            sendFailures += failure("TRANSPORT_ERROR")
        }
        val viewModel = viewModel(repository, generator = ClientMessageIdGenerator { "turn-1" })
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.onInputChange("测试对账")
        viewModel.sendMessage()
        advanceUntilIdle()

        assertEquals(SendStatus.IDLE, viewModel.sendStatus)
        assertNull(viewModel.pendingTurn)
        assertEquals(listOf("测试对账", "实际上已完成"), viewModel.messages.map { it.content })
    }

    @Test
    fun `历史末尾用户消息恢复为可重试轮次`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"))).apply {
            messages["s1"] = mutableListOf(message("u1", "user", "未完成问题", "recover-id"))
        }
        val viewModel = viewModel(repository)

        viewModel.initialize()
        advanceUntilIdle()

        assertEquals(SendStatus.FAILED, viewModel.sendStatus)
        assertEquals("recover-id", viewModel.pendingTurn?.clientMessageId)
        assertEquals(2, viewModel.messages.size)
        assertEquals(MessageStatus.ERROR, viewModel.messages.last().status)
    }

    @Test
    fun `发送和对账都失败时保留原轮次并阻止新轮次`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"))).apply {
            sendFailures += failure("AI_TIMEOUT")
        }
        val viewModel = viewModel(repository, generator = ClientMessageIdGenerator { "stable-turn" })
        viewModel.initialize()
        advanceUntilIdle()
        repository.historyFailures["s1"] = failure("TRANSPORT_ERROR")

        viewModel.onInputChange("第一次")
        viewModel.sendMessage()
        advanceUntilIdle()

        assertEquals(SendStatus.FAILED, viewModel.sendStatus)
        assertEquals("stable-turn", viewModel.pendingTurn?.clientMessageId)
        assertEquals(2, viewModel.messages.size)

        viewModel.onInputChange("不应发送的新轮次")
        viewModel.sendMessage()
        advanceUntilIdle()

        assertEquals(1, repository.sendCalls.size)
        assertEquals(2, viewModel.messages.size)
        assertFalse(viewModel.canSend)
        assertFalse(viewModel.canEditInput)
    }

    @Test
    fun `失败轮次切换会话后仍可恢复并使用原ID重试`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"), session("s2"))).apply {
            sendFailures += failure("TRANSPORT_ERROR")
        }
        val viewModel = viewModel(repository, generator = ClientMessageIdGenerator { "stable-turn" })
        viewModel.initialize()
        advanceUntilIdle()
        repository.historyFailures["s1"] = failure("TRANSPORT_ERROR")

        viewModel.onInputChange("需要保留")
        viewModel.sendMessage()
        advanceUntilIdle()
        viewModel.selectSession("s2")
        advanceUntilIdle()

        assertEquals("s2", viewModel.currentSessionId)
        assertNull(viewModel.pendingTurn)
        assertTrue(viewModel.canEditInput)

        repository.historyFailures.clear()
        viewModel.selectSession("s1")
        advanceUntilIdle()

        assertEquals("stable-turn", viewModel.pendingTurn?.clientMessageId)
        assertEquals(listOf("需要保留", "请求失败"), viewModel.messages.map { it.content })
        viewModel.retryPendingTurn()
        advanceUntilIdle()

        assertEquals(listOf("stable-turn", "stable-turn"), repository.sendCalls.map { it.clientMessageId })
        assertNull(viewModel.pendingTurn)
    }

    @Test
    fun `成功后的元数据刷新失败不影响消息和继续发送`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1")))
        val viewModel = viewModel(repository)
        viewModel.initialize()
        advanceUntilIdle()
        repository.listFailure = failure("TRANSPORT_ERROR")

        viewModel.onInputChange("已成功")
        viewModel.sendMessage()
        advanceUntilIdle()

        assertEquals(SendStatus.IDLE, viewModel.sendStatus)
        assertNull(viewModel.pendingTurn)
        assertEquals(listOf("已成功", "回复：已成功"), viewModel.messages.map { it.content })
        assertTrue(viewModel.messages.all { it.status == MessageStatus.SENT })

        viewModel.onInputChange("仍可继续")
        assertTrue(viewModel.canSend)
    }

    @Test
    fun `删除当前和最后会话时选择剩余项或创建空会话`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"), session("s2")))
        val viewModel = viewModel(repository)
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.deleteSession("s1")
        advanceUntilIdle()
        assertEquals("s2", viewModel.currentSessionId)

        viewModel.deleteSession("s2")
        advanceUntilIdle()
        assertNotNull(viewModel.currentSessionId)
        assertEquals(1, viewModel.sessions.size)
        assertEquals(1, repository.createCalls)
    }

    @Test
    fun `删除非当前会话保持当前选择和历史`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"), session("s2"))).apply {
            messages["s1"] = mutableListOf(
                message("u1", "user", "当前历史", "turn-1"),
                message("a1", "assistant", "当前回复"),
            )
        }
        val store = MemorySelectedSessionStore("s1")
        val viewModel = viewModel(repository, store)
        viewModel.initialize()
        advanceUntilIdle()

        viewModel.deleteSession("s2")
        advanceUntilIdle()

        assertEquals(listOf("s2"), repository.deleteCalls)
        assertEquals("s1", viewModel.currentSessionId)
        assertEquals("s1", store.value)
        assertEquals(listOf("当前历史", "当前回复"), viewModel.messages.map { it.content })
    }

    @Test
    fun `同一存储使新ViewModel恢复上次会话和历史`() = runTest {
        val repository = FakeChatRepository(initialSessions = listOf(session("s1"), session("s2"))).apply {
            messages["s2"] = mutableListOf(message("u2", "user", "持久历史", "m2"), message("a2", "assistant", "已恢复"))
        }
        val store = MemorySelectedSessionStore("s2")

        val recreated = viewModel(repository, store)
        recreated.initialize()
        advanceUntilIdle()

        assertEquals("s2", recreated.currentSessionId)
        assertEquals(listOf("持久历史", "已恢复"), recreated.messages.map { it.content })
    }

    private fun TestScope.viewModel(
        repository: ChatRepository,
        store: SelectedSessionStore = MemorySelectedSessionStore(),
        generator: ClientMessageIdGenerator = ClientMessageIdGenerator { "generated-id" },
    ) = SessionViewModel(repository, store, generator, this)

    private fun session(id: String) = ChatSession(id, "会话-$id", "default", NOW, NOW)

    private fun message(id: String, role: String, content: String, clientMessageId: String? = null) =
        ChatMessage(id, role, content, "completed", clientMessageId = clientMessageId, createdAt = NOW)

    private fun failure(code: String) = ChatException(code, "请求失败")

    private data class SendCall(val sessionId: String, val content: String, val clientMessageId: String)

    private class MemorySelectedSessionStore(var value: String? = null) : SelectedSessionStore {
        override fun selectedSessionId(): String? = value
        override fun saveSelectedSessionId(sessionId: String) { value = sessionId }
        override fun clearSelectedSessionId() { value = null }
    }

    private class FakeChatRepository(initialSessions: List<ChatSession> = emptyList()) : ChatRepository {
        val sessions = initialSessions.toMutableList()
        val messages = mutableMapOf<String, MutableList<ChatMessage>>().apply {
            initialSessions.forEach { put(it.id, mutableListOf()) }
        }
        val sendCalls = mutableListOf<SendCall>()
        val deleteCalls = mutableListOf<String>()
        val sendFailures = ArrayDeque<ChatException>()
        val historyFailures = mutableMapOf<String, ChatException>()
        val historyGates = mutableMapOf<String, CompletableDeferred<List<ChatMessage>>>()
        var afterFailedSend: ((SendCall) -> Unit)? = null
        var sendGate: CompletableDeferred<Unit>? = null
        var listFailure: ChatException? = null
        var listCalls = 0
        var createCalls = 0
        private var nextMessage = 1

        override suspend fun createSession(): ChatSession {
            createCalls++
            val id = "session-${sessions.size + 1}"
            val created = ChatSession(id, "会话-$id", "default", NOW, NOW)
            sessions.add(0, created)
            messages[created.id] = mutableListOf()
            return created
        }

        override suspend fun listSessions(): List<ChatSession> {
            listCalls++
            listFailure?.let { throw it }
            return sessions.toList()
        }

        override suspend fun getMessages(sessionId: String): List<ChatMessage> {
            historyFailures[sessionId]?.let { throw it }
            historyGates[sessionId]?.let { return it.await() }
            return messages[sessionId]?.toList() ?: throw ChatException("SESSION_NOT_FOUND", "请求失败")
        }

        override suspend fun sendMessage(sessionId: String, content: String, clientMessageId: String): SendMessageResult {
            val call = SendCall(sessionId, content, clientMessageId)
            sendCalls += call
            sendGate?.await()
            if (sendFailures.isNotEmpty()) {
                val error = sendFailures.removeFirst()
                afterFailedSend?.invoke(call)
                throw error
            }
            val history = messages[sessionId] ?: throw ChatException("SESSION_NOT_FOUND", "请求失败")
            val existingUser = history.firstOrNull { it.clientMessageId == clientMessageId }
            val user = existingUser ?: ChatMessage(
                "user-${nextMessage++}", "user", content, "completed",
                clientMessageId = clientMessageId, createdAt = NOW,
            ).also(history::add)
            val existingAssistant = history.getOrNull(history.indexOf(user) + 1)?.takeIf { it.role == "assistant" }
            val assistant = existingAssistant ?: ChatMessage(
                "assistant-${nextMessage++}", "assistant", "回复：$content", "completed", createdAt = NOW,
            ).also(history::add)
            val index = sessions.indexOfFirst { it.id == sessionId }
            if (index >= 0) sessions[index] = sessions[index].copy(title = content.take(32), updatedAt = "2026-09-09T01:00:00Z")
            sessions.sortByDescending { it.updatedAt }
            return SendMessageResult("request-$clientMessageId", user, assistant)
        }

        override suspend fun deleteSession(sessionId: String) {
            deleteCalls += sessionId
            if (!sessions.removeAll { it.id == sessionId }) throw ChatException("SESSION_NOT_FOUND", "请求失败")
            messages.remove(sessionId)
        }
    }

    private companion object {
        const val NOW = "2026-09-09T00:00:00Z"
    }
}
