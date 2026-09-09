package com.heli.flowmind.data

import com.tencent.kuikly.core.module.NetworkModule
import com.tencent.kuikly.core.module.SharedPreferencesModule
import com.tencent.kuikly.core.pager.Pager

/** Creates a real repository only from the active page's Kuikly modules. */
object ChatRepositoryFactory {
    fun remote(pager: Pager, config: ChatApiConfig): ChatRepository = RemoteChatRepository(
        transport = NetworkModuleChatHttpTransport(
            pager.acquireModule(NetworkModule.MODULE_NAME),
        ),
        config = config,
        clientIdentity = SharedPreferencesClientIdentity(
            pager.acquireModule(SharedPreferencesModule.MODULE_NAME),
        ),
    )

    fun selectedSessionStore(pager: Pager) = com.heli.flowmind.state.SharedPreferencesSelectedSessionStore(
        pager.acquireModule(SharedPreferencesModule.MODULE_NAME),
    )
}
