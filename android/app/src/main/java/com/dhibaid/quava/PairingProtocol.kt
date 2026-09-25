package com.dhibaid.quava

import android.content.Context
import android.util.Base64
import android.util.Log
import com.upokecenter.cbor.CBORObject
import com.upokecenter.cbor.CBORType
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import org.bouncycastle.jce.provider.BouncyCastleProvider
import java.io.DataInputStream
import java.io.DataOutputStream
import java.net.Socket
import java.security.KeyFactory
import java.security.KeyPair
import java.security.KeyPairGenerator
import java.security.MessageDigest
import java.security.PrivateKey
import java.security.PublicKey
import java.security.SecureRandom
import java.security.Signature
import java.security.Provider
import java.security.spec.PKCS8EncodedKeySpec
import java.security.spec.X509EncodedKeySpec
import javax.crypto.KeyAgreement
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/** Wire-compatible responder for daemon/libquava's pairing.Initiate. */
class PairingProtocol(private val appContext: Context) {
    private val random = SecureRandom()
    private val provider: Provider = BouncyCastleProvider()
    private val identity: Identity

    init {
        // Android ships an older provider named "BC". Passing our bundled
        // provider directly avoids accidentally selecting that incomplete one.
        identity = IdentityStore(appContext, provider).loadOrCreate()
    }

    class BusyException : Exception()

    class Session internal constructor(
        internal val socket: Socket,
        internal val input: DataInputStream,
        internal val output: DataOutputStream,
        internal val request: Message,
        internal val initiatorDeviceId: ByteArray,
        internal val initiatorPublicKey: ByteArray,
        internal val initiatorNonce: ByteArray,
        internal val responderNonce: ByteArray,
        internal val responderEphemeral: KeyPair,
        internal val responderDeviceId: ByteArray,
        val peerName: String,
    ) {
        private val userConfirmation = CompletableDeferred<Boolean>()

        fun confirm(accepted: Boolean) {
            userConfirmation.complete(accepted)
        }

        internal fun awaitConfirmation(): Boolean = runBlocking { userConfirmation.await() }

        fun close() = try {
            socket.close()
        } catch (_: Exception) {
        }
    }

    internal fun readInitial(socket: Socket): Message {
        socket.soTimeout = PAIRING_TIMEOUT_MS
        return readMessage(DataInputStream(socket.getInputStream()))
    }

    /** Reads PAIR_REQUEST and sends PAIR_CHALLENGE before asking the user. */
    fun begin(socket: Socket, onChallenge: (String, Int, Session) -> Unit): Session =
        begin(socket, readInitial(socket), onChallenge)

    internal fun begin(socket: Socket, request: Message, onChallenge: (String, Int, Session) -> Unit): Session {
        socket.soTimeout = PAIRING_TIMEOUT_MS
        val input = DataInputStream(socket.getInputStream())
        val output = DataOutputStream(socket.getOutputStream())
        requireMessage(request, PAIR_REQUEST)

        val initiatorDeviceId = request.bytes(0, DEVICE_ID_BYTES)
        val initiatorPublicKey = request.bytes(1, ED25519_PUBLIC_KEY_BYTES)
        val initiatorNonce = request.bytes(2, NONCE_BYTES)
        require(request.uints(3).contains(PROTOCOL_VERSION)) { "peer does not support protocol version 1" }
        require(initiatorDeviceId.contentEquals(deviceId(initiatorPublicKey))) { "initiator device ID does not match public key" }
        val peerName = request.string(4).ifBlank { socket.inetAddress.hostAddress }

        val responderNonce = randomBytes(NONCE_BYTES)
        val responderEphemeral = x25519KeyPair()
        val responderDeviceId = deviceId(identity.publicRaw)
        val session = Session(
            socket, input, output, request, initiatorDeviceId, initiatorPublicKey,
            initiatorNonce, responderNonce, responderEphemeral, responderDeviceId, peerName,
        )
        val code = verificationCode(
            initiatorPublicKey, identity.publicRaw, initiatorNonce, responderNonce, request.transactionId,
        )
        writeMessage(
            output, Message(
                PAIR_CHALLENGE,
                request.transactionId,
                mapOf(
                    0 to responderDeviceId,
                    1 to identity.publicRaw,
                    2 to responderNonce,
                    3 to PROTOCOL_VERSION,
                    4 to rawPublic(responderEphemeral.public),
                    5 to code.toLong(),
                ),
            )
        )
        onChallenge(peerName, code, session)
        return session
    }

