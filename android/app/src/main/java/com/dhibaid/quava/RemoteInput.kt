package com.dhibaid.quava

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.launch

/** One pointer or keyboard event sent to the Linux daemon. */
data class RemoteInput(
    val type: Type,
    val action: Action = Action.NONE,
    val keyCode: Int = 0,
    val button: Int = 0,
    val x: Double = 0.0,
    val y: Double = 0.0,
    val text: String = "",
) {
    enum class Type(val wireValue: Int) {
        KEY(1),
        POINTER_MOVE(2),
        POINTER_BUTTON(3),
        SCROLL(4),
        TEXT(5),
    }

    enum class Action(val wireValue: Int) { NONE(0), DOWN(1), UP(2) }

    fun toPayload(): Map<Int, Any> = buildMap {
        put(0, type.wireValue)
        if (action != Action.NONE) put(1, action.wireValue)
        if (keyCode != 0) put(2, keyCode)
        if (button != 0) put(3, button)
        if (x != 0.0) put(4, x)
        if (y != 0.0) put(5, y)
        if (type == Type.TEXT && text.isNotEmpty()) put(6, text)
    }
}

/** Serializes UI input onto the current authenticated session in gesture order. */
internal class RemoteInputDispatcher(
    scope: CoroutineScope,
    private val protocol: PairingProtocol,
    private val activeSession: () -> PairingProtocol.RuntimeSession?,
) {
    // Keep ordering reliable for key/button down-up pairs. The daemon coalesces
    // pointer movement; don't use a dropping bounded channel here because it
    // could drop a button/key release and leave the host in a held state.
    private val queue = Channel<QueuedInput>(Channel.UNLIMITED)

    init {
        scope.launch(Dispatchers.IO) {
            for ((session, input) in queue) {
                if (activeSession() === session) {
                    runCatching { protocol.sendRemoteInput(session, input) }
                        .onFailure { Log.w(TAG, "failed to send remote input", it) }
                }
            }
        }
    }

    fun send(input: RemoteInput) {
        val session = activeSession() ?: return
        queue.trySend(QueuedInput(session, input))
    }

    private data class QueuedInput(
        val session: PairingProtocol.RuntimeSession,
        val input: RemoteInput,
    )

    private companion object {
        const val TAG = "QuavaRemoteInput"
    }
}
