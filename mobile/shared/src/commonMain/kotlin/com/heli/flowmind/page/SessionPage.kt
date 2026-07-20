package com.heli.flowmind.page

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.heli.flowmind.base.Button
import com.heli.flowmind.base.TextField
import com.heli.flowmind.base.BasePage
import com.tencent.kuikly.compose.foundation.background
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.Column
import com.tencent.kuikly.compose.foundation.layout.WindowInsets
import com.tencent.kuikly.compose.foundation.layout.fillMaxSize
import com.tencent.kuikly.compose.foundation.layout.fillMaxWidth
import com.tencent.kuikly.compose.foundation.layout.height
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.foundation.shape.RoundedCornerShape
import com.tencent.kuikly.compose.material3.Scaffold
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.setContent
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.compose.ui.graphics.Color
import com.tencent.kuikly.compose.ui.platform.LocalActivity
import com.tencent.kuikly.compose.ui.unit.dp
import com.tencent.kuikly.compose.ui.unit.sp
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
//        const val CACHE_KEY = "router_last_input_key2"
//        const val LOGO = "https://vfiles.gtimg.cn/wuji_dashboard/wupload/xy/starter/62394e19.png"
//        const val JUMP_TEXT = "跳转"
//        const val TEXT_KEY = "text"
//        const val TITLE = "Kuikly页面路由"
//        const val AAR_MODE_TIP = "如：router 或者 router&key=value （&后面为页面参数）"
    }
}

@Composable
fun SessionScreen() {
    var inputText by remember { mutableStateOf("") }
    Scaffold(
        // 顶/底安全区由 SessionHeader 与底部输入栏统一处理，避免 Scaffold 再叠一层
        contentWindowInsets = WindowInsets(0.dp),
        topBar = { SessionHeader() },
        bottomBar = {
            SessionInputBar(
                value = inputText,
                onValueChange = { inputText = it },
                onSend = { inputText = "" },
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

@Composable
fun SessionHeader() {
    val statusBarHeight = LocalActivity.current.pageData.statusBarHeight
    Column(
        modifier = Modifier.fillMaxWidth()
            .background(Color(0xFF7B7FE4))
            .padding(top = statusBarHeight.dp),
    ) {
        Box(
            modifier = Modifier.fillMaxWidth().height(48.dp),
            contentAlignment = Alignment.Center,
        ) {
            Text(text = "会话", color = Color.White, fontSize = 18.sp)
        }
    }
}

@Composable
fun SessionInputBar(
    value: String,
    onValueChange: (String) -> Unit,
    onSend: () -> Unit,
) {
    // 外层 Box 让整个输入条在底部栏内水平居中
    Box(
        modifier = Modifier.fillMaxWidth().padding(8.dp),
        contentAlignment = Alignment.Center,
    ) {
        // 居中显示的输入框容器（占 92% 宽，左右留白即为"居中"效果）
        Box(
            modifier = Modifier.fillMaxWidth(0.92f)
                .background(color = Color(0xFFF2F2F2), shape = RoundedCornerShape(24.dp))
                .padding(4.dp),
        ) {
            TextField(
                modifier = Modifier.fillMaxWidth()
                    .padding(start = 12.dp, end = 56.dp, top = 8.dp, bottom = 8.dp),
                value = value,
                placeholder = "输入消息...",
                placeholderColor = Color(0xFF999999),
                onValueChange = onValueChange,
            )
            Button(
                modifier = Modifier.align(Alignment.CenterEnd)
                    .background(color = Color(0xFF7B7FE4), shape = RoundedCornerShape(20.dp))
                    .padding(horizontal = 14.dp, vertical = 8.dp),
                onClick = onSend,
            ) {
                Text(text = "发送", color = Color.White)
            }
        }
    }
}