package com.dhibaid.quava

import android.content.Context
import android.util.Log
import com.jcraft.jsch.ChannelShell
import com.jcraft.jsch.HostKey
import com.jcraft.jsch.HostKeyRepository
import com.jcraft.jsch.JSch
import com.jcraft.jsch.KeyPair
import com.jcraft.jsch.Session
import com.jcraft.jsch.SocketFactory
import com.jcraft.jsch.UserInfo
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.io.OutputStream
import java.net.Socket
import java.security.MessageDigest
import java.util.concurrent.LinkedBlockingQueue

/**
 * Drives an interactive shell over an SSH tunnel carried entirely inside the
 * already-authenticated, encrypted Quava session - there is no direct socket
 * to a host:port here. JSch is pointed at a [SocketFactory] that forwards
 * bytes through [PairingServer.sendSshData]/[PairingServer.setSshListeners]
 * instead of a real network connection, and the client identity is a fresh
 * Ed25519 keypair generated in memory for this one attempt and never
 * persisted - see the design notes on daemon/libquava/ssh.OpenTunnel.
 */
class SshTerminal(
    private val context: Context,
    private val scope: CoroutineScope,
    private val pairingServer: PairingServer,
) {
    private val emulator = TerminalEmulator()
    private val _screen = MutableStateFlow(emulator.snapshot())
    val screen: StateFlow<TerminalEmulator.Snapshot> = _screen.asStateFlow()
    private var session: Session? = null
    private var channel: ChannelShell? = null
    private var input: OutputStream? = null
    private var readerJob: Job? = null
    private var writerJob: Job? = null
    private var outbox: Channel<String>? = null
    private var bridgeInput: TunnelInputStream? = null
    private var lastScreenPublishNanos = 0L

    fun connect() {
        close()
        emulator.reset()
        _screen.value = emulator.snapshot()
        lastScreenPublishNanos = 0L
        readerJob = scope.launch(Dispatchers.IO) {
            try {
                val jsch = JSch()
                val keyPair = KeyPair.genKeyPair(jsch, KeyPair.ED25519)
                val publicKeyBlob = keyPair.publicKeyBlob
                // Ed25519 keys have no legacy PEM encoding - writePrivateKey() throws
                // UnsupportedOperationException for them. The OpenSSH-v1 format is the
                // only one that can represent an EdDSA key, and addIdentity() reads it
                // back the same way it would a key loaded from disk.
                val privateKeyBlob = ByteArrayOutputStream().also { keyPair.writeOpenSSHv1PrivateKey(it, null) }.toByteArray()
                jsch.addIdentity("quava-android", privateKeyBlob, publicKeyBlob, null)
                keyPair.dispose()

                val fingerprint = pairingServer.openSshTunnel(publicKeyBlob).getOrThrow()

                val tunnelInput = TunnelInputStream()
                bridgeInput = tunnelInput
                pairingServer.setSshListeners(
                    onData = tunnelInput::offer,
                    onClosed = { reason ->
                        tunnelInput.signalEof()
                        emulator.feed("\r\nConnection closed${if (reason.isNotBlank()) ": $reason" else ""}\r\n")
                        _screen.value = emulator.snapshot()
                    },
                )

                val nextSession = jsch.getSession("quava", "quava-tunnel", 0)
                nextSession.setSocketFactory(TunnelSocketFactory(tunnelInput, pairingServer::sendSshData))
                nextSession.setConfig("StrictHostKeyChecking", "yes")
                nextSession.hostKeyRepository = PinnedHostKeyRepository(fingerprint)
                nextSession.connect(10_000)

                val nextChannel = nextSession.openChannel("shell") as ChannelShell
                nextChannel.setPty(true)
                nextChannel.setPtyType("xterm", 80, 24, 640, 480)
                nextChannel.connect(5_000)
                session = nextSession
                channel = nextChannel
                input = nextChannel.outputStream

                val mailbox = Channel<String>(Channel.UNLIMITED)
                outbox = mailbox
                // A single writer drains the mailbox strictly in order, so
                // keystrokes, toolbar keys and pastes fired in quick
                // succession (each previously its own independent
                // Dispatchers.IO coroutine) can never interleave or reorder
                // bytes on the wire.
                writerJob = scope.launch(Dispatchers.IO) {
                    for (text in mailbox) {
                        try {
                            input?.write(text.toByteArray())
                            input?.flush()
                        } catch (error: Exception) {
                            emulator.feed("\r\nWrite failed: ${error.message}\r\n")
                            _screen.value = emulator.snapshot()
                        }
                    }
                }

                readOutput(nextChannel.inputStream)
            } catch (error: Exception) {
                Log.e("Quava", "ssh connection failed", error)
                emulator.feed("\r\nConnection failed: ${error.message}\r\n")
                _screen.value = emulator.snapshot()
                close()
            }
        }
    }

    fun send(text: String) {
        outbox?.trySend(text)
    }

    fun resize(columns: Int, rows: Int, widthPixels: Int, heightPixels: Int) {
        emulator.resize(columns, rows)
        _screen.value = emulator.snapshot()
        scope.launch(Dispatchers.IO) {
            // Resizing rides inside the SSH protocol's own "window-change"
            // channel request once the tunnel is open; no separate Quava
            // message is needed for it.
            channel?.setPtySize(columns, rows, widthPixels, heightPixels)
        }
    }

    fun close() {
        readerJob?.cancel()
        readerJob = null
        writerJob?.cancel()
        writerJob = null
        outbox?.close()
        outbox = null
        input = null
        channel?.disconnect()
        channel = null
        session?.disconnect()
        session = null
        bridgeInput?.signalEof()
        bridgeInput = null
        pairingServer.setSshListeners(null, null)
        pairingServer.closeSshTunnel()
    }

    private suspend fun readOutput(stream: InputStream) = withContext(Dispatchers.IO) {
        val reader = stream.reader(Charsets.UTF_8)
        val buffer = CharArray(4096)
        while (true) {
            val count = reader.read(buffer)
            if (count < 0) break
            val text = String(buffer, 0, count)
            emulator.feed(text)
            val now = System.nanoTime()
            if (now - lastScreenPublishNanos >= SCREEN_FRAME_NANOS) {
                _screen.value = emulator.snapshot()
                lastScreenPublishNanos = now
            }
        }
        _screen.value = emulator.snapshot()
    }

    private companion object {
        const val SCREEN_FRAME_NANOS = 33_000_000L
    }
}

