package com.heli.flowmind.page

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import com.heli.flowmind.base.BasePage
import com.heli.flowmind.base.offset
import com.heli.flowmind.component.SessionHeader
import com.heli.flowmind.component.SessionInputBar
import com.heli.flowmind.component.SessionMessageList
import com.heli.flowmind.state.SessionViewModel
import com.tencent.kuikly.compose.animation.core.animateDpAsState
import com.tencent.kuikly.compose.foundation.gestures.detectTapGestures
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.WindowInsets
import com.tencent.kuikly.compose.foundation.layout.fillMaxSize
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.material3.Scaffold
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

@Page("flowmind_session_page", supportInLocal = true)
class SessionPage : BasePage() {

    override fun willInit() {
        super.willInit()
        setContent {
            val viewModel = viewModel { SessionViewModel() }
            SessionScreen(viewModel = viewModel)
        }
    }
}

@Composable
fun SessionScreen(viewModel: SessionViewModel) {
    var keyboardHeight by remember { mutableStateOf(0.dp) }
    val animatedKeyboardHeight by animateDpAsState(keyboardHeight)

    // 输入栏实测高度:消息列表底部要留出这块,最后一条消息才不会被浮层输入栏永久遮住
    var inputBarHeight by remember { mutableStateOf(0.dp) }
    val density = LocalDensity.current

    // 点击主区域收键盘:清除焦点(TextField 失焦会自动收键盘),hide() 兜底
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    val currentFocusManager by rememberUpdatedState(focusManager)
    val currentKeyboardController by rememberUpdatedState(keyboardController)

    Scaffold(
        // 顶/底安全区由 SessionHeader 与输入栏自行处理,Scaffold 不再叠一层
        contentWindowInsets = WindowInsets(0.dp),
        topBar = { SessionHeader() },
        // 不再使用 bottomBar:它是"分区"布局,无法让输入栏浮在消息列表之上
    ) { innerPadding ->
        // Box 浮层:先声明的子节点在下层,后声明的在上层
        // → 消息列表(底层)被输入栏(上层)遮住
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .pointerInput(Unit) {
                    // 仅响应轻点收键盘;不消费拖动,不影响 LazyColumn 滚动
                    detectTapGestures(onTap = {
                        currentFocusManager.clearFocus()
                        currentKeyboardController?.hide()
                    })
                },
        ) {
            // 底层:消息列表,底部留出"输入栏 + 键盘"高度,让最后一条可滚到可见区
            SessionMessageList(
                messages = viewModel.messages,   // mutableStateListOf 本身就是可观察的 List
                modifier = Modifier.fillMaxSize(),
                bottomInset = inputBarHeight + animatedKeyboardHeight,
            )

            // 上层:输入栏,底部对齐,跟随键盘上移;onSizeChanged 测出高度回传给消息列表
            SessionInputBar(
                value = viewModel.inputText,
                onValueChange = { viewModel.onInputChange(it) },
                onSend = { viewModel.sendMessage() },
                onKeyboardHeightChange = { keyboardHeight = it.height.dp },
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .offset(x = 0f, y = -animatedKeyboardHeight.value)
                    .onSizeChanged {
                        inputBarHeight = with(density) { it.height.toDp() }
                    },
            )
        }
    }
}
