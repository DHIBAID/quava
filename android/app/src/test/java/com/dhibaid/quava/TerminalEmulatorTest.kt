package com.dhibaid.quava

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TerminalEmulatorTest {
    @Test
    fun preservesStylesAcrossChunksAndDoesNotRenderEscapeCodes() {
        val terminal = TerminalEmulator(columns = 20, rows = 4)

        terminal.feed("list \u001b[01;34m")
        terminal.feed("src\u001b[0m\n")

        val line = terminal.snapshot().lines[0]
        assertEquals("list src", line.take(8).joinToString("") { it.character.toString() })
        assertTrue(line[5].bold)
        // "01;34" is LS_COLORS' default for directories; bold is promoted to
        // the bright palette (every mainstream terminal's convention), so
        // this reads as a legible light blue rather than a near-invisible
        // navy on the app's near-black terminal background.
        assertEquals(0x5555FF, line[5].foreground)
        assertFalse(line.any { it.character == '[' })
        assertFalse(line.any { it.character == 'm' })
    }

    @Test
    fun promotesBoldBaseColorsToBrightButLeavesPlainColorsAlone() {
        val terminal = TerminalEmulator(columns = 20, rows = 4)

        // Plain (non-bold) blue stays the dim/normal palette colour.
        terminal.feed("\u001b[34mA\u001b[0m")
        // Bold set *after* the color code still promotes it (order-independent).
        terminal.feed("\u001b[34;1mB\u001b[0m")
        // An explicit bright color plus bold stays bright, not double-brightened.
        terminal.feed("\u001b[1;94mC\u001b[0m")

        val line = terminal.snapshot().lines[0]
        assertEquals(0x0000AA, line[0].foreground)
        assertEquals(0x5555FF, line[1].foreground)
        assertEquals(0x5555FF, line[2].foreground)
    }

    @Test
    fun handlesCursorMovementAndErase() {
        val terminal = TerminalEmulator(columns = 10, rows = 3)

        terminal.feed("abc\u001b[2DXY\u001b[K")

        assertEquals("aXY       ", terminal.snapshot().lines[0].joinToString("") { it.character.toString() })
    }

    @Test
    fun treatsPrivateCsiParametersAsParameters() {
        val terminal = TerminalEmulator(columns = 20, rows = 4)

        terminal.feed("\u001b[?25lvisible\u001b[?25h")

        assertEquals("visible", terminal.snapshot().lines[0].take(7).joinToString("") { it.character.toString() })
    }

    @Test
    fun scrollsTheScreenWhenTheCursorReachesTheBottom() {
        val terminal = TerminalEmulator(columns = 4, rows = 2)

        terminal.feed("one\ntwo\nthree")

        assertEquals("thre", terminal.snapshot().lines[0].joinToString("") { it.character.toString() })
        assertEquals("e   ", terminal.snapshot().lines[1].joinToString("") { it.character.toString() })
    }

    @Test
    fun keepsLinesScrolledOffTheTopInScrollback() {
        val terminal = TerminalEmulator(columns = 4, rows = 2)

        terminal.feed("one\ntwo\nthree")

        // "three" is 5 chars on a 4-column terminal, so it wraps and forces a
        // second scroll - "one" and "two" both get pushed into scrollback.
        val scrollback = terminal.snapshot().scrollback
        assertEquals(2, scrollback.size)
        assertEquals("one ", scrollback[0].joinToString("") { it.character.toString() })
        assertEquals("two ", scrollback[1].joinToString("") { it.character.toString() })
    }

    @Test
    fun capsScrollbackSoItDoesNotGrowWithoutBound() {
        val terminal = TerminalEmulator(columns = 4, rows = 1)

        repeat(2100) { terminal.feed("x\n") }

        assertEquals(2000, terminal.snapshot().scrollback.size)
    }

    @Test
    fun togglesCursorVisibilityWithThePrivateCsiMode() {
        val terminal = TerminalEmulator(columns = 20, rows = 4)

        terminal.feed("\u001b[?25l")
        assertFalse(terminal.snapshot().cursorVisible)

        terminal.feed("\u001b[?25h")
        assertTrue(terminal.snapshot().cursorVisible)
    }

    @Test
    fun collapsesASurrogatePairIntoASingleCell() {
        val terminal = TerminalEmulator(columns = 10, rows = 2)

        // U+1F680 ROCKET, encoded as a UTF-16 surrogate pair.
        terminal.feed("ab🚀cd")

        val line = terminal.snapshot().lines[0]
        assertEquals("ab�cd     ", line.joinToString("") { it.character.toString() })
    }

    @Test
    fun marksCellsWrittenUnderReverseVideo() {
        // This is how zsh's PROMPT_EOL_MARK draws its highlighted "%".
        val terminal = TerminalEmulator(columns = 10, rows = 2)

        terminal.feed("\u001b[7m%\u001b[27mx")

        val line = terminal.snapshot().lines[0]
        assertTrue(line[0].reverse)
        assertFalse(line[1].reverse)
    }
}
