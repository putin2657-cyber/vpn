package com.familyvpn.app.vpn

import org.amnezia.awg.backend.Tunnel

class FamilyTunnel(
    private val onChanged: (Tunnel.State) -> Unit = {},
) : Tunnel {
    override fun getName(): String = NAME

    override fun onStateChange(newState: Tunnel.State) {
        onChanged(newState)
    }

    companion object {
        const val NAME = "familyvpn"
    }
}
