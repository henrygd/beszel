package dev.beszel.mobile.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import dev.beszel.mobile.ui.theme.BeszelMotion
import dev.beszel.mobile.ui.theme.metricValue
import dev.beszel.mobile.ui.theme.rememberReducedMotion

/** Monospaced value that counts between states instead of snapping. */
@Composable
fun AnimatedNumber(
    value: Float,
    format: (Float) -> String,
    modifier: Modifier = Modifier,
    style: TextStyle = MaterialTheme.typography.metricValue,
    color: Color = Color.Unspecified,
) {
    val reducedMotion = rememberReducedMotion()
    val animated by animateFloatAsState(
        targetValue = value,
        animationSpec = if (reducedMotion) BeszelMotion.springGentle() else BeszelMotion.springStandard(),
        label = "animated-number",
    )
    Text(
        text = format(animated),
        style = style,
        color = color,
        modifier = modifier,
    )
}
