package com.heli.flowmind.state

import com.heli.flowmind.data.KeyValueStore
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SessionStateTest {
    @Test
    fun `选择存储会规范化保存读取和清除`() {
        val values = mutableMapOf<String, String>()
        val store = StoredSelectedSessionStore(object : KeyValueStore {
            override fun getString(key: String): String = values[key].orEmpty()
            override fun putString(key: String, value: String) { values[key] = value }
        })

        assertNull(store.selectedSessionId())
        store.saveSelectedSessionId(" session-1 ")
        assertEquals("session-1", store.selectedSessionId())
        store.clearSelectedSessionId()
        assertNull(store.selectedSessionId())
    }

    @Test
    fun `默认客户端消息ID生成器生成UUID形式标识`() {
        val id = RandomClientMessageIdGenerator.newClientMessageId()

        assertTrue(
            Regex("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")
                .matches(id)
        )
    }
}
