package com.heli.flowmind.page

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import com.heli.flowmind.base.BasePage
import com.heli.flowmind.base.offset
import com.heli.flowmind.component.DeleteSessionDialog
import com.heli.flowmind.component.SessionDrawerContent
import com.heli.flowmind.component.SessionHeader
import com.heli.flowmind.component.SessionInputBar
import com.heli.flowmind.component.SessionMessageList
import com.heli.flowmind.component.SessionContentMode
import com.heli.flowmind.component.resolveSessionContentMode
import com.heli.flowmind.data.ChatApiConfig
import com.heli.flowmind.data.ChatRepository
import com.heli.flowmind.data.ChatRepositoryFactory
import com.heli.flowmind.data.ChatRepositoryProvider
import com.heli.flowmind.data.ChatSession
import com.heli.flowmind.state.LoadStatus
import com.heli.flowmind.state.SessionViewModel
import com.tencent.kuikly.compose.animation.core.animateDpAsState
import com.tencent.kuikly.compose.foundation.gestures.detectTapGestures
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.Column
import com.tencent.kuikly.compose.foundation.layout.Spacer
import com.tencent.kuikly.compose.foundation.layout.WindowInsets
import com.tencent.kuikly.compose.foundation.layout.fillMaxSize
import com.tencent.kuikly.compose.foundation.layout.height
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.material3.CircularProgressIndicator
import com.tencent.kuikly.compose.material3.DrawerValue
import com.tencent.kuikly.compose.material3.ModalDrawerSheet
import com.tencent.kuikly.compose.material3.ModalNavigationDrawer
import com.tencent.kuikly.compose.material3.Scaffold
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.material3.rememberDrawerState
import com.tencent.kuikly.compose.setContent
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.compose.ui.input.pointer.pointerInput
import com.tencent.kuikly.compose.ui.layout.onSizeChanged
import com.tencent.kuikly.compose.ui.platform.LocalDensity
import com.tencent.kuikly.compose.ui.platform.LocalFocusManager
import com.tencent.kuikly.compose.ui.platform.LocalSoftwareKeyboardController
import com.tencent.kuikly.compose.ui.unit.dp
import com.tencent.kuikly.core.annotations.Page
import com.tencent.kuikly.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.launch

@Page("flowmind_session_page", supportInLocal = true)
class SessionPage : BasePage() {
    override fun willInit() {
        super.willInit()
        setContent {
            val repository = remember { createRepository() }
            val selectedSessionStore = remember { ChatRepositoryFactory.selectedSessionStore(this) }
            val viewModel = viewModel { SessionViewModel(repository, selectedSessionStore) }
            SessionScreen(viewModel = viewModel)
        }
    }

    private fun createRepository(): ChatRepository {
        val params = pageData.params
        val baseUrl = params.optString("chatApiBaseUrl").trim()
        if (baseUrl.isEmpty()) return ChatRepositoryProvider.repository
        return ChatRepositoryFactory.remote(
            pager = this,
            config = ChatApiConfig(
                baseUrl = baseUrl,
                timeoutSeconds = params.optInt("chatApiTimeoutSeconds", 30),
                production = params.optBoolean("chatApiProduction"),
            ),
        )
    }
}

