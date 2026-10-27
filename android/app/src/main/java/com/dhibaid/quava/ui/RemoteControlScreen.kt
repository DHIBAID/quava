package com.dhibaid.quava.ui

import android.view.KeyEvent
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
// import androidx.compose.foundation.layout.weight
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Keyboard
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.positionChange
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.text.BasicTextField
import com.dhibaid.quava.PairingServer
import com.dhibaid.quava.RemoteInput
import kotlin.math.abs
import android.util.Log


@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RemoteControlScreen(server: PairingServer, onBack: () -> Unit) {
    val keyboard = LocalSoftwareKeyboardController.current
    val focusRequester = remember { FocusRequester() }
    var editorValue by remember { mutableStateOf(editorValue()) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Remote Input") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                actions = {
                    IconButton(onClick = {
                        focusRequester.requestFocus()
                        keyboard?.show()
                    }) {
                        Icon(Icons.Filled.Keyboard, contentDescription = "Show keyboard")
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            Box(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .background(
                        color = MaterialTheme.colorScheme.surfaceContainerLow,
                        shape = RoundedCornerShape(20.dp),
                    )
                    .pointerInput(server) {
                        awaitEachGesture {
                            val firstDown = awaitFirstDown(requireUnconsumed = false)
                            var moved = false
                            var scrolling = false

                            do {
                                val event = awaitPointerEvent()
                                val pressed = event.changes.filter { it.pressed }

                                if (pressed.size >= 2) {
                                    val dx = pressed.sumOf {
                                        it.positionChange().x.toDouble()
                                    } / pressed.size
                                    val dy = pressed.sumOf {
                                        it.positionChange().y.toDouble()
                                    } / pressed.size

                                    if (abs(dx) >= 1.0 || abs(dy) >= 1.0) {
                                        scrolling = true
                                        server.sendRemoteInput(
                                            RemoteInput(
                                                RemoteInput.Type.SCROLL,
                                                x = dx / 24,
                                                y = dy / 24,
                                            )
                                        )
                                    }
                                } else {
                                    event.changes.firstOrNull { it.id == firstDown.id }?.let { change ->
                                        val delta = change.positionChange()
                                        if (delta.x != 0f || delta.y != 0f) {
                                            moved = true
                                            server.sendRemoteInput(
                                                RemoteInput(
                                                    RemoteInput.Type.POINTER_MOVE,
                                                    x = delta.x.toDouble(),
                                                    y = delta.y.toDouble(),
                                                )
                                            )
                                        }
                                    }
                                }
                            } while (event.changes.any { it.pressed })

                            // Single-finger tap = left click.
                            if (!moved && !scrolling) clickMouseButton(server, BUTTON_LEFT)
                        }
                    },
            ) {
                // IME text is sent as text payloads, not synthesized letter
                // keycodes. Keep active composition in the editor until commit.
                BasicTextField(
                    value = editorValue,
                    onValueChange = { updated ->
                        Log.d(
                            "QuavaRemoteInput",
                            "editor update: text=${updated.text}, composition=${updated.composition}"
                        )
                        if (updated.composition != null) {
                            editorValue = updated
                        } else {
                            when {
                                updated.text.isEmpty() -> {
                                    sendKeyTap(server, KeyEvent.KEYCODE_DEL)
                                }

                                updated.text.startsWith(EDITOR_SENTINEL) -> {
                                    sendImeText(server, updated.text.removePrefix(EDITOR_SENTINEL))
                                }

                                else -> {
                                    // Some IMEs replace the sentinel instead of
                                    // appending after it.
                                    sendImeText(server, updated.text)
                                }
                            }
                            editorValue = editorValue()
                        }
                    },
                    modifier = Modifier
                        .size(1.dp)
                        .alpha(0f)
                        .focusRequester(focusRequester)
                        .onPreviewKeyEvent { event ->
                            val action = when (event.type) {
                                KeyEventType.KeyDown -> RemoteInput.Action.DOWN
                                KeyEventType.KeyUp -> RemoteInput.Action.UP
                                else -> return@onPreviewKeyEvent false
                            }
                            val keyCode = event.nativeKeyEvent.keyCode
                            if (!isSupportedSpecialKey(keyCode)) {
                                return@onPreviewKeyEvent false
                            }
                            server.sendRemoteInput(
                                RemoteInput(
                                    RemoteInput.Type.KEY,
                                    action,
                                    keyCode = keyCode,
                                )
                            )
                            true
                        },
                )
            }

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Button(
                    modifier = Modifier.weight(1f),
                    onClick = { clickMouseButton(server, BUTTON_LEFT) },
                ) { Text("LMB") }

                Button(
                    modifier = Modifier.weight(1f),
                    onClick = { clickMouseButton(server, BUTTON_MIDDLE) },
                ) { Text("MMB") }

                Button(
                    modifier = Modifier.weight(1f),
                    onClick = { clickMouseButton(server, BUTTON_RIGHT) },
                ) { Text("RMB") }
            }
        }
    }
}

