package com.dhibaid.quava

import org.bouncycastle.crypto.InvalidCipherTextException
import org.bouncycastle.crypto.modes.ChaCha20Poly1305
import org.bouncycastle.crypto.params.AEADParameters
import org.bouncycastle.crypto.params.KeyParameter
import java.io.ByteArrayOutputStream
import java.io.EOFException
import java.io.IOException
import java.io.InputStream
import java.io.OutputStream
import java.nio.ByteBuffer

/**
 * Frame encryption for authenticated sessions. It must match the daemon's
 * `protocol/secure.go` exactly:
 *
 *  - ChaCha20-Poly1305, one key per direction.
 *  - Nonce: 4 zero bytes then the big-endian frame counter (starts at 0).
 *  - Associated data: "quava-frame-v1".
 *  - On the wire each frame is `[4-byte length of what follows][ciphertext + 16-byte tag]`.
 *  - The encrypted part is the CBOR message alone, with no length prefix of its
 *    own; the outer length is the only one. (Callers still see plain
 *    `[length][CBOR]` frames: the wrappers below add and remove the prefix.)
 *
 * Counters make replayed, reordered or reflected frames fail authentication.
 */
internal object FrameCipher {
    const val KEY_BYTES = 32
    const val TAG_BYTES = 16
    private val ASSOCIATED_DATA = "quava-frame-v1".encodeToByteArray()

    private fun nonce(counter: Long): ByteArray =
        ByteArray(12).also { ByteBuffer.wrap(it).putLong(4, counter) }

    private fun cipher(key: ByteArray, counter: Long, encrypt: Boolean) =
        ChaCha20Poly1305().apply {
            init(encrypt, AEADParameters(KeyParameter(key), TAG_BYTES * 8, nonce(counter), ASSOCIATED_DATA))
        }

    fun seal(key: ByteArray, counter: Long, plaintext: ByteArray): ByteArray {
        val cipher = cipher(key, counter, true)
        val out = ByteArray(cipher.getOutputSize(plaintext.size))
        var length = cipher.processBytes(plaintext, 0, plaintext.size, out, 0)
        length += cipher.doFinal(out, length)
        return out.copyOf(length)
    }

    /** Throws [IOException] if the frame does not authenticate. */
    fun open(key: ByteArray, counter: Long, sealed: ByteArray): ByteArray {
        val cipher = cipher(key, counter, false)
        val out = ByteArray(cipher.getOutputSize(sealed.size))
        return try {
            var length = cipher.processBytes(sealed, 0, sealed.size, out, 0)
            length += cipher.doFinal(out, length)
            out.copyOf(length)
        } catch (e: InvalidCipherTextException) {
            throw IOException("frame failed authentication (is the other side up to date?)", e)
        }
    }
}

/**
 * Wraps the socket's output. Callers keep writing the plain `[length][body]`
 * frames they always did; each complete frame leaves as one sealed record.
 * Writers are already serialized by the caller, and this is synchronized too
 * because counter order must equal wire order.
 */
internal class SealingOutputStream(
    private val out: OutputStream,
    private val key: ByteArray,
    private val maxFrame: Int,
) : OutputStream() {
    private val pending = ByteArrayOutputStream()
    private var counter = 0L

    @Synchronized
    override fun write(b: Int) {
        pending.write(b)
        emitCompleteFrames()
    }

    @Synchronized
    override fun write(b: ByteArray, off: Int, len: Int) {
        pending.write(b, off, len)
        emitCompleteFrames()
    }

    @Synchronized
    override fun flush() {
        emitCompleteFrames()
        out.flush()
    }

    private fun emitCompleteFrames() {
        val data = pending.toByteArray()
        var position = 0

        while (data.size - position >= 4) {
            val length = ByteBuffer.wrap(data, position, 4).int
            if (length !in 1..maxFrame) throw IOException("invalid frame length $length")
            if (data.size - position < 4 + length) break

            // Seal only the message; the record's own length prefix replaces the caller's.
            val record = FrameCipher.seal(key, counter++, data.copyOfRange(position + 4, position + 4 + length))
            out.write(ByteBuffer.allocate(4).putInt(record.size).array())
            out.write(record)
            position += 4 + length
        }

        pending.reset()
        if (position < data.size) pending.write(data, position, data.size - position)
    }
}

/** Unwraps sealed records back into the plain `[length][body]` byte stream. */
internal class SealedInputStream(
    private val input: InputStream,
    private val key: ByteArray,
    private val maxFrame: Int,
) : InputStream() {
    private var buffer = ByteArray(0)
    private var position = 0
    private var counter = 0L

    override fun read(): Int {
        if (!fill()) return -1
        return buffer[position++].toInt() and 0xff
    }

    override fun read(b: ByteArray, off: Int, len: Int): Int {
        if (len == 0) return 0
        if (!fill()) return -1

        val count = minOf(len, buffer.size - position)
        System.arraycopy(buffer, position, b, off, count)
        position += count
        return count
    }

    /** Loads the next record when the current one is used up. False on a clean end of stream. */
    private fun fill(): Boolean {
        while (position >= buffer.size) {
            val header = ByteArray(4)
            val first = input.read(header, 0, 4)
            if (first < 0) return false
            readFully(header, first)

            val size = ByteBuffer.wrap(header).int
            if (size !in (FrameCipher.TAG_BYTES + 1)..(maxFrame + FrameCipher.TAG_BYTES)) {
                throw IOException("invalid sealed frame length $size")
            }

            val sealed = ByteArray(size)
            readFully(sealed, 0)

            // Hand the reader the plain `[length][CBOR]` frame it expects.
            val message = FrameCipher.open(key, counter++, sealed)
            buffer = ByteBuffer.allocate(4 + message.size).putInt(message.size).put(message).array()
            position = 0
        }
        return true
    }

    private fun readFully(target: ByteArray, already: Int) {
        var read = already
        while (read < target.size) {
            val n = input.read(target, read, target.size - read)
            if (n < 0) throw EOFException("connection closed mid-frame")
            read += n
        }
    }
}