@Composable
fun SessionScreen(viewModel: SessionViewModel) {
    LaunchedEffect(viewModel) { viewModel.initialize() }

    val drawerState = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    var pendingDelete by remember { mutableStateOf<ChatSession?>(null) }
    var keyboardHeight by remember { mutableStateOf(0.dp) }
    val animatedKeyboardHeight by animateDpAsState(keyboardHeight)
    var inputBarHeight by remember { mutableStateOf(0.dp) }
    val density = LocalDensity.current
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    val currentFocusManager by rememberUpdatedState(focusManager)
    val currentKeyboardController by rememberUpdatedState(keyboardController)

    ModalNavigationDrawer(
        drawerState = drawerState,
        gesturesEnabled = viewModel.canManageSessions,
        drawerContent = {
            ModalDrawerSheet {
                SessionDrawerContent(
                    sessions = viewModel.sessions,
                    selectedSessionId = viewModel.currentSessionId,
                    actionsEnabled = viewModel.canManageSessions,
                    onCreateSession = {
                        viewModel.createSession()
                        scope.launch { drawerState.close() }
                    },
                    onSelectSession = { sessionId ->
                        viewModel.selectSession(sessionId)
                        scope.launch { drawerState.close() }
                    },
                    onRequestDelete = { pendingDelete = it },
                )
            }
        },
    ) {
        Scaffold(
            contentWindowInsets = WindowInsets(0.dp),
            topBar = {
                SessionHeader(
                    title = viewModel.currentTitle,
                    actionsEnabled = viewModel.canManageSessions,
                    onOpenSessions = { scope.launch { drawerState.open() } },
                    onCreateSession = viewModel::createSession,
                )
            },
        ) { innerPadding ->
            Box(
                modifier = Modifier.fillMaxSize().padding(innerPadding).pointerInput(Unit) {
                    detectTapGestures(onTap = {
                        currentFocusManager.clearFocus()
                        currentKeyboardController?.hide()
                    })
                },
            ) {
                SessionContent(
                    viewModel = viewModel,
                    bottomInset = inputBarHeight + animatedKeyboardHeight,
                )

                if (viewModel.bootstrapStatus == LoadStatus.READY) {
                    SessionInputBar(
                        value = viewModel.inputText,
                        onValueChange = viewModel::onInputChange,
                        onSend = viewModel::sendMessage,
                        onKeyboardHeightChange = { keyboardHeight = it.height.dp },
                        inputEnabled = viewModel.canEditInput,
                        sendEnabled = viewModel.canSend,
                        modifier = Modifier.align(Alignment.BottomCenter)
                            .offset(x = 0f, y = -animatedKeyboardHeight.value)
                            .onSizeChanged { inputBarHeight = with(density) { it.height.toDp() } },
                    )
                }
            }
        }
    }

    pendingDelete?.let { session ->
        DeleteSessionDialog(
            session = session,
            onConfirm = {
                pendingDelete = null
                viewModel.deleteSession(session.id)
            },
            onDismiss = { pendingDelete = null },
        )
    }
}

@Composable
private fun SessionContent(viewModel: SessionViewModel, bottomInset: com.tencent.kuikly.compose.ui.unit.Dp) {
    when (resolveSessionContentMode(viewModel.bootstrapStatus, viewModel.historyStatus, viewModel.messages.isNotEmpty())) {
        SessionContentMode.INITIAL_LOADING ->
            CenterStatus("正在加载会话…")

        SessionContentMode.INITIAL_ERROR ->
            CenterStatus(viewModel.error?.message ?: "会话加载失败", "重试", viewModel::retryInitialization)

        SessionContentMode.HISTORY_LOADING ->
            CenterStatus("正在加载历史消息…")

        SessionContentMode.HISTORY_ERROR ->
            CenterStatus(viewModel.error?.message ?: "历史加载失败", "重试", viewModel::retryHistory)

        SessionContentMode.EMPTY, SessionContentMode.MESSAGES -> SessionMessageList(
            sessionId = viewModel.currentSessionId,
            messages = viewModel.messages,
            modifier = Modifier.fillMaxSize(),
            bottomInset = bottomInset,
            onRetry = viewModel::retryPendingTurn,
        )
    }
}

@Composable
private fun CenterStatus(message: String, action: String? = null, onAction: () -> Unit = {}) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            if (action == null) CircularProgressIndicator()
            Spacer(Modifier.height(10.dp))
            Text(text = message)
            if (action != null) {
                Spacer(Modifier.height(10.dp))
                com.tencent.kuikly.compose.material3.Button(onClick = onAction) { Text(action) }
            }
        }
    }
}
