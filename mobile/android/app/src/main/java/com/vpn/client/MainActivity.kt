package com.vpn.client

import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity

class MainActivity : AppCompatActivity() {
    private lateinit var statusView: TextView
    private lateinit var serverInput: EditText
    private lateinit var tokenInput: EditText
    private lateinit var connectButton: Button

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        statusView = findViewById(R.id.statusText)
        serverInput = findViewById(R.id.serverInput)
        tokenInput = findViewById(R.id.tokenInput)
        connectButton = findViewById(R.id.connectButton)

        restoreCredentials()

        connectButton.setOnClickListener {
            when (VpnStateBus.current) {
                is VpnState.Connected, is VpnState.Connecting -> stopVpn()
                else -> startVpn()
            }
        }
    }

    override fun onStart() {
        super.onStart()
        VpnStateBus.observe(::render)
    }

    override fun onStop() {
        super.onStop()
        VpnStateBus.observe(null)
    }

    private fun render(state: VpnState) {
        val connecting = state is VpnState.Connecting
        val connected = state is VpnState.Connected

        statusView.text = when (state) {
            is VpnState.Disconnected -> getString(R.string.status_disconnected)
            is VpnState.Connecting -> getString(R.string.status_connecting)
            is VpnState.Connected -> getString(R.string.status_connected, state.ip)
            is VpnState.Failed -> getString(R.string.status_failed, state.reason)
        }

        connectButton.text =
            if (connected || connecting) getString(R.string.disconnect) else getString(R.string.connect)
        serverInput.isEnabled = !connected && !connecting
        tokenInput.isEnabled = !connected && !connecting
    }

    private fun startVpn() {
        val server = serverInput.text.toString().trim()
        val token = tokenInput.text.toString().trim()
        if (server.isEmpty() || token.isEmpty()) {
            statusView.text = getString(R.string.error_missing_fields)
            return
        }
        saveCredentials(server, token)

        // Android shows a consent dialog the first time an app asks to act as a VPN.
        val consent = VpnService.prepare(this)
        if (consent != null) {
            pendingServer = server
            pendingToken = token
            startActivityForResult(consent, REQUEST_VPN)
            return
        }
        launchVpn(server, token)
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != REQUEST_VPN) return

        if (resultCode == RESULT_OK) {
            launchVpn(pendingServer, pendingToken)
        } else {
            VpnStateBus.publish(VpnState.Failed(getString(R.string.error_permission_denied)))
        }
    }

    private fun launchVpn(server: String, token: String) {
        startForegroundService(Intent(this, VPNService::class.java).apply {
            action = VPNService.ACTION_CONNECT
            putExtra(VPNService.EXTRA_SERVER, server)
            putExtra(VPNService.EXTRA_TOKEN, token)
        })
        VpnStateBus.publish(VpnState.Connecting)
    }

    private fun stopVpn() {
        startService(Intent(this, VPNService::class.java).apply {
            action = VPNService.ACTION_DISCONNECT
        })
    }

    private fun saveCredentials(server: String, token: String) {
        prefs().edit()
            .putString(KEY_SERVER, server)
            .putString(KEY_TOKEN, token)
            .apply()
    }

    private fun restoreCredentials() {
        val saved = prefs()
        serverInput.setText(saved.getString(KEY_SERVER, DEFAULT_SERVER))
        tokenInput.setText(saved.getString(KEY_TOKEN, ""))
    }

    private fun prefs() = getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    companion object {
        private const val REQUEST_VPN = 1001
        private const val PREFS = "vpn_client"
        private const val KEY_SERVER = "server"
        private const val KEY_TOKEN = "token"
        private const val DEFAULT_SERVER = "10.0.2.2:8443"

        private var pendingServer = ""
        private var pendingToken = ""
    }
}
