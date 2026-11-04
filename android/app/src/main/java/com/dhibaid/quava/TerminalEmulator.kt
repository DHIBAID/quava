package com.dhibaid.quava

/**
 * Small stateful VT100/ANSI screen model. It deliberately keeps terminal
 * control handling out of Compose so output is interpreted before rendering.
 */
class TerminalEmulator(
    private var columns: Int = DEFAULT_COLUMNS,
    private var rows: Int = DEFAULT_ROWS,
) {
    data class Cell(
        val character: Char = ' ',
        val foreground: Int = DEFAULT_FOREGROUND,
        val background: Int? = null,
        val bold: Boolean = false,
        val underline: Boolean = false,
        val reverse: Boolean = false,
    )

    data class Snapshot(
        val lines: List<List<Cell>>,
        val scrollback: List<List<Cell>>,
        val cursorRow: Int,
        val cursorColumn: Int,
        val cursorVisible: Boolean,
    )

    private enum class ParserState { TEXT, ESCAPE, CSI, OSC }
    private var parserState = ParserState.TEXT
    private val parameters = StringBuilder()
    private var cursorColumn = 0
    private var cursorRow = 0
    private var savedColumn = 0
    private var savedRow = 0
    private var foreground = DEFAULT_FOREGROUND
    // Tracks the base-8 ANSI index (30-37) behind `foreground`, if any, so
    // `put()` can promote it to the bright variant when bold is also set -
    // the "bold is bright" convention every mainstream terminal follows.
    // Without it, LS_COLORS' default "01;34" directories render as the dim
    // navy 0x0000AA, nearly unreadable on a near-black background.
    private var foregroundAnsiIndex: Int? = null
    private var background: Int? = null
    private var bold = false
    private var underline = false
    private var reverse = false
    private var cursorVisible = true
    private var pendingHighSurrogate: Char? = null
    private var screen = Array(rows) { Array(columns) { Cell() } }
    // Lines pushed off the top of the grid by a normal bottom-of-screen
    // line feed. Capped so very chatty sessions (build logs, etc.) can't
    // grow this unboundedly; oldest history is dropped first, same as any
    // real terminal's scrollback limit.
    private val scrollback = ArrayDeque<List<Cell>>()

    @Synchronized
    fun reset() {
        parserState = ParserState.TEXT
        parameters.clear()
        cursorColumn = 0
        cursorRow = 0
        savedColumn = 0
        savedRow = 0
        foreground = DEFAULT_FOREGROUND
        foregroundAnsiIndex = null
        background = null
        bold = false
        underline = false
        reverse = false
        cursorVisible = true
        pendingHighSurrogate = null
        screen = Array(rows) { Array(columns) { Cell() } }
        scrollback.clear()
    }

    @Synchronized
    fun resize(newColumns: Int, newRows: Int) {
        val safeColumns = newColumns.coerceIn(20, 240)
        val safeRows = newRows.coerceIn(4, 400)
        if (safeColumns == columns && safeRows == rows) return
        val next = Array(safeRows) { row ->
            Array(safeColumns) { column ->
                screen.getOrNull(row)?.getOrNull(column) ?: Cell()
            }
        }
        columns = safeColumns
        rows = safeRows
        screen = next
        cursorColumn = cursorColumn.coerceAtMost(columns - 1)
        cursorRow = cursorRow.coerceAtMost(rows - 1)
    }

    @Synchronized
    fun feed(value: String) {
        value.forEach(::consume)
    }

    @Synchronized
    fun snapshot(): Snapshot =
        Snapshot(screen.map { it.toList() }, scrollback.toList(), cursorRow, cursorColumn, cursorVisible)

    private fun consume(character: Char) {
        // A Char is one UTF-16 code unit; anything above the Basic
        // Multilingual Plane (emoji, some newer Nerd Font icon ranges)
        // arrives as a surrogate pair. Cell can only hold one Char, so a
        // complete pair renders as a single replacement glyph instead of
        // leaking both halves as separate garbage cells.
        if (Character.isSurrogate(character)) {
            val high = pendingHighSurrogate
            when {
                high != null && Character.isLowSurrogate(character) -> {
                    pendingHighSurrogate = null
                    put('�')
                }
                Character.isHighSurrogate(character) -> pendingHighSurrogate = character
                else -> pendingHighSurrogate = null
            }
            return
        }
        pendingHighSurrogate = null

        when (parserState) {
            ParserState.TEXT -> when (character) {
                '\u001b' -> parserState = ParserState.ESCAPE
                '\u009b' -> {
                    parameters.clear()
                    parserState = ParserState.CSI
                }
                '\n' -> {
                    // A bare LF from a PTY in cooked mode never arrives
                    // without a preceding CR, but anything feeding raw text
                    // (tests, non-cooked output) still expects it to behave
                    // like a newline rather than a pure vertical index.
                    cursorColumn = 0
                    lineFeed()
                }
                '\r' -> cursorColumn = 0
                '\b' -> cursorColumn = (cursorColumn - 1).coerceAtLeast(0)
                '\t' -> cursorColumn = (((cursorColumn / TAB_SIZE) + 1) * TAB_SIZE)
                    .coerceAtMost(columns - 1)
                '\u0007' -> Unit
                in ' '..'\u007e', in '\u00a0'..'\uffff' -> put(character)
            }
            ParserState.ESCAPE -> when (character) {
                '[' -> {
                    parameters.clear()
                    parserState = ParserState.CSI
                }
                ']' -> parserState = ParserState.OSC
                '7' -> {
                    savedColumn = cursorColumn
                    savedRow = cursorRow
                    parserState = ParserState.TEXT
                }
                '8' -> {
                    cursorColumn = savedColumn
                    cursorRow = savedRow
                    parserState = ParserState.TEXT
                }
                'D' -> {
                    lineFeed()
                    parserState = ParserState.TEXT
                }
                'M' -> {
                    if (cursorRow > 0) cursorRow--
                    parserState = ParserState.TEXT
                }
                'c' -> {
                    reset()
                }
                else -> parserState = ParserState.TEXT
            }
            ParserState.CSI -> {
                if (character.code in 0x40..0x7e) {
                    handleCsi(character, parameters.toString())
                    parameters.clear()
                    parserState = ParserState.TEXT
                } else if (character.code in 0x20..0x3f) {
                    parameters.append(character)
                } else {
                    parserState = ParserState.TEXT
                }
            }
            ParserState.OSC -> if (character == '\u0007') {
                parserState = ParserState.TEXT
            } else if (character == '\u001b') {
                parserState = ParserState.ESCAPE
            }
        }
    }

    private fun handleCsi(finalByte: Char, raw: String) {
        val privateMode = raw.startsWith("?")
        val values = raw.removePrefix("?").removePrefix(">").split(';')
            .map { it.toIntOrNull() ?: 0 }
        fun value(index: Int, default: Int = 1) = (values.getOrNull(index) ?: 0).let {
            if (it == 0) default else it
        }
        when (finalByte) {
            'A' -> cursorRow = (cursorRow - value(0)).coerceAtLeast(0)
            'B', 'e' -> cursorRow = (cursorRow + value(0)).coerceAtMost(rows - 1)
            'C', 'a' -> cursorColumn = (cursorColumn + value(0)).coerceAtMost(columns - 1)
            'D' -> cursorColumn = (cursorColumn - value(0)).coerceAtLeast(0)
            'G', '`' -> cursorColumn = (value(0) - 1).coerceIn(0, columns - 1)
            'd' -> cursorRow = (value(0) - 1).coerceIn(0, rows - 1)
            'H', 'f' -> {
                cursorRow = (value(0) - 1).coerceIn(0, rows - 1)
                cursorColumn = (value(1) - 1).coerceIn(0, columns - 1)
            }
            // Unlike cursor-movement and insert/delete counts, an omitted
            // parameter on erase commands means mode 0 ("to end"), not 1.
            'J' -> eraseDisplay(value(0, default = 0))
            'K' -> eraseLine(value(0, default = 0))
            'P' -> eraseCharacters(value(0))
            '@' -> insertCharacters(value(0))
            'L' -> insertLines(value(0))
            'M' -> deleteLines(value(0))
            'm' -> applySgr(values)
            'h', 'l' -> if (privateMode && values.firstOrNull() == 25) {
                cursorVisible = finalByte == 'h'
            }
            's' -> {
                savedColumn = cursorColumn
                savedRow = cursorRow
            }
            'u' -> {
                cursorColumn = savedColumn.coerceIn(0, columns - 1)
                cursorRow = savedRow.coerceIn(0, rows - 1)
            }
        }
    }

    private fun applySgr(values: List<Int>) {
        val codes = if (values.isEmpty() || values.all { it == 0 }) listOf(0) else values
        var index = 0
        while (index < codes.size) {
            when (val code = codes[index]) {
                0 -> {
                    foreground = DEFAULT_FOREGROUND
                    foregroundAnsiIndex = null
                    background = null
                    bold = false
                    underline = false
                    reverse = false
                }
                1 -> bold = true
                22 -> bold = false
                4 -> underline = true
                24 -> underline = false
                7 -> reverse = true
                27 -> reverse = false
                39 -> {
                    foreground = DEFAULT_FOREGROUND
                    foregroundAnsiIndex = null
                }
                49 -> background = null
                in 30..37 -> {
                    foreground = ansiColor(code - 30, false)
                    foregroundAnsiIndex = code - 30
                }
                in 90..97 -> {
                    foreground = ansiColor(code - 90, true)
                    foregroundAnsiIndex = null
                }
                in 40..47 -> background = ansiColor(code - 40, false)
                in 100..107 -> background = ansiColor(code - 100, true)
                38, 48 -> {
                    val targetBackground = code == 48
                    val color = when {
                        codes.getOrNull(index + 1) == 5 ->
                            codes.getOrNull(index + 2)?.let(::ansi256Color).also { index += 2 }
                        codes.getOrNull(index + 1) == 2 && index + 4 < codes.size -> {
                            val rgb = codes.subList(index + 2, index + 5)
                            index += 4
                            (rgb[0].coerceIn(0, 255) shl 16) or
                                (rgb[1].coerceIn(0, 255) shl 8) or rgb[2].coerceIn(0, 255)
                        }
                        else -> null
                    }
                    if (color != null) {
                        if (targetBackground) {
                            background = color
                        } else {
                            foreground = color
                            foregroundAnsiIndex = null
                        }
                    }
                }
            }
            index++
        }
    }

    private fun put(character: Char) {
        if (cursorColumn >= columns) {
            cursorColumn = 0
            lineFeed()
        }

        val effectiveForeground = if (bold && foregroundAnsiIndex != null) {
            ansiColor(foregroundAnsiIndex!!, true)
        } else {
            foreground
        }
        screen[cursorRow][cursorColumn] = Cell(character, effectiveForeground, background, bold, underline, reverse)
        cursorColumn++
    }

    private fun eraseCharacters(count: Int) {
        val end = (cursorColumn + count).coerceAtMost(columns)
        for (column in cursorColumn until end) screen[cursorRow][column] = Cell()
    }

    private fun insertCharacters(count: Int) {
        val amount = count.coerceAtLeast(1).coerceAtMost(columns - cursorColumn)
        for (column in columns - 1 downTo cursorColumn + amount) {
            screen[cursorRow][column] = screen[cursorRow][column - amount]
        }
        for (column in cursorColumn until cursorColumn + amount) screen[cursorRow][column] = Cell()
    }

    private fun insertLines(count: Int) {
        val amount = count.coerceAtLeast(1).coerceAtMost(rows - cursorRow)
        repeat(amount) {
            for (row in rows - 1 downTo cursorRow + 1) screen[row] = screen[row - 1]
            screen[cursorRow] = Array(columns) { Cell() }
        }
    }

    private fun deleteLines(count: Int) {
        val amount = count.coerceAtLeast(1).coerceAtMost(rows - cursorRow)
        repeat(amount) {
            for (row in cursorRow until rows - 1) screen[row] = screen[row + 1]
            screen[rows - 1] = Array(columns) { Cell() }
        }
    }

    private fun lineFeed() {
        if (cursorRow == rows - 1) {
            scrollback.addLast(screen[0].toList())
            if (scrollback.size > MAX_SCROLLBACK) scrollback.removeFirst()
            for (row in 1 until rows) screen[row - 1] = screen[row]
            screen[rows - 1] = Array(columns) { Cell() }
        } else {
            cursorRow++
        }
    }

    private fun eraseDisplay(mode: Int) {
        when (mode) {
            2 -> screen = Array(rows) { Array(columns) { Cell() } }
            0 -> {
                eraseLine(0)
                for (row in cursorRow + 1 until rows) screen[row] = Array(columns) { Cell() }
            }
            1 -> {
                for (row in 0 until cursorRow) screen[row] = Array(columns) { Cell() }
                eraseLine(1)
            }
        }
    }

    private fun eraseLine(mode: Int) {
        val start = when (mode) { 1 -> 0; 2 -> 0; else -> cursorColumn }
        val end = when (mode) { 1 -> cursorColumn; else -> columns }
        for (column in start until end) screen[cursorRow][column] = Cell()
    }

    private companion object {
        const val DEFAULT_COLUMNS = 80
        const val DEFAULT_ROWS = 24
        const val MAX_SCROLLBACK = 2000
        const val TAB_SIZE = 8
        const val DEFAULT_FOREGROUND = 0xE8E3D8

        fun ansiColor(index: Int, bright: Boolean): Int {
            val colors = if (bright) intArrayOf(
                0x555555, 0xFF5555, 0x55FF55, 0xFFFF55,
                0x5555FF, 0xFF55FF, 0x55FFFF, 0xFFFFFF,
            ) else intArrayOf(
                0x000000, 0xAA0000, 0x00AA00, 0xAA5500,
                0x0000AA, 0xAA00AA, 0x00AAAA, 0xAAAAAA,
            )
            return colors[index.coerceIn(0, colors.lastIndex)]
        }

        fun ansi256Color(value: Int): Int {
            if (value < 16) return ansiColor(value % 8, value >= 8)
            if (value in 232..255) {
                val gray = 8 + (value - 232) * 10
                return (gray shl 16) or (gray shl 8) or gray
            }
            val color = value - 16
            fun channel(component: Int) = if (component == 0) 0 else 55 + component * 40
            return (channel((color / 36) % 6) shl 16) or
                (channel((color / 6) % 6) shl 8) or channel(color % 6)
        }
    }
}
