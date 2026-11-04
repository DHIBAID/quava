package com.dhibaid.quava

import android.content.Context
import android.util.Base64
import com.jcraft.jsch.ChannelShell
import com.jcraft.jsch.JSch
import com.jcraft.jsch.KeyPair
import com.jcraft.jsch.Session
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

class SshTerminal(private val context: Context, private val scope: CoroutineScope) {
    private val emulator = TerminalEmulator()
    private val _screen = MutableStateFlow(emulator.snapshot())
    val screen: StateFlow<TerminalEmulator.Snapshot> = _screen.asStateFlow()
    private var session: Session? = null
    private var channel: ChannelShell? = null
    private var input: OutputStream? = null
    private var readerJob: Job? = null
    private var writerJob: Job? = null
    private var outbox: Channel<String>? = null
    private var lastScreenPublishNanos = 0L

    fun connect(host: String) {
        close()
        emulator.reset()
        _screen.value = emulator.snapshot()
        lastScreenPublishNanos = 0L
        readerJob = scope.launch(Dispatchers.IO) {
            try {
                val jsch = JSch()
                val key = loadOrGenerateKey()
                jsch.addIdentity("quava-android", key.privateKey, null, null)
                val nextSession = jsch.getSession("quava", host, 2222)
                nextSession.setConfig("StrictHostKeyChecking", "no")
                nextSession.connect(5_000)

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

    private fun loadOrGenerateKey(): StoredKey {
        val prefs = context.getSharedPreferences(KEY_PREFS, Context.MODE_PRIVATE)
        val encoded = prefs.getString(KEY_PRIVATE, null)
        if (encoded != null) {
            return StoredKey(Base64.decode(encoded, Base64.DEFAULT))
        }

        val keyPair = KeyPair.genKeyPair(JSch(), KeyPair.RSA, 2048)
        val privateKey = ByteArrayOutputStream().also { output ->
            keyPair.writePrivateKey(output)
        }.toByteArray()
        keyPair.dispose()
        prefs.edit()
            .putString(KEY_PRIVATE, Base64.encodeToString(privateKey, Base64.DEFAULT))
            .apply()
        return StoredKey(privateKey)
    }

    private data class StoredKey(val privateKey: ByteArray)

    private companion object {
        const val KEY_PREFS = "quava-ssh"
        const val KEY_PRIVATE = "private-key"
        const val SCREEN_FRAME_NANOS = 33_000_000L
    }
}
