package com.dhibaid.quava

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.fail
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.IOException

class SecureStreamsTest {
    private val key = ByteArray(FrameCipher.KEY_BYTES) { 7 }
    private val max = 64 * 1024

    private fun frame(body: ByteArray): ByteArray =
        ByteArrayOutputStream().also { DataOutputStream(it).apply { writeInt(body.size); write(body) } }.toByteArray()

    private fun seal(vararg bodies: ByteArray): ByteArray {
        val wire = ByteArrayOutputStream()
        val output = DataOutputStream(SealingOutputStream(wire, key, max))
        for (body in bodies) {
            output.writeInt(body.size)
            output.write(body)
            output.flush()
        }
        return wire.toByteArray()
    }

    private fun hex(bytes: ByteArray) = bytes.joinToString("") { "%02x".format(it) }

    /** Same values as TestSealedFrameKnownAnswer in the daemon: the wire-format contract. */
    @Test
    fun matchesTheDaemonsSealedFrames() {
        val sevens = ByteArray(FrameCipher.KEY_BYTES) { 7 }
        val message = byteArrayOf(1, 2, 3)

        assertEquals("36862a81bd5e971942124e693d88f61937f0a6", hex(FrameCipher.seal(sevens, 0, message)))
        assertEquals("e81550a050873d57dd31500485eb76175c56db", hex(FrameCipher.seal(sevens, 1, message)))
    }

    /** One wire record is [length][sealed message]; the message has no length of its own. */
    @Test
    fun recordIsLengthThenSealedMessage() {
        val wire = seal(byteArrayOf(1, 2, 3))
        val sealedLength = ((wire[0].toInt() and 0xff) shl 24) or ((wire[1].toInt() and 0xff) shl 16) or
                ((wire[2].toInt() and 0xff) shl 8) or (wire[3].toInt() and 0xff)

        assertEquals(3 + FrameCipher.TAG_BYTES, sealedLength)
        assertEquals(4 + sealedLength, wire.size)
        assertArrayEquals(FrameCipher.seal(key, 0, byteArrayOf(1, 2, 3)), wire.copyOfRange(4, wire.size))
    }

    @Test
    fun framesRoundTripInOrder() {
        val bodies = listOf(byteArrayOf(1, 2, 3), ByteArray(5000) { it.toByte() }, byteArrayOf(9))
        val wire = seal(*bodies.toTypedArray())
        val input = DataInputStream(SealedInputStream(ByteArrayInputStream(wire), key, max))

        for (expected in bodies) {
            assertEquals(expected.size, input.readInt())
            val body = ByteArray(expected.size)
            input.readFully(body)
            assertArrayEquals(expected, body)
        }
    }

    @Test
    fun wireDoesNotContainPlaintext() {
        val secret = "clipboard-secret".encodeToByteArray()
        val wire = seal(secret)
        assertFalse(String(wire, Charsets.ISO_8859_1).contains("clipboard-secret"))
    }

    @Test
    fun tamperedFrameIsRejected() {
        val wire = seal(byteArrayOf(1, 2, 3))
        wire[wire.size - 1] = (wire[wire.size - 1].toInt() xor 1).toByte()

        try {
            DataInputStream(SealedInputStream(ByteArrayInputStream(wire), key, max)).readInt()
            fail("a tampered frame was accepted")
        } catch (_: IOException) {
        }
    }

    @Test
    fun replayedFrameIsRejected() {
        val one = seal(byteArrayOf(1))
        val input = DataInputStream(SealedInputStream(ByteArrayInputStream(one + one), key, max))

        input.readInt()
        input.readFully(ByteArray(1))
        try {
            input.readInt()
            fail("a replayed frame was accepted")
        } catch (_: IOException) {
        }
    }
}