    /** Authenticates and completes an approved pairing transaction. */
    fun complete(session: Session) {
        val authenticate = readMessage(session.input)
        requireMatchingMessage(authenticate, PAIR_AUTHENTICATE, session.request.transactionId)
        val initiatorEphemeral = authenticate.bytes(0, X25519_KEY_BYTES)
        val initiatorSignature = authenticate.bytes(1, ED25519_SIGNATURE_BYTES)
        val transcript = transcript(
            session.request.transactionId,
            session.initiatorDeviceId,
            session.responderDeviceId,
            session.initiatorPublicKey,
            identity.publicRaw,
            session.initiatorNonce,
            session.responderNonce,
            initiatorEphemeral,
            rawPublic(session.responderEphemeral.public),
        )
        require(
            verify(
                session.initiatorPublicKey,
                transcript,
                initiatorSignature
            )
        ) { "initiator signature verification failed" }

        val confirm = readMessage(session.input)
        requireMatchingMessage(confirm, PAIR_CONFIRM, session.request.transactionId)
        require(confirm.bool(1)) { "initiator did not confirm pairing" }
        require(session.awaitConfirmation()) { "pairing rejected by user" }

        val sharedSecret = x25519(session.responderEphemeral.private, initiatorEphemeral)
        val masterSecret = hkdfSha256(
            sha256(session.initiatorNonce + session.responderNonce),
            sharedSecret,
            "quava-pairing-v1".encodeToByteArray(),
            32,
        )
        val confirmationMac = hmacSha256(
            masterSecret,
            "quava-pair-complete".encodeToByteArray() + session.request.transactionId,
        )
        writeMessage(
            session.output, Message(
                PAIR_COMPLETE,
                session.request.transactionId,
                mapOf(
                    0 to session.responderDeviceId,
                    1 to confirmationMac,
                    2 to sign(transcript),
                ),
            )
        )
        val peerCredential = hkdfSha256(
            sha256(session.initiatorNonce + session.responderNonce),
            sharedSecret,
            "quava-pairing-v1".encodeToByteArray(),
            32,
        ).let { master ->
            hkdfSha256(
                sha256(session.request.transactionId),
                master,
                "quava-peer-credential-v1".encodeToByteArray(),
                32
            )
        }
        IdentityStore(appContext, provider).persistPeer(
            session.initiatorDeviceId,
            session.initiatorPublicKey,
            peerCredential
        )
    }

    internal data class RuntimeSession(
        internal val socket: Socket,
        internal val input: DataInputStream,
        internal val output: DataOutputStream,
        internal val transactionId: ByteArray,
        internal val peerDeviceId: ByteArray,
        internal val peerName: String,
    ) {
        fun close() = runCatching { socket.close() }
    }

    internal fun beginSession(socket: Socket, hello: Message): RuntimeSession {
        requireMatchingMessage(hello, SESSION_HELLO, hello.transactionId)
        val peerId = hello.bytes(0, DEVICE_ID_BYTES)
        val peerPublic = hello.bytes(1, ED25519_PUBLIC_KEY_BYTES)
        val peerNonce = hello.bytes(2, NONCE_BYTES)
        require(peerId.contentEquals(deviceId(peerPublic))) { "session peer device ID does not match public key" }
        val trusted = IdentityStore(appContext, provider).loadPeer(peerId) ?: error("unknown peer; pairing required")
        require(trusted.publicKey.contentEquals(peerPublic)) { "session peer identity conflict" }
        val localId = deviceId(identity.publicRaw)
        val localNonce = randomBytes(NONCE_BYTES)
        val transcript = sessionTranscript(
            hello.transactionId,
            peerId,
            localId,
            peerPublic,
            identity.publicRaw,
            peerNonce,
            localNonce
        )
        val output = DataOutputStream(socket.getOutputStream())
        val input = DataInputStream(socket.getInputStream())
        writeMessage(
            output, Message(
                SESSION_CHALLENGE, hello.transactionId, mapOf(
                    0 to localId,
                    1 to identity.publicRaw,
                    2 to localNonce,
                    3 to hmacSha256(trusted.credential, "challenge".encodeToByteArray() + transcript)
                )
            )
        )
        val auth = readMessage(input)
        requireMatchingMessage(auth, SESSION_AUTHENTICATE, hello.transactionId)
        val authMac = auth.bytes(0, HMAC_BYTES)
        require(
            authMac.contentEquals(
                hmacSha256(
                    trusted.credential,
                    "authenticate".encodeToByteArray() + transcript
                )
            )
        ) { "session authentication failed" }
        writeMessage(
            output, Message(
                SESSION_READY, hello.transactionId, mapOf(
                    0 to hmacSha256(trusted.credential, "ready".encodeToByteArray() + transcript)
                )
            )
        )
        val peerName = socket.inetAddress.hostAddress ?: "Quava peer"
        return RuntimeSession(socket, input, output, hello.transactionId, peerId, peerName)
    }

