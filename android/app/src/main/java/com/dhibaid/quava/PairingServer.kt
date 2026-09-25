package com.dhibaid.quava

import android.content.Context
import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket

data class PairingState(
    val status: String = "Ready to advertise and accept pairing requests",
    val paired: Boolean = false,
    val pairingCode: String = "",
    val pendingPeerName: String = "",
    val confirmationPending: Boolean = false,
)

/** TCP responder for the Quava pairing protocol. */
class PairingServer(
    private val scope: CoroutineScope,
    context: Context,
) {
    private val protocol = PairingProtocol(context.applicationContext)
    private val _state = MutableStateFlow(PairingState())
    val state: StateFlow<PairingState> = _state.asStateFlow()

    private var serverSocket: ServerSocket? = null
    private var activeSession: PairingProtocol.Session? = null
    private val lock = Any()

    fun start(port: Int): Boolean = try {
        ServerSocket().also { ss ->
            ss.reuseAddress = true
            ss.bind(InetSocketAddress(port))
            serverSocket = ss
            update { it.copy(status = "Listening on port ${ss.localPort}, waiting for pairing") }
            scope.launch(Dispatchers.IO) { acceptLoop(ss) }
        }
        true
    } catch (e: Exception) {
        update { it.copy(status = "TCP server failed to listen on $port") }
        Log.e(TAG, "listen failed", e)
        false
    }

    fun stop() {
        val session = synchronized(lock) { activeSession.also { activeSession = null } }
        session?.close()
        serverSocket?.closeQuietly()
        serverSocket = null
    }

    private suspend fun acceptLoop(ss: ServerSocket) {
        while (ss.isBound && !ss.isClosed) {
            val socket = try {
                ss.accept()
            } catch (_: Exception) {
                break
            }
            if (synchronized(lock) { activeSession != null }) {
                socket.closeQuietly()
                continue
            }
            scope.launch(Dispatchers.IO) { handle(socket) }
        }
    }

    private fun handle(socket: Socket) {
        try {
            val session = protocol.begin(socket) { peerName, code, newSession ->
                synchronized(lock) {
                    if (activeSession != null) throw PairingProtocol.BusyException()
                    activeSession = newSession
                }
                update {
                    it.copy(
                        paired = false,
                        pendingPeerName = peerName,
                        pairingCode = "%03d %03d".format(code / 1000, code % 1000),
                        confirmationPending = true,
                        status = "Verify the pairing code, then confirm",
                    )
                }
            }
            protocol.complete(session)
            update {
                it.copy(
                    paired = true,
                    confirmationPending = false,
                    pairingCode = "",
                    pendingPeerName = "",
                    status = "Paired with ${session.peerName}",
                )
            }
        } catch (e: PairingProtocol.BusyException) {
            socket.closeQuietly()
        } catch (e: Exception) {
            Log.w(TAG, "pairing failed: ${e.message}", e)
            update {
                it.copy(
                    confirmationPending = false,
                    pairingCode = "",
                    pendingPeerName = "",
                    status = "Pairing failed: ${e.message ?: "connection closed"}",
                )
            }
        } finally {
            synchronized(lock) {
                if (activeSession?.socket === socket) activeSession = null
            }
            socket.closeQuietly()
        }
    }

    fun confirmPairing() {
        synchronized(lock) { activeSession }?.confirm(true)
    }

    fun rejectPairing() {
        val session = synchronized(lock) { activeSession }
        session?.confirm(false)
        // Interrupt a pending socket read if Android rejects before the peer
        // sends PAIR_AUTHENTICATE.
        session?.close()
        update {
            it.copy(
                paired = false,
                confirmationPending = false,
                pairingCode = "",
                pendingPeerName = "",
                status = "Pairing rejected",
            )
        }
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
