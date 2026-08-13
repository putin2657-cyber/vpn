package com.familyvpn.app

import android.content.Intent
import android.net.Uri
import android.net.VpnService
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.core.view.WindowCompat
import androidx.lifecycle.lifecycleScope
import com.familyvpn.app.ui.FamilyVpnTheme
import com.familyvpn.app.ui.HomeScreen
import com.familyvpn.app.vpn.VpnUiState
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    private val app get() = application as FamilyVpnApp

    private val prepareVpn = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == RESULT_OK) {
            lifecycleScope.launch { app.tunnelController.connect() }
        }
    }

    private var onConfigImported: ((String) -> Unit)? = null

    private val openConfig = registerForActivityResult(
        ActivityResultContracts.OpenDocument()
    ) { uri ->
        if (uri == null) return@registerForActivityResult
        runCatching {
            contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        val text = contentResolver.openInputStream(uri)?.bufferedReader()?.use { it.readText() }.orEmpty()
        if (app.configStore.looksLikeConfig(text)) {
            onConfigImported?.invoke(text)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window, false)
        handleImportIntent(intent)

        setContent {
            FamilyVpnTheme {
                val scope = rememberCoroutineScope()
                val vpnState by app.tunnelController.state.collectAsState()
                val vpnMessage by app.tunnelController.message.collectAsState()
                val savedConfig by app.configStore.configFlow.collectAsState(initial = "")
                var draft by remember { mutableStateOf("") }
                var statusNote by remember { mutableStateOf<String?>(null) }

                LaunchedEffect(Unit) { app.tunnelController.refreshFromBackend() }
                LaunchedEffect(savedConfig) {
                    if (draft.isBlank() && savedConfig.isNotBlank()) draft = savedConfig
                }

                onConfigImported = { text ->
                    draft = text
                    scope.launch {
                        app.configStore.saveConfig(text)
                        statusNote = "Конфиг импортирован из файла"
                    }
                }

                HomeScreen(
                    vpnState = vpnState,
                    hasConfig = savedConfig.isNotBlank() || draft.isNotBlank(),
                    draft = draft,
                    message = statusNote ?: vpnMessage,
                    onDraftChange = { draft = it },
                    onSaveConfig = {
                        scope.launch {
                            if (!app.configStore.looksLikeConfig(draft)) {
                                statusNote = "Это не похоже на AmneziaWG .conf"
                                return@launch
                            }
                            app.configStore.saveConfig(draft)
                            statusNote = "Конфиг сохранён"
                        }
                    },
                    onPickFile = {
                        openConfig.launch(arrayOf("text/*", "application/octet-stream", "*/*"))
                    },
                    onToggle = {
                        scope.launch {
                            statusNote = null
                            when (vpnState) {
                                VpnUiState.Connected, VpnUiState.Connecting -> {
                                    app.tunnelController.disconnect()
                                }
                                else -> {
                                    if (draft.isNotBlank() && app.configStore.looksLikeConfig(draft)) {
                                        app.configStore.saveConfig(draft)
                                    }
                                    val prep = VpnService.prepare(this@MainActivity)
                                    if (prep != null) {
                                        prepareVpn.launch(prep)
                                    } else {
                                        app.tunnelController.connect()
                                    }
                                }
                            }
                        }
                    },
                    onSupport = {
                        startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(BuildConfig.SUPPORT_TG)))
                    },
                    onCabinet = {
                        startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(BuildConfig.CABINET_URL)))
                    },
                )
            }
        }
    }

    override fun onResume() {
        super.onResume()
        app.tunnelController.refreshFromBackend()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleImportIntent(intent)
    }

    private fun handleImportIntent(intent: Intent?) {
        val uri = intent?.data ?: return
        val text = runCatching {
            contentResolver.openInputStream(uri)?.bufferedReader()?.use { it.readText() }.orEmpty()
        }.getOrNull() ?: return
        if (app.configStore.looksLikeConfig(text)) {
            onConfigImported?.invoke(text)
            lifecycleScope.launch {
                app.configStore.saveConfig(text)
            }
        }
    }
}