private const val EDITOR_SENTINEL = "\u200B"
private const val MAX_TEXT_BYTES = 4096
private const val BUTTON_LEFT = 1
private const val BUTTON_RIGHT = 2
private const val BUTTON_MIDDLE = 3

private fun editorValue() = TextFieldValue(
    text = EDITOR_SENTINEL,
    selection = TextRange(EDITOR_SENTINEL.length),
)

private fun isSupportedSpecialKey(keyCode: Int): Boolean = when (keyCode) {
    KeyEvent.KEYCODE_DPAD_UP,
    KeyEvent.KEYCODE_DPAD_DOWN,
    KeyEvent.KEYCODE_DPAD_LEFT,
    KeyEvent.KEYCODE_DPAD_RIGHT,
    KeyEvent.KEYCODE_SPACE,
    KeyEvent.KEYCODE_ENTER,
    KeyEvent.KEYCODE_DEL,
    KeyEvent.KEYCODE_ESCAPE,
    KeyEvent.KEYCODE_FORWARD_DEL -> true

    // KEYCODE_TAB is currently accepted by the daemon model, but its daemon
    // key map must be corrected as described in the accompanying README.
    KeyEvent.KEYCODE_TAB -> true
    else -> false
}

private fun clickMouseButton(server: PairingServer, button: Int) {
    server.sendRemoteInput(
        RemoteInput(RemoteInput.Type.POINTER_BUTTON, RemoteInput.Action.DOWN, button = button)
    )
    server.sendRemoteInput(
        RemoteInput(RemoteInput.Type.POINTER_BUTTON, RemoteInput.Action.UP, button = button)
    )
}

private fun sendKeyTap(server: PairingServer, keyCode: Int) {
    server.sendRemoteInput(RemoteInput(RemoteInput.Type.KEY, RemoteInput.Action.DOWN, keyCode = keyCode))
    server.sendRemoteInput(RemoteInput(RemoteInput.Type.KEY, RemoteInput.Action.UP, keyCode = keyCode))
}

/**
 * Sends committed IME text in UTF-8 chunks of at most 4096 bytes. Enter and
 * Tab are sent as special-key events because the daemon rejects control chars.
 * The daemon's current Wayland ydotool backend only supports printable ASCII;
 * non-ASCII text will be rejected there until a Unicode-capable backend exists.
 */
private fun sendImeText(server: PairingServer, text: String) {
    val chunk = StringBuilder()
    var chunkBytes = 0

    fun flush() {
        if (chunk.isNotEmpty()) {
            server.sendRemoteInput(
                RemoteInput(RemoteInput.Type.TEXT, text = chunk.toString())
            )
            chunk.clear()
            chunkBytes = 0
        }
    }

    var index = 0
    while (index < text.length) {
        val codePoint = text.codePointAt(index)
        val value = String(Character.toChars(codePoint))
        index += Character.charCount(codePoint)

        when (codePoint) {
            '\n'.code -> {
                flush()
                sendKeyTap(server, KeyEvent.KEYCODE_ENTER)
            }

            '\r'.code -> {
                flush()
                sendKeyTap(server, KeyEvent.KEYCODE_ENTER)
                if (index < text.length && text[index] == '\n') index++
            }

            '\t'.code -> {
                flush()
                sendKeyTap(server, KeyEvent.KEYCODE_TAB)
            }

            else -> {
                if (Character.isISOControl(codePoint)) continue
                val bytes = value.toByteArray(Charsets.UTF_8).size
                if (chunkBytes + bytes > MAX_TEXT_BYTES) flush()
                chunk.append(value)
                chunkBytes += bytes
            }
        }
    }

    flush()
}
