package com.familyvpn.app.vpn

import android.content.Context
import android.util.Log
import com.familyvpn.app.data.ConfigStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withContext
import org.amnezia.awg.backend.Backend
import org.amnezia.awg.backend.GoBackend
import org.amnezia.awg.backend.Tunnel
import org.amnezia.awg.config.Config
import java.io.ByteArrayInputStream
import java.lang.reflect.Proxy

enum class VpnUiState {
    Disconnected,
    Connecting,
    Connected,
    Error,
}

class TunnelController(
    context: Context,
    private val store: ConfigStore,
) {
    private val appContext = context.applicationContext
    private val backend: Backend = createBackend(appContext)
    private val tunnel = FamilyTunnel { state ->
        _state.value = when (state) {
            Tunnel.State.UP -> VpnUiState.Connected
            Tunnel.State.DOWN -> VpnUiState.Disconnected
            else -> _state.value
        }
    }

    private val _state = MutableStateFlow(VpnUiState.Disconnected)
    val state: StateFlow<VpnUiState> = _state.asStateFlow()

    private val _message = MutableStateFlow<String?>(null)
    val message: StateFlow<String?> = _message.asStateFlow()

    fun refreshFromBackend() {
        _state.value = when (backend.getState(tunnel)) {
            Tunnel.State.UP -> VpnUiState.Connected
            else -> VpnUiState.Disconnected
        }
    }

    suspend fun connect(): Boolean = withContext(Dispatchers.IO) {
        _message.value = null
        val raw = store.getConfig()
        if (raw.isBlank()) {
            _state.value = VpnUiState.Error
            _message.value = "Сначала импортируйте .conf"
            return@withContext false
        }
        _state.value = VpnUiState.Connecting
        try {
            val patched = ensureExcludedApps(raw)
            val config = Config.parse(ByteArrayInputStream(patched.toByteArray(Charsets.UTF_8)))
            backend.setState(tunnel, Tunnel.State.UP, config)
            _state.value = VpnUiState.Connected
            true
        } catch (e: Exception) {
            Log.e(TAG, "connect failed", e)
            _state.value = VpnUiState.Error
            _message.value = e.message ?: "Не удалось подключиться"
            false
        }
    }

    suspend fun disconnect() = withContext(Dispatchers.IO) {
        try {
            backend.setState(tunnel, Tunnel.State.DOWN, null)
        } catch (e: Exception) {
            Log.e(TAG, "disconnect failed", e)
            _message.value = e.message
        } finally {
            _state.value = VpnUiState.Disconnected
        }
    }

    companion object {
        private const val TAG = "TunnelController"

        // Keep marketplaces/banks on cellular/Wi‑Fi even with an older .conf.
		private val EXCLUDED_APPS = listOf(
            "com.wildberries.ru",
            "ru.ozon.app.android",
            "ru.sberbankmobile",
            "com.idamob.tinkoff.android",
            "ru.vtb24.mobilebanking.android",
            "ru.alfabank.mobile.android",
            "ru.rostel",
            "com.yandex.browser",
            "ru.yandex.yandexmaps",
            "ru.yandex.taxi",
            "com.valvesoftware.android.steam.community",
        )

        internal fun ensureExcludedApps(raw: String): String {
            val apps = EXCLUDED_APPS.joinToString(", ")
            if (raw.contains("ExcludedApplications", ignoreCase = true)) {
                return raw.replace(
                    Regex("""(?im)^ExcludedApplications\s*=\s*.*$"""),
                    "ExcludedApplications = $apps",
                )
            }
            return raw.replace(
                Regex("""(?im)^\[Interface\]\s*$"""),
                "[Interface]\nExcludedApplications = $apps",
            )
        }

        /**
         * amneziawg-android 1.x uses GoBackend(Context); 2.x also wants TunnelActionHandler.
         */
        fun createBackend(context: Context): Backend {
            return try {
                val handlerClass = Class.forName("org.amnezia.awg.backend.TunnelActionHandler")
                val handler = Proxy.newProxyInstance(
                    handlerClass.classLoader,
                    arrayOf(handlerClass),
                ) { _, _, _ -> null }
                GoBackend::class.java
                    .getConstructor(Context::class.java, handlerClass)
                    .newInstance(context, handler) as Backend
            } catch (_: Throwable) {
                GoBackend::class.java
                    .getConstructor(Context::class.java)
                    .newInstance(context) as Backend
            }
        }
    }
}