    internal fun runSession(
        session: RuntimeSession,
        onReady: (String) -> Unit,
    ) {
        while (!session.socket.isClosed) {
            Log.d("Quava", "Waiting for session message...")

            val message = readMessage(session.input)

            Log.d(
                "Quava",
                "Received session message type=${message.type}, payload=${message.payload}"
            )

            when (message.type) {
                SESSION_READY -> {
                    val hostname = message.string(1)

                    Log.d(
                        "Quava",
                        "Linux SESSION_READY: hostname=$hostname"
                    )

                    onReady(hostname)
                }

                PING -> {
                    Log.d("Quava", "Received PING")

                    writeMessage(
                        session.output,
                        Message(
                            PONG,
                            message.transactionId,
                            emptyMap(),
                        ),
                    )
                }

                else -> {
                    error("unexpected session message ${message.type}")
                }
            }
        }
    }


    private fun sessionTranscript(
        txId: ByteArray,
        initiatorId: ByteArray,
        responderId: ByteArray,
        initiatorPublic: ByteArray,
        responderPublic: ByteArray,
        initiatorNonce: ByteArray,
        responderNonce: ByteArray
    ): ByteArray =
        "quava-session".encodeToByteArray() + txId + initiatorId + responderId + initiatorPublic + responderPublic + initiatorNonce + responderNonce

    private data class Identity(val privateKey: PrivateKey, val publicRaw: ByteArray)

    private class IdentityStore(private val context: Context, private val provider: Provider) {
        private val prefs = context.getSharedPreferences("quava-pairing", Context.MODE_PRIVATE)

        fun loadOrCreate(): Identity {
            val storedPrivate = prefs.getString("ed25519-private", null)
            val storedPublic = prefs.getString("ed25519-public", null)
            if (storedPrivate != null && storedPublic != null) {
                val privateKey = KeyFactory.getInstance("Ed25519", provider).generatePrivate(
                    PKCS8EncodedKeySpec(Base64.decode(storedPrivate, Base64.NO_WRAP)),
                )
                return Identity(privateKey, Base64.decode(storedPublic, Base64.NO_WRAP))
            }
            val pair = KeyPairGenerator.getInstance("Ed25519", provider).generateKeyPair()
            val publicRaw = pair.public.encoded.takeLast(32).toByteArray()
            check(
                prefs.edit()
                    .putString("ed25519-private", Base64.encodeToString(pair.private.encoded, Base64.NO_WRAP))
                    .putString("ed25519-public", Base64.encodeToString(publicRaw, Base64.NO_WRAP))
                    .commit()
            ) { "could not persist Android identity" }
            return Identity(pair.private, publicRaw)
        }

        data class TrustedPeer(val publicKey: ByteArray, val credential: ByteArray)

        fun persistPeer(deviceId: ByteArray, publicKey: ByteArray, credential: ByteArray) {
            val id = Base64.encodeToString(deviceId, Base64.NO_WRAP)
            check(
                prefs.edit()
                    .putString("peer-$id", Base64.encodeToString(publicKey, Base64.NO_WRAP))
                    .putString("peer-credential-$id", Base64.encodeToString(credential, Base64.NO_WRAP))
                    .commit()
            ) { "could not persist trusted peer" }
        }

        fun loadPeer(deviceId: ByteArray): TrustedPeer? {
            val id = Base64.encodeToString(deviceId, Base64.NO_WRAP)
            val publicKey = prefs.getString("peer-$id", null) ?: return null
            val credential = prefs.getString("peer-credential-$id", null) ?: return null
            return TrustedPeer(Base64.decode(publicKey, Base64.NO_WRAP), Base64.decode(credential, Base64.NO_WRAP))
        }
    }

    internal data class Message(
        val type: Long,
        val transactionId: ByteArray,
        val payload: Map<Int, Any?>,
        val version: Long = PROTOCOL_VERSION
    )

