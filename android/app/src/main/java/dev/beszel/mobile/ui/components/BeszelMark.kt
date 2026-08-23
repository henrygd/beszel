package dev.beszel.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.beszel.mobile.ui.theme.BrandMint
import dev.beszel.mobile.ui.theme.BrandViolet

/** Brand tile: violet-to-mint gradient square with the Beszel "B". */
@Composable
fun BeszelMark(modifier: Modifier = Modifier, size: androidx.compose.ui.unit.Dp = 64.dp) {
    Box(
        modifier = modifier
            .size(size)
            .background(
                brush = Brush.linearGradient(listOf(BrandViolet, BrandMint)),
                shape = RoundedCornerShape(percent = 30),
            )
            .border(
                width = 1.dp,
                color = Color.White.copy(alpha = 0.22f),
                shape = RoundedCornerShape(percent = 30),
            ),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = "B",
            color = Color.White,
            style = MaterialTheme.typography.displaySmall.copy(
                fontSize = (size.value * 0.55f).sp,
            ),
        )
    }
}
