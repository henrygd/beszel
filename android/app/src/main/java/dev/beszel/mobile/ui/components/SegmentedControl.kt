package dev.beszel.mobile.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import dev.beszel.mobile.ui.theme.BeszelMotion
import dev.beszel.mobile.ui.theme.rememberReducedMotion

/**
 * Pill segmented control with a sliding selection indicator. Used for chart
 * ranges and metric tabs. Items have equal weight and a 48dp touch height.
 */
@Composable
fun <T> SegmentedControl(
    options: List<T>,
    selected: T,
    onSelect: (T) -> Unit,
    label: @Composable (T) -> String,
    modifier: Modifier = Modifier,
    indicatorColor: Color = MaterialTheme.colorScheme.primary.copy(alpha = 0.16f),
    selectedContentColor: Color = MaterialTheme.colorScheme.primary,
) {
    require(options.isNotEmpty()) { "SegmentedControl needs at least one option" }
    BoxWithConstraints(modifier) {
        val itemWidth = maxWidth / options.size
        val targetFraction = options.indexOf(selected).coerceAtLeast(0) / options.size.toFloat()
        val reducedMotion = rememberReducedMotion()
        val animatedFraction by animateFloatAsState(
            targetValue = targetFraction,
            animationSpec = if (reducedMotion) BeszelMotion.springGentle() else BeszelMotion.springStandard(),
            label = "segment-indicator",
        )
        Surface(
            modifier = Modifier.fillMaxWidth(),
            shape = MaterialTheme.shapes.small,
            color = MaterialTheme.colorScheme.surfaceContainer,
            border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        ) {
            Box(Modifier.height(48.dp)) {
                Box(
                    Modifier
                        .width(itemWidth)
                        .fillMaxHeight()
                        .graphicsLayer { translationX = animatedFraction * size.width }
                        .padding(4.dp)
                        .clip(MaterialTheme.shapes.small)
                        .background(indicatorColor),
                )
                Row(Modifier.fillMaxHeight()) {
                    options.forEach { option ->
                        val isSelected = option == selected
                        Box(
                            modifier = Modifier
                                .weight(1f)
                                .fillMaxHeight()
                                .clickable(
                                    interactionSource = remember { MutableInteractionSource() },
                                    indication = null,
                                    role = Role.RadioButton,
                                ) { onSelect(option) },
                            contentAlignment = Alignment.Center,
                        ) {
                            Text(
                                text = label(option),
                                style = MaterialTheme.typography.labelLarge,
                                color = if (isSelected) selectedContentColor else MaterialTheme.colorScheme.onSurfaceVariant,
                                textAlign = TextAlign.Center,
                                maxLines = 1,
                            )
                        }
                    }
                }
            }
        }
    }
}
