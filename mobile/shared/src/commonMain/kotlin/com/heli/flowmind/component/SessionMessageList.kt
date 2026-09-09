package com.heli.flowmind.component

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import com.heli.flowmind.model.Message
import com.heli.flowmind.theme.AppColors
import com.heli.flowmind.theme.AppDimens
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.PaddingValues
import com.tencent.kuikly.compose.foundation.layout.Spacer
import com.tencent.kuikly.compose.foundation.layout.fillMaxSize
import com.tencent.kuikly.compose.foundation.layout.height
import com.tencent.kuikly.compose.foundation.lazy.LazyColumn
import com.tencent.kuikly.compose.foundation.lazy.items
import com.tencent.kuikly.compose.foundation.lazy.rememberLazyListState
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.compose.ui.unit.Dp
import com.tencent.kuikly.compose.ui.unit.dp

@Composable
fun SessionMessageList(
    sessionId: String?,
    messages: List<Message>,
    modifier: Modifier = Modifier,
    bottomInset: Dp = 0.dp,
    onRetry: () -> Unit = {},
) {
    if (messages.isEmpty()) {
        Box(modifier = modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text(text = "开始你的第一条对话", color = AppColors.Hint)
        }
        return
    }

    val listState = rememberLazyListState()
    val tailState = messageListTailState(sessionId, messages)
    LaunchedEffect(tailState) {
        listState.animateScrollToItem(messages.lastIndex)
    }

    LazyColumn(
        state = listState,
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(
            top = AppDimens.MessageSpacing,
            bottom = AppDimens.MessageSpacing + bottomInset,
        ),
    ) {
        items(messages, key = { it.id }) { message ->
            MessageBubble(message = message, onRetry = onRetry)
            Spacer(Modifier.height(AppDimens.MessageSpacing))
        }
    }
}
