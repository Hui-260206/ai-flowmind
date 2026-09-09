package com.heli.flowmind.component

import androidx.compose.runtime.Composable
import com.heli.flowmind.data.ChatSession
import com.heli.flowmind.theme.AppColors
import com.heli.flowmind.theme.AppDimens
import com.heli.flowmind.theme.AppShapes
import com.tencent.kuikly.compose.foundation.background
import com.tencent.kuikly.compose.foundation.clickable
import com.tencent.kuikly.compose.foundation.layout.Arrangement
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.Column
import com.tencent.kuikly.compose.foundation.layout.Row
import com.tencent.kuikly.compose.foundation.layout.Spacer
import com.tencent.kuikly.compose.foundation.layout.fillMaxSize
import com.tencent.kuikly.compose.foundation.layout.fillMaxWidth
import com.tencent.kuikly.compose.foundation.layout.height
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.foundation.layout.width
import com.tencent.kuikly.compose.foundation.lazy.LazyColumn
import com.tencent.kuikly.compose.foundation.lazy.items
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.compose.ui.graphics.Color
import com.tencent.kuikly.compose.ui.text.font.FontWeight
import com.tencent.kuikly.compose.ui.unit.dp
import com.tencent.kuikly.compose.ui.window.Dialog
import com.tencent.kuikly.compose.ui.window.DialogProperties

@Composable
fun SessionDrawerContent(
    sessions: List<ChatSession>,
    selectedSessionId: String?,
    actionsEnabled: Boolean,
    onCreateSession: () -> Unit,
    onSelectSession: (String) -> Unit,
    onRequestDelete: (ChatSession) -> Unit,
) {
    Column(modifier = Modifier.fillMaxSize().background(AppColors.Background).padding(16.dp)) {
        Text(text = "历史会话", color = AppColors.OnSurface, fontSize = AppDimens.HeaderTitleSize, fontWeight = FontWeight.Bold)
        Spacer(Modifier.height(12.dp))
        Box(
            modifier = Modifier.fillMaxWidth()
                .background(if (actionsEnabled) AppColors.Primary else AppColors.Hint, AppShapes.InputBar)
                .padding(vertical = 12.dp)
                .then(if (actionsEnabled) Modifier.clickable(onClick = onCreateSession) else Modifier),
            contentAlignment = Alignment.Center,
        ) {
            Text(text = "＋ 新建会话", color = AppColors.PrimaryText)
        }
        Spacer(Modifier.height(12.dp))
        LazyColumn(modifier = Modifier.fillMaxSize()) {
            items(sessions, key = { it.id }) { session ->
                val selected = session.id == selectedSessionId
                Row(
                    modifier = Modifier.fillMaxWidth()
                        .background(if (selected) Color(0xFFE8E9FF) else Color.Transparent, AppShapes.Bubble)
                        .padding(horizontal = 12.dp, vertical = 12.dp),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        text = session.title,
                        maxLines = 2,
                        color = AppColors.OnSurface,
                        modifier = Modifier.weight(1f).then(
                            if (actionsEnabled) Modifier.clickable { onSelectSession(session.id) } else Modifier
                        ),
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        text = "删除",
                        color = Color(0xFFD04A4A),
                        modifier = if (actionsEnabled) Modifier.clickable { onRequestDelete(session) } else Modifier,
                    )
                }
                Spacer(Modifier.height(6.dp))
            }
        }
    }
}

@Composable
fun DeleteSessionDialog(
    session: ChatSession,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Column(
                modifier = Modifier.fillMaxWidth(0.82f)
                    .background(AppColors.Background, AppShapes.Bubble)
                    .padding(20.dp),
            ) {
                Text(text = "删除会话？", fontSize = AppDimens.HeaderTitleSize, fontWeight = FontWeight.Bold)
                Spacer(Modifier.height(10.dp))
                Text(text = "“${session.title}”及其历史消息将从当前列表中移除。", color = AppColors.Hint)
                Spacer(Modifier.height(18.dp))
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                    Text(text = "取消", modifier = Modifier.clickable(onClick = onDismiss).padding(10.dp))
                    Spacer(Modifier.width(10.dp))
                    Text(text = "删除", color = Color(0xFFD04A4A), modifier = Modifier.clickable(onClick = onConfirm).padding(10.dp))
                }
            }
        }
    }
}
