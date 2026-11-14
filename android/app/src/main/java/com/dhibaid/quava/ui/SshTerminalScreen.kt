package com.dhibaid.quava.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.Canvas as ComposeCanvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.ContentPaste
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.dhibaid.quava.PairingServer
import com.dhibaid.quava.SshTerminal
import com.dhibaid.quava.TerminalEmulator
import com.dhibaid.quava.ui.theme.WarmDarkContainerLowest
import com.dhibaid.quava.ui.theme.accentColor
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import kotlinx.coroutines.launch

// The terminal pane always renders dark, like a real terminal emulator does
// regardless of the host app's light/dark setting. It's tuned to the warm
// neutrals in ui/theme/Color.kt so it still reads as part of Quava rather
// than a bolted-on component.
private val TerminalBackground = WarmDarkContainerLowest

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun SshTerminalScreen(server: PairingServer, scope: CoroutineScope, onBack: () -> Unit) {
    val state by server.state.collectAsStateWithLifecycle()
    val context = androidx.compose.ui.platform.LocalContext.current
    val terminal = remember { SshTerminal(context, scope, server) }
    val snapshot by terminal.screen.collectAsStateWithLifecycle()
    val scrollState = rememberScrollState()
    val toolbarScrollState = rememberScrollState()
    val clipboard = LocalClipboardManager.current
    val uiScope = rememberCoroutineScope()
    val focusRequester = remember { FocusRequester() }
    val density = LocalDensity.current
    val keyboardController = LocalSoftwareKeyboardController.current
    var menuOpen by remember { mutableStateOf(false) }
    var fontSize by remember { mutableStateOf(13.sp) }
    var keyboardInput by remember { mutableStateOf(TextFieldValue()) }
    var ctrlArmed by remember { mutableStateOf(false) }
    var cursorOn by remember { mutableStateOf(true) }

    val accent = accentColor()
    val accentArgb = remember(accent) { accent.toArgb() }
    val cursorInkArgb = remember { TerminalBackground.toArgb() }

    // A Nerd Font-patched monospace font: identical metrics/look to a plain
    // monospace font for normal text, but also covers the Powerline/devicon
    // glyphs shell prompts (starship, p10k, etc.) rely on - so there's no
    // separate fallback font or per-glyph coverage check needed.
    var terminalTypeface by remember { mutableStateOf<android.graphics.Typeface?>(null) }
    var terminalBoldTypeface by remember { mutableStateOf<android.graphics.Typeface?>(null) }
    LaunchedEffect(Unit) {
        withContext(Dispatchers.IO) {
            terminalTypeface = runCatching {
                android.graphics.Typeface.createFromAsset(context.assets, "fonts/RobotoMonoNerdFontMono-Regular.ttf")
            }.getOrNull()
            terminalBoldTypeface = runCatching {
                android.graphics.Typeface.createFromAsset(context.assets, "fonts/RobotoMonoNerdFontMono-Bold.ttf")
            }.getOrNull()
        }
    }

    DisposableEffect(state.connected) {
        if (state.connected) terminal.connect()
        onDispose { terminal.close() }
    }
    // "Was the user at the bottom" has to be judged against the scroll range
    // from *before* this update, not the range after - once scrollback made
    // the content tall, a single burst of output can jump maxValue from ~0
    // to a lot in one frame, and checking against the new (already-bigger)
    // max would wrongly conclude the user had scrolled away and strand the
    // view at the top instead of following the output.
    var lastKnownMaxScroll by remember { mutableStateOf(0) }
    LaunchedEffect(snapshot) {
        val wasPinnedToBottom = scrollState.value >= lastKnownMaxScroll - 32
        if (wasPinnedToBottom) {
            scrollState.scrollTo(scrollState.maxValue)
        }
        lastKnownMaxScroll = scrollState.maxValue
    }
    LaunchedEffect(Unit) {
        focusRequester.requestFocus()
        keyboardController?.show()
    }
    LaunchedEffect(Unit) {
        while (true) {
            delay(530)
            cursorOn = !cursorOn
        }
    }

    fun focusTerminal() {
        focusRequester.requestFocus()
        keyboardController?.show()
    }

    fun send(value: String) {
        if (value.isNotEmpty()) terminal.send(value)
    }

    // The hidden capture field mirrors whatever the IME actually has, and we
    // diff against the previous value instead of force-clearing it after
    // every keystroke. Clearing on every character fights the IME's own
    // buffer - under fast input (or just bad luck with a recomposition, e.g.
    // right after a heavy redraw like `clear`) that race drops or duplicates
    // characters, including Enter itself.
    fun handleTyped(newValue: TextFieldValue) {
        val oldText = keyboardInput.text
        val newText = newValue.text
        if (newText != oldText) {
            var common = 0
            val minLen = minOf(oldText.length, newText.length)
            while (common < minLen && oldText[common] == newText[common]) common++
            val deleteCount = oldText.length - common
            val insertText = newText.substring(common)

            if (ctrlArmed && deleteCount == 0 && insertText.length == 1) {
                terminal.send(controlCodeFor(insertText[0]))
                ctrlArmed = false
            } else {
                val toSend = buildString {
                    repeat(deleteCount) { append('\u007f') }
                    append(insertText)
                }
                send(toSend)
            }

            // A newline is a submitted line - start the next one with a
            // clean buffer instead of letting it grow for the whole session.
            if (insertText.contains('\n') || insertText.contains('\r')) {
                keyboardInput = TextFieldValue()
                return
            }
        }
        keyboardInput = newValue
    }

    fun pasteFromClipboard() {
        clipboard.getText()?.text?.let { text -> if (text.isNotEmpty()) terminal.send(text) }
    }

    Scaffold(
        contentWindowInsets = WindowInsets.ime,
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            "${state.hostName ?: state.hostAddress ?: "Terminal"}:",
                            fontSize = 16.sp,
                            fontWeight = FontWeight.SemiBold,
                        )
                        StatusDot(
                            connected = state.connected,
                            modifier = Modifier.padding(horizontal = 8.dp),
                        )
                        Text(
                            if (state.connected) "Connected" else "Disconnected",
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            fontSize = 13.sp,
                        )
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back")
                    }
                },
                actions = {
                    Box {
                        IconButton(onClick = { menuOpen = true }) {
                            Icon(Icons.Filled.MoreVert, "Terminal settings")
                        }
                        DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                            DropdownMenuItem(
                                text = { Text("Copy screen") },
                                onClick = {
                                    clipboard.setText(androidx.compose.ui.text.AnnotatedString(plainText(snapshot)))
                                    menuOpen = false
                                },
                                leadingIcon = { Icon(Icons.Filled.ContentCopy, null) },
                            )
                            DropdownMenuItem(
                                text = { Text("Paste") },
                                onClick = {
                                    pasteFromClipboard()
                                    menuOpen = false
                                },
                                leadingIcon = { Icon(Icons.Filled.ContentPaste, null) },
                            )
                            DropdownMenuItem(
                                text = { Text("Smaller text") },
                                onClick = {
                                    fontSize = (fontSize.value - 1).coerceAtLeast(10f).sp
                                    menuOpen = false
                                },
                            )
                            DropdownMenuItem(
                                text = { Text("Larger text") },
                                onClick = {
                                    fontSize = (fontSize.value + 1).coerceAtMost(20f).sp
                                    menuOpen = false
                                },
                            )
                        }
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .windowInsetsPadding(WindowInsets.navigationBars)
                .padding(horizontal = 8.dp, vertical = 6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Box(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(20.dp))
                    .background(TerminalBackground)
                    .onSizeChanged { size ->
                        val columns = (size.width / with(density) { (fontSize.value * 0.6f).dp.toPx() }).toInt()
                        val rows = (size.height / with(density) { 18.sp.toPx() }).toInt()
                        if (columns > 0 && rows > 0) terminal.resize(columns, rows, size.width, size.height)
                    }
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null,
                        onClick = ::focusTerminal,
                    )
                    .verticalScroll(scrollState)
                    .padding(horizontal = 10.dp, vertical = 8.dp),
            ) {
                TerminalText(
                    snapshot = snapshot,
                    fontSize = fontSize,
                    showCursor = state.connected && snapshot.cursorVisible && cursorOn,
                    cursorColorArgb = accentArgb,
                    cursorInkArgb = cursorInkArgb,
                    regularTypeface = terminalTypeface ?: android.graphics.Typeface.MONOSPACE,
                    boldTypeface = terminalBoldTypeface
                        ?: android.graphics.Typeface.create(android.graphics.Typeface.MONOSPACE, android.graphics.Typeface.BOLD),
                )
                BasicTextField(
                    value = keyboardInput,
                    onValueChange = ::handleTyped,
                    keyboardOptions = KeyboardOptions(
                        autoCorrectEnabled = false,
                        capitalization = KeyboardCapitalization.None,
                        keyboardType = KeyboardType.Ascii,
                    ),
                    modifier = Modifier
                        .size(1.dp)
                        .focusRequester(focusRequester)
                        .onPreviewKeyEvent { event ->
                            if (event.type == KeyEventType.KeyDown) {
                                val sequence = when (event.key) {
                                    Key.DirectionUp -> "\u001b[A"
                                    Key.DirectionDown -> "\u001b[B"
                                    Key.DirectionRight -> "\u001b[C"
                                    Key.DirectionLeft -> "\u001b[D"
                                    Key.Enter -> "\r"
                                    Key.Backspace -> "\u007f"
                                    Key.Tab -> "\t"
                                    else -> null
                                }
                                if (sequence != null) {
                                    send(sequence)
                                    true
                                } else false
                            } else false
                        },
                    textStyle = TextStyle(color = Color.Transparent, fontSize = 1.sp),
                    cursorBrush = SolidColor(Color.Transparent),
                )
            }

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(toolbarScrollState),
                horizontalArrangement = Arrangement.spacedBy(5.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                TerminalKey("ESC") { send("\u001b") }
                TerminalKey("TAB") { send("\t") }
                TerminalKey("CTRL", active = ctrlArmed) { ctrlArmed = !ctrlArmed }
                listOf(
                    Triple("Left", Icons.AutoMirrored.Filled.KeyboardArrowLeft, "\u001b[D"),
                    Triple("Up", Icons.Filled.KeyboardArrowUp, "\u001b[A"),
                    Triple("Down", Icons.Filled.KeyboardArrowDown, "\u001b[B"),
                    Triple("Right", Icons.AutoMirrored.Filled.KeyboardArrowRight, "\u001b[C"),
                ).forEach { (label, icon, value) ->
                    TerminalKey(label, icon = icon, minWidth = 40.dp) { send(value) }
                }
                listOf(
                    "HOME" to "\u001b[H",
                    "END" to "\u001b[F",
                    "CTRL-C" to "\u0003",
                    "CTRL-D" to "\u0004",
                    "CTRL-L" to "\u000c",
                ).forEach { (label, value) -> TerminalKey(label) { send(value) } }
                IconButton(onClick = ::pasteFromClipboard, modifier = Modifier.size(32.dp)) {
                    Icon(
                        Icons.Filled.ContentPaste,
                        "Paste",
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                IconButton(
                    onClick = { uiScope.launch { scrollState.animateScrollTo(scrollState.maxValue) } },
                    modifier = Modifier.size(32.dp),
                ) {
                    Icon(
                        Icons.Filled.KeyboardArrowDown,
                        "Scroll to bottom",
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Composable
private fun TerminalText(
    snapshot: TerminalEmulator.Snapshot,
    fontSize: androidx.compose.ui.unit.TextUnit,
    showCursor: Boolean,
    cursorColorArgb: Int,
    cursorInkArgb: Int,
    regularTypeface: android.graphics.Typeface,
    boldTypeface: android.graphics.Typeface,
) {
    val density = androidx.compose.ui.platform.LocalDensity.current
    val fontPx = with(density) { fontSize.toPx() }
    val lineHeightPx = fontPx * 1.38f
    val charWidthPx = fontPx * 0.6f
    // Scrollback lines render above the live grid so the whole session's
    // history - not just what currently fits on screen - is scrollable.
    val allLines = remember(snapshot) { snapshot.scrollback + snapshot.lines }
    val cursorRow = snapshot.scrollback.size + snapshot.cursorRow
    ComposeCanvas(
        modifier = Modifier
            .fillMaxWidth()
            .height(with(density) { (allLines.size * lineHeightPx).toDp() }),
    ) {
        drawIntoCanvas { canvas ->
            val paint = android.graphics.Paint().apply {
                typeface = regularTypeface
                textSize = fontPx
                isAntiAlias = true
            }
            allLines.forEachIndexed { row, line ->
                var index = 0
                while (index < line.size) {
                    val first = line[index]
                    var end = index + 1
                    while (end < line.size && sameStyle(first, line[end])) end++
                    val text = buildString(end - index) {
                        for (cellIndex in index until end) append(line[cellIndex].character)
                    }
                    // Reverse video (used by zsh's end-of-line marker, selection
                    // highlights, etc.) swaps the cell's own colors; a cell with
                    // no explicit background falls back to the terminal's own
                    // background colour so the swapped text stays legible.
                    val fillColor = if (first.reverse) first.foreground else first.background
                    val textColor = if (first.reverse) (first.background ?: cursorInkArgb) else first.foreground

                    fillColor?.let {
                        paint.color = it or 0xFF000000.toInt()
                        canvas.nativeCanvas.drawRect(
                            index * charWidthPx,
                            row * lineHeightPx,
                            end * charWidthPx,
                            (row + 1) * lineHeightPx,
                            paint,
                        )
                    }
                    paint.color = textColor or 0xFF000000.toInt()
                    paint.typeface = if (first.bold) boldTypeface else regularTypeface
                    canvas.nativeCanvas.drawText(text, index * charWidthPx, row * lineHeightPx + fontPx, paint)
                    index = end
                }
            }

            if (showCursor && snapshot.cursorRow in snapshot.lines.indices) {
                val line = snapshot.lines[snapshot.cursorRow]
                val column = snapshot.cursorColumn.coerceIn(0, (line.size - 1).coerceAtLeast(0))
                val left = column * charWidthPx
                val top = cursorRow * lineHeightPx
                paint.color = cursorColorArgb or 0xFF000000.toInt()
                paint.typeface = regularTypeface
                canvas.nativeCanvas.drawRect(left, top, left + charWidthPx, top + lineHeightPx, paint)

                val cellChar = line.getOrNull(column)?.character ?: ' '
                if (cellChar != ' ') {
                    paint.color = cursorInkArgb or 0xFF000000.toInt()
                    paint.typeface = regularTypeface
                    canvas.nativeCanvas.drawText(cellChar.toString(), left, top + fontPx, paint)
                }
            }
        }
    }
}

@Composable
private fun StatusDot(connected: Boolean, modifier: Modifier = Modifier) {
    val accent = accentColor()
    Box(
        modifier = modifier
            .size(8.dp)
            .background(if (connected) accent else MaterialTheme.colorScheme.outline, RoundedCornerShape(50)),
    )
}

@Composable
private fun TerminalKey(
    label: String,
    active: Boolean = false,
    minWidth: androidx.compose.ui.unit.Dp = 0.dp,
    icon: androidx.compose.ui.graphics.vector.ImageVector? = null,
    onClick: () -> Unit,
) {
    val colors = MaterialTheme.colorScheme
    val accent = accentColor()
    Surface(
        onClick = onClick,
        color = if (active) accent.copy(alpha = 0.2f) else colors.surfaceContainerHigh,
        contentColor = if (active) accent else colors.onSurfaceVariant,
        shape = RoundedCornerShape(8.dp),
        border = if (active) BorderStroke(1.dp, accent) else null,
    ) {
        Box(
            modifier = Modifier
                .widthIn(min = minWidth)
                .padding(horizontal = 10.dp, vertical = 7.dp),
            contentAlignment = Alignment.Center,
        ) {
            if (icon != null) {
                Icon(icon, label, modifier = Modifier.size(18.dp))
            } else {
                Text(
                    label,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Medium,
                )
            }
        }
    }
}

private fun sameStyle(first: TerminalEmulator.Cell, second: TerminalEmulator.Cell): Boolean =
    first.foreground == second.foreground &&
        first.background == second.background &&
        first.bold == second.bold &&
        first.underline == second.underline &&
        first.reverse == second.reverse

private fun plainText(snapshot: TerminalEmulator.Snapshot): String =
    snapshot.lines.joinToString("\n") { line -> line.joinToString("") { it.character.toString() } }

// Maps a key to the control code a physical keyboard's Ctrl chord would send
// (Ctrl+A..Ctrl+Z, plus the punctuation in the same 0x00-0x1f band).
private fun controlCodeFor(character: Char): String {
    val upper = character.uppercaseChar()
    if (upper < 'A' || upper > '_') return character.toString()
    return (upper.code and 0x1f).toChar().toString()
}
