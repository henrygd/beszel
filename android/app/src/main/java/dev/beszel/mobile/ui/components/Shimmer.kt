package dev.beszel.mobile.ui.components

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.ui.theme.rememberReducedMotion

/** Sweeping highlight used by skeleton placeholders while data loads. */
@Composable
fun shimmerBrush(): Brush {
    val shimmerColor = MaterialTheme.colorScheme.surfaceContainerHighest
    val baseColor = MaterialTheme.colorScheme.surfaceContainer
    if (rememberReducedMotion()) {
        return Brush.linearGradient(listOf(baseColor, baseColor))
    }
    val transition = rememberInfiniteTransition(label = "shimmer")
    val translate by transition.animateFloat(
        initialValue = -360f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(tween(1300, easing = LinearEasing)),
        label = "shimmer-translate",
    )
    return Brush.linearGradient(
        colors = listOf(baseColor, shimmerColor, baseColor),
        start = Offset(translate, translate / 3f),
        end = Offset(translate + 320f, (translate + 320f) / 3f),
    )
}

@Composable
fun ShimmerLine(width: Dp, modifier: Modifier = Modifier, height: Dp = 12.dp) {
    val brush = shimmerBrush()
    Spacer(
        modifier = modifier
            .size(width = width, height = height)
            .background(brush, MaterialTheme.shapes.extraSmall),
    )
}

@Composable
fun ShimmerCircle(size: Dp, modifier: Modifier = Modifier) {
    val brush = shimmerBrush()
    Spacer(modifier = modifier.size(size).background(brush, CircleShape))
}

/** Skeleton mirroring the SystemCard layout so first paint doesn't jump. */
@Composable
fun SystemCardSkeleton(modifier: Modifier = Modifier) {
    val brush = shimmerBrush()
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                ShimmerCircle(34.dp)
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    ShimmerLine(120.dp)
                    ShimmerLine(80.dp, height = 10.dp)
                }
            }
            repeat(3) {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    ShimmerLine(46.dp, height = 10.dp)
                    Spacer(
                        Modifier
                            .fillMaxWidth()
                            .height(4.dp)
                            .background(brush, CircleShape),
                    )
                }
            }
        }
    }
}