    private fun readMessage(input: DataInputStream): Message {
        val size = input.readInt()
        require(size in 1..MAX_FRAME_BYTES) { "invalid frame length $size" }
        val frame = ByteArray(size)
        input.readFully(frame)
        val map = CBORObject.DecodeFromBytes(frame)
        require(map.type == CBORType.Map) { "message is not a CBOR map" }
        val version = map[CBORObject.FromObject(0)].AsInt64Value()
        require(version == PROTOCOL_VERSION) { "unsupported protocol version $version" }
        val type = map[CBORObject.FromObject(1)].AsInt64Value()
        val transactionId = map[CBORObject.FromObject(2)].GetByteString()
        require(transactionId.size == TRANSACTION_ID_BYTES) { "invalid transaction ID" }
        val payloadObject = map[CBORObject.FromObject(3)]
        require(payloadObject.type == CBORType.Map) { "message payload is not a CBOR map" }
        val payload = mutableMapOf<Int, Any?>()
        for (key in payloadObject.keys) {
            payload[key.AsInt32Value()] = payloadObject[key].toValue()
        }
        return Message(type, transactionId, payload, version)
    }

    private fun writeMessage(output: DataOutputStream, message: Message) {
        val payload = CBORObject.NewMap()
        message.payload.forEach { (key, value) ->
            payload[CBORObject.FromObject(key)] = CBORObject.FromObject(value)
        }
        val encoded = CBORObject.NewMap().apply {
            this[CBORObject.FromObject(0)] = CBORObject.FromObject(PROTOCOL_VERSION)
            this[CBORObject.FromObject(1)] = CBORObject.FromObject(message.type)
            this[CBORObject.FromObject(2)] = CBORObject.FromObject(message.transactionId)
            this[CBORObject.FromObject(3)] = payload
        }.EncodeToBytes()
        require(encoded.size <= MAX_FRAME_BYTES) { "response frame is too large" }
        output.writeInt(encoded.size)
        output.write(encoded)
        output.flush()
    }

    private fun CBORObject.toValue(): Any? = when (type) {
        CBORType.ByteString -> GetByteString()
        CBORType.TextString -> AsString()
        CBORType.Boolean -> AsBoolean()
        CBORType.Array -> values.map { it.AsInt64Value() }
        // Go's append([]uint64(nil), empty...) produces a nil slice, which
        // fxamacker/cbor correctly encodes as CBOR null. Capabilities are
        // informational, but the null still has to be decoded safely.
        CBORType.SimpleValue -> if (isNull) null else error("unsupported CBOR simple value")
        else -> AsInt64Value()
    }

    private fun Message.bytes(key: Int, size: Int): ByteArray = (payload[key] as? ByteArray)?.also {
        require(it.size == size) { "field $key must be $size bytes" }
    } ?: error("field $key must be bytes")

    private fun Message.string(key: Int): String = payload[key] as? String ?: error("field $key must be text")

    @Suppress("UNCHECKED_CAST")
    private fun Message.uints(key: Int): List<Long> =
        payload[key] as? List<Long> ?: error("field $key must be an integer array")

    private fun Message.bool(key: Int): Boolean = payload[key] as? Boolean ?: error("field $key must be boolean")

    private fun requireMessage(message: Message, type: Long) =
        require(message.type == type) { "unexpected message type ${message.type}" }

    private fun requireMatchingMessage(message: Message, type: Long, transactionId: ByteArray) {
        requireMessage(message, type)
        require(message.transactionId.contentEquals(transactionId)) { "transaction ID mismatch" }
    }

    private fun verificationCode(
        initiatorPublic: ByteArray,
        responderPublic: ByteArray,
        initiatorNonce: ByteArray,
        responderNonce: ByteArray,
        transactionId: ByteArray
    ): Int {
        val digest =
            sha256("quava-pairing-code".encodeToByteArray() + initiatorPublic + responderPublic + initiatorNonce + responderNonce + transactionId)
        val number = ((digest[0].toLong() and 0xff) shl 24) or
                ((digest[1].toLong() and 0xff) shl 16) or
                ((digest[2].toLong() and 0xff) shl 8) or
                (digest[3].toLong() and 0xff)
        return (number % 1_000_000).toInt()
    }

