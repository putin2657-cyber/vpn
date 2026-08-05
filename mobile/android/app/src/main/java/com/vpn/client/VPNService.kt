package com.vpn.client

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.FileInputStream
import java.io.FileOutputStream
import java.util.concurrent.atomic.AtomicBoolean
import javax.net.ssl.SSLSocket

class VPNService : VpnService() {
    companion object {
        const val ACTION_CONNECT = "com.vpn.client.CONNECT"
        const val ACTION_DISCONNECT = "com.vpn.client.DISCONNECT"
        const val EXTRA_SERVER = "server"
        const val EXTRA_TOKEN = "token"
        private const val TAG = "VPNService"
        private const val CHANNEL_ID = "vpn_channel"
    }

    private var tunInterface: ParcelFileDescriptor? = null
    private var socket: SSLSocket? = null
    private val running = AtomicBoolean(false)

    // Пакеты из TUN и keepalive пишутся в один сокет из разных потоков.
    private val writeLock = Any()

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_DISCONNECT -> {
                stopVpn()
                return START_NOT_STICKY
            }
            ACTION_CONNECT -> {
                val server = intent.getStringExtra(EXTRA_SERVER) ?: return START_NOT_STICKY
                val token = intent.getStringExtra(EXTRA_TOKEN) ?: return START_NOT_STICKY
                startForeground(1, buildNotification("Connecting…"))
                Thread { startVpn(server, token) }.start()
            }
        }
        return START_STICKY
    }

    private fun startVpn(serverAddress: String, token: String) {
        try {
            val (host, port) = VPNProtocol.parseHostPort(serverAddress)
            val sslSocket = VPNProtocol.createTrustAllSocket(host, port)
            val assignedIP = VPNProtocol.authenticate(sslSocket, token)
            Log.i(TAG, "Authenticated, IP=$assignedIP")

            val builder = Builder()
                .setSession("Custom VPN")
                .addAddress(assignedIP, 24)
                .addRoute("10.8.0.0", 24)
                .setMtu(1500)

            tunInterface = builder.establish()
            socket = sslSocket
            running.set(true)

            updateNotification("Connected ($assignedIP)")
            tunnelLoop(sslSocket)
        } catch (e: Exception) {
            Log.e(TAG, "VPN failed", e)
            updateNotification("Error: ${e.message}")
            stopVpn()
        }
    }

    private fun tunnelLoop(sslSocket: SSLSocket) {
        val tunIn = FileInputStream(tunInterface!!.fileDescriptor)
        val tunOut = FileOutputStream(tunInterface!!.fileDescriptor)
        val netIn = DataInputStream(sslSocket.getInputStream())
        val netOut = DataOutputStream(sslSocket.getOutputStream())

        Thread {
            val buffer = ByteArray(1500)
            while (running.get()) {
                try {
                    val read = tunIn.read(buffer)
                    if (read <= 0) continue
                    synchronized(writeLock) { VPNProtocol.writePacket(netOut, buffer.copyOf(read)) }
                } catch (e: Exception) {
                    if (running.get()) Log.e(TAG, "TUN->net error", e)
                    break
                }
            }
        }.start()

        // Сервер разрывает соединение с клиентом, который слишком долго молчит.
        Thread {
            try {
                while (running.get()) {
                    Thread.sleep(VPNProtocol.KEEPALIVE_INTERVAL_MS)
                    if (!running.get()) break
                    synchronized(writeLock) { VPNProtocol.writeKeepalive(netOut) }
                }
            } catch (e: Exception) {
                if (running.get()) Log.e(TAG, "keepalive error", e)
            }
        }.start()

        try {
            while (running.get()) {
                val packet = VPNProtocol.readPacket(netIn) ?: continue
                tunOut.write(packet)
                tunOut.flush()
            }
        } catch (e: Exception) {
            if (running.get()) Log.e(TAG, "net->TUN error", e)
        } finally {
            stopVpn()
        }
    }

    private fun stopVpn() {
        running.set(false)
        try { socket?.close() } catch (_: Exception) {}
        try { tunInterface?.close() } catch (_: Exception) {}
        socket = null
        tunInterface = null
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun buildNotification(text: String): Notification {
        val manager = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            manager.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "VPN", NotificationManager.IMPORTANCE_LOW)
            )
        }
        val pending = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE
        )
        return Notification.Builder(this, CHANNEL_ID)
            .setContentTitle("VPN Client")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setContentIntent(pending)
            .build()
    }

    private fun updateNotification(text: String) {
        val manager = getSystemService(NotificationManager::class.java)
        manager.notify(1, buildNotification(text))
    }

    override fun onDestroy() {
        stopVpn()
        super.onDestroy()
    }
}
