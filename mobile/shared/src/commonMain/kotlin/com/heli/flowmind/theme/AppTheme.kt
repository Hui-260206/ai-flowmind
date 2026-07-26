package com.heli.flowmind.theme

import com.tencent.kuikly.compose.foundation.shape.RoundedCornerShape
import com.tencent.kuikly.compose.ui.graphics.Color
import com.tencent.kuikly.compose.ui.unit.dp
import com.tencent.kuikly.compose.ui.unit.sp

// ── 颜色 ──
object AppColors {
    val Primary = Color(0xFF7B7FE4)
    val PrimaryText = Color.White
    val Background = Color.White
    val Surface = Color(0xFFF2F2F2)
    val OnSurface = Color.Black
    val Hint = Color(0xFF999999)
    val BubbleUser = Primary            // 复用主色，保持品牌一致
    val BubbleAssistant = Surface       // 对方用浅灰
    val OnBubbleUser = PrimaryText
    val OnBubbleAssistant = OnSurface
}

// ── 圆角 ──
object AppShapes {
    val InputBar = RoundedCornerShape(24.dp)
    val SendButton = RoundedCornerShape(20.dp)
    val Bubble = RoundedCornerShape(12.dp) //消息气泡
}

// ── 间距 / 尺寸 ──
object AppDimens {
    val HeaderHeight = 48.dp
    val HeaderTitleSize = 18.sp

    val InputOuterPadding = 8.dp
    val InputWidthFraction = 0.92f
    val InputInnerPadding = 4.dp
    val InputTextStart = 12.dp
    val InputTextEnd = 56.dp
    val InputTextVertical = 8.dp

    val ButtonHorizontal = 14.dp
    val ButtonVertical = 8.dp
    val BubbleMaxWidthFraction = 0.78f   // 气泡最宽不超屏幕 78%，避免一行铺满
    val BubblePaddingH = 12.dp
    val BubblePaddingV = 8.dp
    val MessageSpacing = 8.dp            // 消息间距
    val MessageListPaddingH = 12.dp
    val BodyTextSize = 16.sp
}
