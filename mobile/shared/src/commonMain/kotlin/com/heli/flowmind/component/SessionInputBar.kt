package com.heli.flowmind.component

import androidx.compose.runtime.Composable
import com.heli.flowmind.base.Button
import com.heli.flowmind.base.TextField
import com.tencent.kuikly.compose.foundation.background
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.fillMaxWidth
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.foundation.shape.RoundedCornerShape
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.compose.ui.graphics.Color
import com.tencent.kuikly.compose.ui.unit.dp

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