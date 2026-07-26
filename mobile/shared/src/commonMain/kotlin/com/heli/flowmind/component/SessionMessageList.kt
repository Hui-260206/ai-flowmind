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
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.ui.unit.Dp
import com.tencent.kuikly.compose.ui.unit.dp


@Composable
fun SessionMessageList(
    messages: List<Message>,
    modifier: Modifier = Modifier,
    // 底部留白 = 输入栏高度 + 键盘高度,让最后一条消息能滚到浮层输入栏上方的可见区
    bottomInset: Dp = 0.dp,
) {
    if (messages.isEmpty()) {
        Box(modifier = modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text(text = "开始你的第一条对话", color = AppColors.Hint)
        }
        return
    }

    val listState = rememberLazyListState()
    // 新消息到达时滚到底部
    LaunchedEffect(messages.lastOrNull()?.id) {
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
        items(messages, key = { it.id }) { msg ->
            MessageBubble(message = msg)
            Spacer(Modifier.height(AppDimens.MessageSpacing))
        }
    }
}
