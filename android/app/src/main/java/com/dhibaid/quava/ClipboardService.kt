package com.quava.android.services

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class ClipboardService(
    context: Context,
    private val onClipboardChanged: suspend (ByteArray) -> Unit
) {
    private val appContext = context.applicationContext
    private val clipboardManager =
        appContext.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    private var listener: ClipboardManager.OnPrimaryClipChangedListener? = null
    private var lastClipboard: ByteArray? = null

    @Synchronized
    fun start() {
        if (listener != null) return

        listener = ClipboardManager.OnPrimaryClipChangedListener {
            scope.launch {
                val text = readClipboardText() ?: return@launch
                val bytes = text.toByteArray(Charsets.UTF_8)

                if (lastClipboard?.contentEquals(bytes) == true) {
                    return@launch
                }

                lastClipboard = bytes.copyOf()
                onClipboardChanged(bytes)
            }
        }

        clipboardManager.addPrimaryClipChangedListener(listener)
    }

    suspend fun applyRemoteClipboard(content: ByteArray) {
        withContext(Dispatchers.Main) {
            if (lastClipboard?.contentEquals(content) == true) return@withContext

            lastClipboard = content.copyOf()
            clipboardManager.setPrimaryClip(
                ClipData.newPlainText(
                    "Quava",
                    content.toString(Charsets.UTF_8)
                )
            )
        }
    }

    private fun readClipboardText(): String? {
        val clip = clipboardManager.primaryClip ?: return null
        if (clip.itemCount == 0) return null

        return clip.getItemAt(0).coerceToText(appContext)?.toString()
    }

    @Synchronized
    fun stop() {
        listener?.let {
            clipboardManager.removePrimaryClipChangedListener(it)
        }
        listener = null
        scope.cancel()
    }
}