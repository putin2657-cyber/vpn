package com.vpn.client

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

        serverInput.setText("10.0.2.2:8443")

        connectButton.setOnClickListener {
            if (connectButton.text == getString(R.string.disconnect)) {
                stopVpn()
            } else {
                startVpn()
            }
        }
    }

    private fun startVpn() {
        val server = serverInput.text.toString().trim()
        val token = tokenInput.text.toString().trim()
        if (server.isEmpty() || token.isEmpty()) {
            statusView.text = getString(R.string.error_missing_fields)
            return
        }

        val prepare = VpnService.prepare(this)
        if (prepare != null) {
            startActivityForResult(prepare, REQUEST_VPN)
            pendingServer = server
            pendingToken = token
            return
        }
        launchVpn(server, token)
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQUEST_VPN && resultCode == RESULT_OK) {
            launchVpn(pendingServer, pendingToken)
        }
    }

    private fun launchVpn(server: String, token: String) {
        val intent = Intent(this, VPNService::class.java).apply {
            action = VPNService.ACTION_CONNECT
            putExtra(VPNService.EXTRA_SERVER, server)
            putExtra(VPNService.EXTRA_TOKEN, token)
        }
        startForegroundService(intent)
        statusView.text = getString(R.string.status_connecting)
        connectButton.text = getString(R.string.disconnect)
        serverInput.isEnabled = false
        tokenInput.isEnabled = false
    }

    private fun stopVpn() {
        startService(Intent(this, VPNService::class.java).apply {
            action = VPNService.ACTION_DISCONNECT
        })
        statusView.text = getString(R.string.status_disconnected)
        connectButton.text = getString(R.string.connect)
        serverInput.isEnabled = true
        tokenInput.isEnabled = true
    }

    companion object {
        private const val REQUEST_VPN = 1001
        private var pendingServer = ""
        private var pendingToken = ""
    }
}
