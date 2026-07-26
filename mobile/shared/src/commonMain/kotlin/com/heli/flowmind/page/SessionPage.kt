package com.heli.flowmind.page

import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import com.heli.flowmind.base.BasePage
import com.heli.flowmind.component.SessionHeader
import com.heli.flowmind.component.SessionInputBar
import com.heli.flowmind.state.SessionViewModel
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

@Page("flowmind_session_page", supportInLocal = true)
class SessionPage : BasePage() {

    override fun willInit() {
        super.willInit()
        setContent {
            SessionScreen()
        }
    }

    companion object {
//        const val PLACEHOLDER = "输入pageName（不区分大小写）"
    }
}

@Composable
fun SessionScreen() {
    var viewModel = remember { SessionViewModel() }
    Scaffold(
        // 顶/底安全区由 SessionHeader 与底部输入栏统一处理，避免 Scaffold 再叠一层
        contentWindowInsets = WindowInsets(0.dp),
        topBar = { SessionHeader() },
        bottomBar = {
            SessionInputBar(
                value = viewModel.inputText,
                onValueChange = { viewModel.onInputChange(it) },
                onSend = { viewModel.sendMessage() },
            )
        },
    ) { innerPadding ->
        Box(
            modifier = Modifier.fillMaxSize().padding(innerPadding),
            contentAlignment = Alignment.Center,
        ) {
            Text(text = "SessionScreen")
        }
    }
}
