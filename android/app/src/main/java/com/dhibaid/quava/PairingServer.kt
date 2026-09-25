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
    val status: String = "Ready to advertise and accept connections",
    val paired: Boolean = false,
    val pairingCode: String = "",
    val pendingPeerName: String = "",
    val confirmationPending: Boolean = false,
    val connected: Boolean = false,
)

/** TCP server for both one-time pairing and authenticated runtime sessions. */
class PairingServer(
    private val scope: CoroutineScope,
    context: Context,
) {
    private val quavaService = context as? QuavaService
    private val protocol = PairingProtocol(context.applicationContext)
    private val _state = MutableStateFlow(PairingState())
    val state: StateFlow<PairingState> = _state.asStateFlow()

    private var serverSocket: ServerSocket? = null
    private var activePairing: PairingProtocol.Session? = null
    private var activeSocket: Socket? = null
    private val lock = Any()

    fun start(port: Int): Boolean = try {
        ServerSocket().also { ss ->
            ss.reuseAddress = true
            ss.bind(InetSocketAddress(port))
            serverSocket = ss
            update { it.copy(status = "Listening on port ${ss.localPort}, waiting for Quava connections") }
            scope.launch(Dispatchers.IO) { acceptLoop(ss) }
        }
        true
    } catch (e: Exception) {
        update { it.copy(status = "TCP server failed to listen on $port") }
        Log.e(TAG, "listen failed", e)
        false
    }

    fun stop() {
        val socket = synchronized(lock) {
            activePairing?.close()
            activePairing = null
            activeSocket.also { activeSocket = null }
        }
        socket?.closeQuietly()
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
            if (synchronized(lock) { activeSocket != null }) {
                socket.closeQuietly()
                continue
            }
            synchronized(lock) { activeSocket = socket }
            scope.launch(Dispatchers.IO) { handle(socket) }
        }
    }

    private fun handle(socket: Socket) {
        try {
            val first = protocol.readInitial(socket)
            when (first.type) {
                PairingProtocol.PAIR_REQUEST -> handlePairing(socket, first)
                PairingProtocol.SESSION_HELLO -> handleSession(socket, first)
                else -> error("unsupported initial message ${first.type}")
            }
        } catch (e: PairingProtocol.BusyException) {
            socket.closeQuietly()
        } catch (e: Exception) {
            Log.w(TAG, "connection failed: ${e.message}")
            update {
                it.copy(
                    confirmationPending = false,
                    pairingCode = "",
                    pendingPeerName = "",
                    connected = false,
                    status = "Connection failed: ${e.message ?: "connection closed"}"
                )
            }

            quavaService?.onDisconnected()
        } finally {
            synchronized(lock) {
                if (activePairing?.socket === socket) activePairing = null
                if (activeSocket === socket) activeSocket = null
            }
            socket.closeQuietly()
        }
    }

    private fun handlePairing(socket: Socket, first: PairingProtocol.Message) {
        val session = protocol.begin(socket, first) { peerName, code, newSession ->
            synchronized(lock) {
                if (activePairing != null) throw PairingProtocol.BusyException()
                activePairing = newSession
            }
            update {
                it.copy(
                    paired = false,
                    pendingPeerName = peerName,
                    pairingCode = "%03d %03d".format(code / 1000, code % 1000),
                    confirmationPending = true,
                    status = "Verify the pairing code, then confirm"
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
                connected = false,
                status = "Paired with ${session.peerName}"
            )
        }
    }

    private fun handleSession(socket: Socket, first: PairingProtocol.Message) {
        val session = protocol.beginSession(socket, first)
        update { it.copy(connected = true, status = "Connected to ${session.peerName}") }
        quavaService?.onConnected(session.peerName)
        protocol.runSession(session)
    }

    fun confirmPairing() {
        synchronized(lock) { activePairing }?.confirm(true)
    }

    fun rejectPairing() {
        val session = synchronized(lock) { activePairing }
        session?.confirm(false)
        session?.close()
        update {
            it.copy(
                paired = false,
                confirmationPending = false,
                pairingCode = "",
                pendingPeerName = "",
                status = "Pairing rejected"
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
