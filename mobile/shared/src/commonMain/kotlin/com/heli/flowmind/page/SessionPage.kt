package com.heli.flowmind.page

import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.runtime.sourceInformation
import com.heli.flowmind.base.BasePage
import com.heli.flowmind.base.offset
import com.heli.flowmind.component.SessionHeader
import com.heli.flowmind.component.SessionInputBar
import com.heli.flowmind.component.SessionMessageList
import com.heli.flowmind.state.SessionViewModel
import com.tencent.kuikly.compose.animation.core.animateDpAsState
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.WindowInsets
import com.tencent.kuikly.compose.foundation.layout.fillMaxSize
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.material3.Scaffold
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.setContent
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
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

    companion object {
//        const val PLACEHOLDER = "输入pageName（不区分大小写）"
    }
}

@Composable
fun SessionScreen(viewModel: SessionViewModel) {
    var keyboardHeight by remember { mutableStateOf(0.dp) }
    val animatedKeyboardHeight by animateDpAsState(keyboardHeight)

    Scaffold(
        // 顶/底安全区由 SessionHeader 与底部输入栏统一处理，避免 Scaffold 再叠一层
        contentWindowInsets = WindowInsets(0.dp),
        topBar = { SessionHeader() },
        bottomBar = {
            SessionInputBar(
                value = viewModel.inputText,
                modifier = Modifier.offset(x = 0f, y = -animatedKeyboardHeight.value),
                onValueChange = { viewModel.onInputChange(it) },
                onSend = { viewModel.sendMessage() },
                onKeyboardHeightChange = { keyboardHeight = it.height.dp },
            )
        },
    ) { innerPadding ->
        SessionMessageList(
            messages = viewModel.messages,   // mutableStateListOf 本身就是可观察的 List
            modifier = Modifier.padding(innerPadding),
        )
    }
}
