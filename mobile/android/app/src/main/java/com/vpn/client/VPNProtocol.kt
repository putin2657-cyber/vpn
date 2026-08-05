package com.vpn.client

import android.util.Log
import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.InputStream
import java.net.InetSocketAddress
import java.nio.ByteBuffer
import java.nio.ByteOrder
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLSocket
import javax.net.ssl.X509TrustManager

object VPNProtocol {
    private const val TAG = "VPNProtocol"
    const val MAX_PACKET_SIZE = 65535
    private const val MAX_AUTH_LINE = 512

    // Должно совпадать с сервером: keepalive обязан уложиться в его окно простоя.
    const val KEEPALIVE_INTERVAL_MS = 30_000L
    private const val IDLE_TIMEOUT_MS = 90_000

    fun createTrustAllSocket(host: String, port: Int): SSLSocket {
        val trustAll = arrayOf(object : X509TrustManager {
            override fun checkClientTrusted(chain: Array<java.security.cert.X509Certificate>, authType: String) {}
            override fun checkServerTrusted(chain: Array<java.security.cert.X509Certificate>, authType: String) {}
            override fun getAcceptedIssuers(): Array<java.security.cert.X509Certificate> = emptyArray()
        })
        val context = SSLContext.getInstance("TLS")
        context.init(null, trustAll, java.security.SecureRandom())
        val socket = context.socketFactory.createSocket() as SSLSocket
        socket.connect(InetSocketAddress(host, port), 10_000)
        socket.soTimeout = IDLE_TIMEOUT_MS
        socket.startHandshake()
        return socket
    }

    fun authenticate(socket: SSLSocket, token: String): String {
        val out = socket.getOutputStream()
        out.write((token + "\n").toByteArray())
        out.flush()

        val line = readAuthLine(socket.getInputStream())
        if (line.startsWith("AUTH_FAIL")) {
            throw SecurityException("Authentication failed")
        }
        if (!line.startsWith("OK")) {
            throw IllegalStateException("Unexpected auth response: $line")
        }
        val parts = line.split(" ")
        require(parts.size >= 2) { "Missing assigned IP" }
        return parts[1]
    }

    // Читаем побайтово: BufferedReader утащил бы в свой буфер начало первого пакета.
    private fun readAuthLine(input: InputStream): String {
        val line = StringBuilder()
        while (line.length < MAX_AUTH_LINE) {
            val b = input.read()
            if (b == -1) throw IllegalStateException("Empty auth response")
            if (b == '\n'.code) return line.toString().trim()
            line.append(b.toChar())
        }
        throw IllegalStateException("Auth line exceeds $MAX_AUTH_LINE bytes")
    }

    fun writePacket(out: DataOutputStream, packet: ByteArray) {
        require(packet.isNotEmpty() && packet.size <= MAX_PACKET_SIZE) { "Invalid packet size" }
        out.writeInt(packet.size)
        out.write(packet)
        out.flush()
    }

    /** Кадр нулевой длины: держит простаивающий туннель открытым. */
    fun writeKeepalive(out: DataOutputStream) {
        out.writeInt(0)
        out.flush()
    }

    /** Возвращает null, если пришёл keepalive. */
    fun readPacket(input: DataInputStream): ByteArray? {
        val length = input.readInt()
        if (length == 0) return null
        require(length in 1..MAX_PACKET_SIZE) { "Invalid packet length: $length" }
        val packet = ByteArray(length)
        input.readFully(packet)
        return packet
    }

    fun parseHostPort(address: String): Pair<String, Int> {
        val parts = address.split(":")
        val host = parts[0]
        val port = if (parts.size > 1) parts[1].toIntOrNull() ?: 8443 else 8443
        return host to port
    }
}
