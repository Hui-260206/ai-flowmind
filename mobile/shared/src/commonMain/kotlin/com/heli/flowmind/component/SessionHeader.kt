package com.heli.flowmind.component

import androidx.compose.runtime.Composable
import com.heli.flowmind.theme.AppColors
import com.heli.flowmind.theme.AppDimens
import com.tencent.kuikly.compose.foundation.background
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.Row
import com.tencent.kuikly.compose.foundation.layout.Column
import com.tencent.kuikly.compose.foundation.layout.fillMaxWidth
import com.tencent.kuikly.compose.foundation.layout.height
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.foundation.clickable
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.compose.ui.platform.LocalActivity
import com.tencent.kuikly.compose.ui.unit.dp

@Composable
fun SessionHeader(
    title: String,
    actionsEnabled: Boolean,
    onOpenSessions: () -> Unit,
    onCreateSession: () -> Unit,
) {
    val statusBarHeight = LocalActivity.current.pageData.statusBarHeight
    Column(
        modifier = Modifier.fillMaxWidth()
            .background(AppColors.Primary)
            .padding(top = statusBarHeight.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().height(AppDimens.HeaderHeight),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                modifier = Modifier.height(AppDimens.HeaderHeight).padding(horizontal = 16.dp)
                    .then(if (actionsEnabled) Modifier.clickable(onClick = onOpenSessions) else Modifier),
                contentAlignment = Alignment.Center,
            ) {
                Text(text = "☰", color = AppColors.PrimaryText, fontSize = AppDimens.HeaderTitleSize)
            }
            Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.Center) {
                Text(text = title, color = AppColors.PrimaryText, fontSize = AppDimens.HeaderTitleSize, maxLines = 1)
            }
            Box(
                modifier = Modifier.height(AppDimens.HeaderHeight).padding(horizontal = 16.dp)
                    .then(if (actionsEnabled) Modifier.clickable(onClick = onCreateSession) else Modifier),
                contentAlignment = Alignment.Center,
            ) {
                Text(text = "＋", color = AppColors.PrimaryText, fontSize = AppDimens.HeaderTitleSize)
            }
        }
    }
}
