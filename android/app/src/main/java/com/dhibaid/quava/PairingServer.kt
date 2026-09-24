package com.dhibaid.quava

import android.util.Log
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket
import kotlin.random.Random

data class PairingState(
    val status: String = "Ready to advertise and accept pairing requests",
    val paired: Boolean = false,
    val pairingCode: String = "",
    val pendingPeerName: String = "",
    val confirmationPending: Boolean = false,
)

class PairingServer(
    private val scope: CoroutineScope,
    private val proxyHost: String = "127.0.0.1",
    private val proxyPort: Int = 48273,
) {
    private val _state = MutableStateFlow(PairingState())
    val state: StateFlow<PairingState> = _state.asStateFlow()

    private var serverSocket: ServerSocket? = null
    private var pendingSocket: Socket? = null
    private val lock = Any()

    fun start(port: Int): Boolean {
        return try {
            val ss = ServerSocket()
            ss.reuseAddress = true
            ss.bind(InetSocketAddress(port)) // all IPv4 interfaces
            serverSocket = ss
            update { it.copy(status = "Listening on port ${ss.localPort}, waiting for pairing") }
            scope.launch(Dispatchers.IO) { acceptLoop(ss) }
            true
        } catch (e: Exception) {
            update { it.copy(status = "TCP server failed to listen on $port") }
            Log.e(TAG, "listen failed", e)
            false
        }
    }

    fun stop() {
        synchronized(lock) {
            pendingSocket?.closeQuietly()
            pendingSocket = null
        }
        serverSocket?.closeQuietly()
        serverSocket = null
    }

    private suspend fun acceptLoop(ss: ServerSocket) {
        while (currentCoroutineContext().isActive && !ss.isClosed) {
            val sock = try {
                ss.accept()
            } catch (e: Exception) {
                break
            }
            val accepted = synchronized(lock) {
                if (pendingSocket != null) {
                    false // only one pending pairing at a time
                } else {
                    pendingSocket = sock
                    true
                }
            }
            if (!accepted) {
                sock.closeQuietly()
                continue
            }

            Log.d(TAG, "new connection from ${sock.inetAddress.hostAddress}:${sock.port}")
            val code = Random.nextInt(1_000_000)
            update {
                it.copy(
                    pendingPeerName = "${sock.inetAddress.hostAddress}:${sock.port}",
                    pairingCode = "%03d %03d".format(code / 1000, code % 1000),
                    confirmationPending = true,
                    status = "Awaiting user confirmation for pairing",
                )
            }
            watchForDisconnect(sock)
        }
    }

    /** While the user hasn't answered yet, notice if the peer hangs up. */
    private fun watchForDisconnect(sock: Socket) {
        scope.launch(Dispatchers.IO) {
            try {
                // read() returns -1 on EOF. Peer shouldn't send before confirmation,
                // but if it does we just wait; data is consumed by the proxy later.
                sock.soTimeout = 0
                val probe = sock.getInputStream()
                while (isActive) {
                    if (synchronized(lock) { pendingSocket !== sock }) return@launch // decided
                    if (sock.isClosed || probe.available() < 0) break
                    delay(300)
                    // detect a half-closed peer by a zero-length write attempt
                    try {
                        sock.getOutputStream().flush()
                    } catch (_: Exception) {
                        break
                    }
                }
            } catch (_: Exception) {
            }
            val stillPending = synchronized(lock) {
                if (pendingSocket === sock) {
                    pendingSocket = null; true
                } else false
            }
            if (stillPending) {
                sock.closeQuietly()
                resetToReady()
            }
        }
    }

    fun confirmPairing() {
        val inbound = synchronized(lock) {
            val s = pendingSocket
            pendingSocket = null
            s
        } ?: return

        update { it.copy(confirmationPending = false, pairingCode = "", pendingPeerName = "") }

        scope.launch(Dispatchers.IO) {
            val outbound = Socket()
            try {
                outbound.connect(InetSocketAddress(proxyHost, proxyPort), 5000)
                Log.d(TAG, "outbound connected to host responder")
                update { it.copy(paired = true, status = "Proxying pairing to host responder") }

                // Bidirectional pipe; when either direction ends, close both.
                coroutineScope {
                    val a = launch { pipe(inbound, outbound, "inbound->outbound") }
                    val b = launch { pipe(outbound, inbound, "outbound->inbound") }
                    select@ while (isActive && (a.isActive && b.isActive)) delay(100)
                    a.cancel(); b.cancel()
                }
            } catch (e: Exception) {
                Log.w(TAG, "outbound socket error: ${e.message}")
            } finally {
                inbound.closeQuietly()
                outbound.closeQuietly()
                resetToReady()
            }
        }
    }

    fun rejectPairing() {
        synchronized(lock) {
            pendingSocket?.closeQuietly()
            pendingSocket = null
        }
        update {
            it.copy(
                confirmationPending = false,
                paired = false,
                pairingCode = "",
                pendingPeerName = "",
                status = "Pairing rejected",
            )
        }
    }

    private suspend fun pipe(from: Socket, to: Socket, label: String) =
        withContext(Dispatchers.IO) {
            val buf = ByteArray(8192)
            try {
                val input = from.getInputStream()
                val output = to.getOutputStream()
                while (true) {
                    val n = input.read(buf)
                    if (n < 0) break
                    Log.d(TAG, "$label $n bytes")
                    output.write(buf, 0, n)
                    output.flush()
                }
            } catch (_: Exception) {
            }
        }

    private fun resetToReady() = update {
        it.copy(
            confirmationPending = false,
            pairingCode = "",
            pendingPeerName = "",
            status = "Ready to advertise and accept pairing requests",
        )
    }

    private inline fun update(block: (PairingState) -> PairingState) = _state.update(block)

    private fun java.io.Closeable.closeQuietly() = try {
        close()
    } catch (_: Exception) {
    }

    companion object {
        private const val TAG = "Quava"
    }
}