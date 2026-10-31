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
    val hostName: String? = null,
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
    private val _mediaState = MutableStateFlow(HostMediaState())
    val mediaState: StateFlow<HostMediaState> = _mediaState.asStateFlow()

    fun localDeviceIdHex(): String = protocol.localDeviceIdHex()

    private var serverSocket: ServerSocket? = null
    private var activePairing: PairingProtocol.Session? = null
    private var activeSocket: Socket? = null
    private var activeSession: PairingProtocol.RuntimeSession? = null
    private val lock = Any()
    private val remoteInputDispatcher = RemoteInputDispatcher(scope, protocol) {
        synchronized(lock) { activeSession }
    }

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

    private suspend fun handle(socket: Socket) {
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
            Log.e(TAG, "connection failed", e)

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

    private suspend fun handleSession(
        socket: Socket,
        first: PairingProtocol.Message,
    ) {

        val session = protocol.beginSession(socket, first)
        var peerName = session.peerName

        update {
            it.copy(
                status = "Establishing session..."
            )
        }

        synchronized(lock) { activeSession = session }

        try {
            protocol.runSession(
                session = session,
                onReady = { hostName ->
                    peerName = hostName

                    Log.d(
                        "QuavaSession",
                        "Session established with Linux host: $hostName"
                    )

                    update {
                        it.copy(
                            connected = true,
                            hostName = hostName,
                            status = "Connected to: $hostName.",
                        )
                    }

                    quavaService?.onConnected(hostName)
                    sendMediaControl(MediaControl(MediaActions.STATE))
                },
                onPing = { quavaService?.onPing(peerName) },

                onRing = { quavaService?.onRing(peerName) },

                onClipboardSync = { message ->
                    quavaService?.onClipboardSync(message)
                },

                onNotificationSync = { message ->
                    quavaService?.onNotificationSync(message)
                },

                onNotificationClear = { message ->
                    quavaService?.onNotificationClear(message)
                },
                onNotificationAction = { message -> quavaService?.onAndroidNotificationAction(message) },
                onAndroidNotificationDismiss = { message -> quavaService?.onAndroidNotificationDismiss(message) },
                onMediaAck = ::handleMediaAck,
                onMediaState = ::updateMediaState,
            )
        } finally {
            // A replacement connection may already be active when an older
            // session finishes. Do not let the old cleanup erase it.
            synchronized(lock) {
                if (activeSession === session) activeSession = null
            }
        }

    }

    fun sendNotificationClear(id: Int) {
        val session = synchronized(lock) { activeSession }
        if (session == null) {
            Log.w(TAG, "cannot clear remote notification $id: no active session")
            return
        }
        try {
            protocol.sendNotificationClear(session, id)
            Log.i(TAG, "sent remote notification clear for $id")
        } catch (e: Exception) {
            Log.e(TAG, "failed to clear remote notification $id", e)
        }
    }

    fun sendAndroidNotification(notification: NotificationTransport.NotificationData) {
        val session = synchronized(lock) { activeSession }
        if (session == null) Log.w(TAG, "cannot forward notification: no active session")
        else try {
            protocol.sendAndroidNotification(session, notification); Log.d(
                TAG,
                "forwarded notification ${notification.key}"
            )
        } catch (e: Exception) {
            Log.e(TAG, "failed to forward notification", e)
        }
    }

    fun sendAndroidNotificationDismiss(key: String) {
        synchronized(lock) { activeSession }?.let { protocol.sendAndroidNotificationDismiss(it, key) }
    }

    fun sendRemoteInput(input: RemoteInput) {
        remoteInputDispatcher.send(input)
    }

    fun sendMediaControl(control: MediaControl) {
        val session = synchronized(lock) { activeSession }
        if (session == null) {
            Log.w(TAG, "cannot control host media: no active session")
            _mediaState.value = HostMediaState(error = "No active connection to the host")
            return
        }

        scope.launch(Dispatchers.IO) {
            try {
                Log.d(TAG, "sending host media command=${control.action}")
                protocol.sendMediaControl(session, control)
                quavaService?.showHostMediaControls()
            } catch (e: Exception) {
                Log.e(TAG, "failed to send host media control", e)
                _mediaState.value = HostMediaState(error = "Could not send media control: ${e.message}")
            }
        }
    }


    private fun updateMediaState(message: PairingProtocol.Message) {
        val previous = _mediaState.value
        val updateType = (message.payload[MULTIMEDIA_UPDATE_TYPE] as? Number)?.toInt()
            ?: MULTIMEDIA_UPDATE_FULL

        when (updateType) {
            MULTIMEDIA_UPDATE_POSITION -> {
                val positionUs = (message.payload[10] as? Number)?.toLong() ?: return
                if (previous.available && previous.playerId.isNotBlank() &&
                    positionUs != previous.positionUs
                ) {
                    _mediaState.value = previous.copy(positionUs = positionUs)
                    quavaService?.updateHostMediaPosition(positionUs)
                }
                return
            }
            MULTIMEDIA_UPDATE_FULL -> Unit
            else -> {
                Log.w(TAG, "Ignoring unknown multimedia update type $updateType")
                return
            }
        }

        fun text(key: Int): String =
            message.payload[key] as? String ?: ""

        fun number(key: Int): Long =
            (message.payload[key] as? Number)?.toLong() ?: 0L

        val hasMediaState = message.payload.containsKey(3)

        val devices = (message.payload[33] as? List<*>)
            ?.mapNotNull { item ->
                val fields = item as? Map<*, *> ?: return@mapNotNull null

                fun field(key: Int): Any? =
                    fields.entries.firstOrNull {
                        (it.key as? Number)?.toInt() == key
                    }?.value

                val name = field(0) as? String
                    ?: return@mapNotNull null

                AudioOutputDevice(
                    name = name,
                    description = field(1) as? String ?: name,
                    volume = (field(2) as? Number)?.toDouble() ?: 0.0,
                    muted = field(3) as? Boolean ?: false,
                    isDefault = field(4) as? Boolean ?: false,
                )
            } ?: previous.outputDevices

        val next = if (hasMediaState) {
            previous.copy(
                available = true,
                error = null,
                playerId = text(2),
                identity = text(3).ifBlank { "Host media" },
                title = text(5).ifBlank { "Unknown title" },
                artists = (message.payload[6] as? List<*>)
                    ?.filterIsInstance<String>()
                    ?.joinToString(", ")
                    ?.ifBlank { "Unknown artist" }
                    ?: "Unknown artist",
                album = text(7),
                artworkUrl = text(8),
                // Clear old artwork when a new media state has no artwork bytes.
                artwork = (message.payload[19] as? ByteArray)
                    ?: if (text(5) == previous.title && text(7) == previous.album) {
                        previous.artwork
                    } else {
                        null
                    },
                playbackStatus = text(4),
                durationUs = number(9),
                positionUs = number(10),
                volume = (message.payload[11] as? Number)?.toDouble()
                    ?: previous.volume,
                shuffle = message.payload[12] as? Boolean ?: previous.shuffle,
                loopStatus = text(13).ifBlank { "None" },
                canGoPrevious = message.payload[14] as? Boolean ?: previous.canGoPrevious,
                canGoNext = message.payload[15] as? Boolean ?: previous.canGoNext,
                canSeek = message.payload[16] as? Boolean ?: previous.canSeek,
                systemVolume = (message.payload[30] as? Number)?.toDouble()
                    ?: previous.systemVolume,
                systemMuted = message.payload[31] as? Boolean
                    ?: previous.systemMuted,
                outputDevice = text(32).ifBlank { previous.outputDevice },
                outputDevices = devices,
            )
        } else {
            HostMediaState(
                error = null,
                systemVolume = (message.payload[30] as? Number)?.toDouble()
                    ?: previous.systemVolume,
                systemMuted = message.payload[31] as? Boolean
                    ?: previous.systemMuted,
                outputDevice = text(32).ifBlank { previous.outputDevice },
                outputDevices = devices,
            )
        }

        if (sameMediaState(previous, next)) return
        _mediaState.value = next

        Log.d(
            "QuavaMedia",
            "Track=${next.title}, status=${next.playbackStatus}, " +
                    "positionUs=${next.positionUs}, durationUs=${next.durationUs}" +
                    "State update: shuffle=${next.shuffle}, loop=${next.loopStatus}"
        )

        if (mediaNotificationChanged(previous, next)) {
            quavaService?.updateHostMediaState(next)
        }
    }

    private fun sameMediaState(left: HostMediaState, right: HostMediaState): Boolean =
        sameArtwork(left.artwork, right.artwork) &&
            left.copy(artwork = null) == right.copy(artwork = null)

    private fun mediaNotificationChanged(left: HostMediaState, right: HostMediaState): Boolean =
        left.available != right.available ||
            left.title != right.title ||
            left.artists != right.artists ||
            !sameArtwork(left.artwork, right.artwork) ||
            left.playbackStatus != right.playbackStatus ||
            left.positionUs != right.positionUs ||
            left.durationUs != right.durationUs

    private fun sameArtwork(left: ByteArray?, right: ByteArray?): Boolean = when {
        left === right -> true
        left == null || right == null -> false
        else -> left.contentEquals(right)
    }

    private fun handleMediaAck(message: PairingProtocol.Message) {
        val success = message.payload[0] as? Boolean ?: false

        if (!success) {
            val error = message.payload[1] as? String ?: "Unknown multimedia error"
            Log.e("Quava", "Media command failed: $error")
            // Surface the failure; the next full state update clears it.
            _mediaState.update { it.copy(error = error) }
        }
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
        private const val MULTIMEDIA_UPDATE_TYPE = 20
        private const val MULTIMEDIA_UPDATE_FULL = 0
        private const val MULTIMEDIA_UPDATE_POSITION = 1
    }
}
