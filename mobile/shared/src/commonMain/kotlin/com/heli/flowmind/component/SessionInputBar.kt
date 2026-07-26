package com.heli.flowmind.component

import androidx.compose.runtime.Composable
import com.heli.flowmind.base.Button
import com.heli.flowmind.base.TextField
import com.heli.flowmind.theme.AppColors
import com.heli.flowmind.theme.AppDimens
import com.heli.flowmind.theme.AppShapes
import com.tencent.kuikly.compose.foundation.background
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.fillMaxWidth
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.core.views.KeyboardParams

@Composable
fun SessionInputBar(
    value: String,
    modifier: Modifier = Modifier,
    onValueChange: (String) -> Unit,
    onSend: () -> Unit,
    onKeyboardHeightChange: (KeyboardParams) -> Unit,
) {
    // 外层 Box 让整个输入条在底部栏内水平居中
    Box(
        modifier = modifier.fillMaxWidth().padding(AppDimens.InputOuterPadding),
        contentAlignment = Alignment.Center,
    ) {
        // 居中显示的输入框容器（占 92% 宽，左右留白即为"居中"效果）
        Box(
            modifier = Modifier.fillMaxWidth(AppDimens.InputWidthFraction)
                .background(color = AppColors.Surface, shape = AppShapes.InputBar)
                .padding(AppDimens.InputInnerPadding),
        ) {
            TextField(
                modifier = Modifier.fillMaxWidth()
                    .padding(start = AppDimens.InputTextStart, end = AppDimens.InputTextEnd,
                             top = AppDimens.InputTextVertical, bottom = AppDimens.InputTextVertical),
                value = value,
                placeholder = "输入消息...",
                placeholderColor = AppColors.Hint,
                onValueChange = onValueChange,
                keyboardHeightChange = onKeyboardHeightChange,
            )
            Button(
                modifier = Modifier.align(Alignment.CenterEnd)
                    .background(color = AppColors.Primary, shape = AppShapes.SendButton)
                    .padding(horizontal = AppDimens.ButtonHorizontal, vertical = AppDimens.ButtonVertical),
                onClick = onSend,
            ) {
                Text(text = "发送", color = AppColors.PrimaryText)
            }
        }
    }
}