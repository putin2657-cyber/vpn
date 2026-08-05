package com.vpn.client

import android.os.Handler
import android.os.Looper

/** Состояние туннеля, которое экран показывает пользователю. */
sealed class VpnState {
    object Disconnected : VpnState()
    object Connecting : VpnState()
    data class Connected(val ip: String) : VpnState()
    data class Failed(val reason: String) : VpnState()
}

/**
 * Связывает VPNService с экраном. Сервис и активность живут в одном процессе,
 * так что хватает общего объекта — межпроцессное взаимодействие не нужно.
 */
object VpnStateBus {
    private val handler = Handler(Looper.getMainLooper())
    private var listener: ((VpnState) -> Unit)? = null

    @Volatile
    var current: VpnState = VpnState.Disconnected
        private set

    /** Подписчик сразу получает текущее состояние, чтобы пережить поворот экрана. */
    fun observe(listener: ((VpnState) -> Unit)?) {
        this.listener = listener
        val state = current
        listener?.let { handler.post { it(state) } }
    }

    fun publish(state: VpnState) {
        current = state
        val target = listener ?: return
        handler.post { target(state) }
    }
}