    private fun transcript(
        transactionId: ByteArray,
        initiatorDeviceId: ByteArray,
        responderDeviceId: ByteArray,
        initiatorPublic: ByteArray,
        responderPublic: ByteArray,
        initiatorNonce: ByteArray,
        responderNonce: ByteArray,
        initiatorEphemeral: ByteArray,
        responderEphemeral: ByteArray
    ): ByteArray =
        "quava-pairing".encodeToByteArray() + byteArrayOf(
            0,
            0,
            0,
            0,
            0,
            0,
            0,
            1
        ) + transactionId + initiatorDeviceId + responderDeviceId + initiatorPublic + responderPublic + initiatorNonce + responderNonce + initiatorEphemeral + responderEphemeral

    private fun sign(transcript: ByteArray): ByteArray = Signature.getInstance("Ed25519", provider).run {
        initSign(identity.privateKey)
        update(sha256(transcript))
        sign()
    }

    private fun verify(publicRaw: ByteArray, transcript: ByteArray, signature: ByteArray): Boolean =
        Signature.getInstance("Ed25519", provider).run {
            initVerify(ed25519Public(publicRaw))
            update(sha256(transcript))
            verify(signature)
        }

    private fun x25519KeyPair(): KeyPair = KeyPairGenerator.getInstance("X25519", provider).generateKeyPair()
    private fun x25519(privateKey: PrivateKey, peerRaw: ByteArray): ByteArray =
        KeyAgreement.getInstance("X25519", provider).run {
            init(privateKey)
            doPhase(x25519Public(peerRaw), true)
            generateSecret()
        }

    private fun ed25519Public(raw: ByteArray): PublicKey =
        KeyFactory.getInstance("Ed25519", provider).generatePublic(X509EncodedKeySpec(ED25519_PREFIX + raw))

    private fun x25519Public(raw: ByteArray): PublicKey =
        KeyFactory.getInstance("X25519", provider).generatePublic(X509EncodedKeySpec(X25519_PREFIX + raw))

    private fun rawPublic(key: PublicKey): ByteArray = key.encoded.takeLast(32).toByteArray()
    private fun deviceId(publicKey: ByteArray): ByteArray = sha256(publicKey).copyOf(DEVICE_ID_BYTES)
    private fun randomBytes(size: Int): ByteArray = ByteArray(size).also(random::nextBytes)
    private fun sha256(input: ByteArray): ByteArray = MessageDigest.getInstance("SHA-256").digest(input)
    private fun hmacSha256(key: ByteArray, input: ByteArray): ByteArray = Mac.getInstance("HmacSHA256").run {
        init(SecretKeySpec(key, "HmacSHA256"))
        doFinal(input)
    }

    private fun hkdfSha256(salt: ByteArray, ikm: ByteArray, info: ByteArray, length: Int): ByteArray {
        val prk = hmacSha256(salt, ikm)
        var previous = ByteArray(0)
        val output = ArrayList<Byte>()
        var counter = 1
        while (output.size < length) {
            previous = hmacSha256(prk, previous + info + byteArrayOf(counter.toByte()))
            output += previous.toList()
            counter++
        }
        return output.take(length).toByteArray()
    }

    companion object {
        private const val PROTOCOL_VERSION = 1L
        internal const val PAIR_REQUEST = 0x01L
        private const val PAIR_CHALLENGE = 0x02L
        private const val PAIR_AUTHENTICATE = 0x03L
        private const val PAIR_CONFIRM = 0x04L
        private const val PAIR_COMPLETE = 0x05L
        internal const val SESSION_HELLO = 0x10L
        private const val SESSION_CHALLENGE = 0x11L
        private const val SESSION_AUTHENTICATE = 0x12L
        private const val SESSION_READY = 0x13L
        private const val PING = 0x20L
        private const val PONG = 0x21L
        private const val MAX_FRAME_BYTES = 64 * 1024
        private const val PAIRING_TIMEOUT_MS = 120_000
        private const val TRANSACTION_ID_BYTES = 16
        private const val DEVICE_ID_BYTES = 16
        private const val NONCE_BYTES = 32
        private const val ED25519_PUBLIC_KEY_BYTES = 32
        private const val ED25519_SIGNATURE_BYTES = 64
        private const val X25519_KEY_BYTES = 32
        private const val HMAC_BYTES = 32
        private val ED25519_PREFIX = byteArrayOf(0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00)
        private val X25519_PREFIX = byteArrayOf(0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x6e, 0x03, 0x21, 0x00)
    }
}
