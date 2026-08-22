package dev.beszel.mobile

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.beszel.mobile.ui.BeszelApp
import dev.beszel.mobile.ui.theme.BeszelTheme

class MainActivity : ComponentActivity() {
    private val viewModel: AppViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            val state by viewModel.state.collectAsStateWithLifecycle()
            BeszelTheme(themeMode = state.themeMode, dynamicColor = state.dynamicColor) {
                BeszelApp(state = state, viewModel = viewModel)
            }
        }
    }
}