/**
 * Feeds JSch's reader thread with bytes that arrive as SSH_DATA frames over
 * the Quava session instead of a real socket. A dedicated end-of-stream
 * sentinel (rather than an empty array) keeps "no more data" unambiguous:
 * the production sender on both platforms only ever forwards non-empty
 * chunks, so an empty [ByteArray] is never a legitimate data frame.
 */
private class TunnelInputStream : InputStream() {
    private sealed class Frame {
        data class Data(val bytes: ByteArray) : Frame()
        object Eof : Frame()
    }

    private val queue = LinkedBlockingQueue<Frame>()
    private var current: ByteArray? = null
    private var pos = 0
    private var eofSeen = false

    fun offer(chunk: ByteArray) {
        if (chunk.isNotEmpty()) queue.put(Frame.Data(chunk))
    }

    fun signalEof() {
        queue.put(Frame.Eof)
    }

    override fun read(): Int {
        val single = ByteArray(1)
        val n = read(single, 0, 1)
        return if (n <= 0) -1 else single[0].toInt() and 0xff
    }

    override fun read(b: ByteArray, off: Int, len: Int): Int {
        if (len == 0) return 0
        if (eofSeen) return -1
        while (current == null || pos >= current!!.size) {
            when (val frame = queue.take()) {
                is Frame.Data -> {
                    current = frame.bytes
                    pos = 0
                }

                Frame.Eof -> {
                    eofSeen = true
                    return -1
                }
            }
        }
        val currentChunk = current!!
        val available = currentChunk.size - pos
        val toCopy = minOf(available, len)
        System.arraycopy(currentChunk, pos, b, off, toCopy)
        pos += toCopy
        return toCopy
    }
}

/**
 * Stands in for a real network connection. JSch calls [createSocket] for a
 * placeholder [Socket] handle (an unconnected one works fine: JSch only
 * calls a handful of harmless methods on it directly - setTcpNoDelay,
 * setSoTimeout, close - and routes every byte of actual I/O through
 * [getInputStream]/[getOutputStream] instead).
 */
private class TunnelSocketFactory(
    private val input: InputStream,
    private val onWrite: (ByteArray) -> Unit,
) : SocketFactory {
    override fun createSocket(host: String?, port: Int): Socket = Socket()

    override fun getInputStream(socket: Socket): InputStream = input

    override fun getOutputStream(socket: Socket): OutputStream = object : OutputStream() {
        override fun write(b: Int) = onWrite(byteArrayOf(b.toByte()))
        override fun write(b: ByteArray, off: Int, len: Int) =
            onWrite(if (off == 0 && len == b.size) b else b.copyOfRange(off, off + len))
    }
}

/**
 * Replaces blind trust-on-first-use with pinning: the only host key this
 * tunnel will ever accept is the exact fingerprint the daemon reported in
 * its SSHOpenAck for this specific authenticated request, never one read
 * from and written back to persistent storage.
 */
private class PinnedHostKeyRepository(private val expectedFingerprint: ByteArray) : HostKeyRepository {
    override fun check(host: String?, key: ByteArray?): Int {
        if (key == null) return HostKeyRepository.NOT_INCLUDED
        val digest = MessageDigest.getInstance("SHA-256").digest(key)
        return if (digest.contentEquals(expectedFingerprint)) {
            HostKeyRepository.OK
        } else {
            HostKeyRepository.CHANGED
        }
    }

    override fun add(hostkey: HostKey?, userinfo: UserInfo?) = Unit
    override fun remove(host: String?, type: String?) = Unit
    override fun remove(host: String?, type: String?, key: ByteArray?) = Unit
    override fun getKnownHostsRepositoryID(): String = "quava-pinned"
    override fun getHostKey(): Array<HostKey> = emptyArray()
    override fun getHostKey(host: String?, type: String?): Array<HostKey> = emptyArray()
}
