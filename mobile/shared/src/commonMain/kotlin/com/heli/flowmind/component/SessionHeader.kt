package com.heli.flowmind.component

import androidx.compose.runtime.Composable
import com.heli.flowmind.theme.AppColors
import com.heli.flowmind.theme.AppDimens
import com.tencent.kuikly.compose.foundation.background
import com.tencent.kuikly.compose.foundation.layout.Box
import com.tencent.kuikly.compose.foundation.layout.Column
import com.tencent.kuikly.compose.foundation.layout.fillMaxWidth
import com.tencent.kuikly.compose.foundation.layout.height
import com.tencent.kuikly.compose.foundation.layout.padding
import com.tencent.kuikly.compose.material3.Text
import com.tencent.kuikly.compose.ui.Alignment
import com.tencent.kuikly.compose.ui.Modifier
import com.tencent.kuikly.compose.ui.platform.LocalActivity
import com.tencent.kuikly.compose.ui.unit.dp

@Composable
fun SessionHeader() {
    val statusBarHeight = LocalActivity.current.pageData.statusBarHeight
    Column(
        modifier = Modifier.fillMaxWidth()
            .background(AppColors.Primary)
            .padding(top = statusBarHeight.dp),
    ) {
        Box(
            modifier = Modifier.fillMaxWidth().height(AppDimens.HeaderHeight),
            contentAlignment = Alignment.Center,
        ) {
            Text(text = "会话", color = AppColors.PrimaryText, fontSize = AppDimens.HeaderTitleSize)
        }
    }
}