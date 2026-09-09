package com.heli.flowmind.component

import androidx.compose.runtime.Composable
import com.heli.flowmind.model.Message
import com.heli.flowmind.model.MessageRole
import com.heli.flowmind.model.MessageStatus
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.Row
import com.tencent.kuikly.compose.foundation.layout.fillMaxWidth
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.heli.flowmind.theme.AppColors
import com.heli.flowmind.theme.AppDimens
import com.heli.flowmind.theme.AppShapes
import com.tencent.kuikly.compose.foundation.background
import com.tencent.kuikly.compose.foundation.layout.Arrangement
import com.tencent.kuikly.compose.foundation.layout.BoxWithConstraints
import com.tencent.kuikly.compose.foundation.layout.widthIn
import com.tencent.kuikly.compose.foundation.layout.Column
import com.tencent.kuikly.compose.foundation.clickable
import com.tencent.kuikly.compose.ui.graphics.Color

@Composable
fun MessageBubble(message: Message, onRetry: () -> Unit = {}) {
    val isUser = message.role == MessageRole.USER
    val bgColor = if (isUser) AppColors.BubbleUser else AppColors.BubbleAssistant
    val fgColor = if (isUser) AppColors.OnBubbleUser else AppColors.OnBubbleAssistant

    BoxWithConstraints(
        modifier = Modifier.fillMaxWidth().padding(horizontal = AppDimens.MessageListPaddingH),
    ){
        //pe 给的 Dp(已扣除上面的 padding)
        val maxBubbleWidth = maxWidth * AppDimens.BubbleMaxWidthFraction

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = if (isUser) Arrangement.End else Arrangement.Start,
        ) {
            Column(
                modifier = Modifier
                    .widthIn(max = maxBubbleWidth)
                    .background(color = bgColor, shape = AppShapes.Bubble)
                    .padding(horizontal = AppDimens.BubblePaddingH, vertical = AppDimens.BubblePaddingV)
            ) {
                Text(
                    text = message.content,
                    color = fgColor,
                    fontSize = AppDimens.BodyTextSize,
                )
                if (message.status == MessageStatus.SENDING) {
                    Text(text = "生成中…", color = AppColors.Hint, fontSize = AppDimens.BodyTextSize)
                }
                if (message.isRetryableAssistantFailure()) {
                    Text(
                        text = "重试",
                        color = Color(0xFFD04A4A),
                        fontSize = AppDimens.BodyTextSize,
                        modifier = Modifier.padding(top = AppDimens.MessageSpacing).clickable(onClick = onRetry),
                    )
                }
            }
        }
    }
}
