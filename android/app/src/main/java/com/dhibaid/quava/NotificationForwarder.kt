package com.dhibaid.quava

import android.app.Notification
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.drawable.Icon
import android.os.Build
import android.os.Bundle
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import android.util.Log
import java.io.ByteArrayOutputStream

/** Forwards posted Android notifications and executes commands received from Linux. */
class NotificationForwarder : NotificationListenerService() {
    override fun onListenerConnected() {
        instance = this
        Log.i(TAG, "notification listener connected")
        activeNotifications?.forEach(::forward)
    }

    override fun onDestroy() { if (instance === this) instance = null; super.onDestroy() }
    override fun onNotificationPosted(sbn: StatusBarNotification) = forward(sbn)

    override fun onNotificationRemoved(sbn: StatusBarNotification) {
        if (!sbn.packageName.startsWith(packageName)) {
            NotificationTransport.sendDismiss(sbn.key)
        }
    }

    private fun forward(sbn: StatusBarNotification) {
        if (sbn.packageName == packageName) return // never mirror Quava itself
        val n = sbn.notification
        val extras = n.extras
        val title = extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
        val body = extras.getCharSequence(Notification.EXTRA_TEXT)?.toString().orEmpty()
        val app = try { packageManager.getApplicationLabel(packageManager.getApplicationInfo(sbn.packageName, 0)).toString() } catch (_: Exception) { sbn.packageName }
        val actions = n.actions?.map { action ->
            NotificationTransport.Action(action.title?.toString().orEmpty(), action.remoteInputs?.isNotEmpty() == true)
        }.orEmpty()
        Log.d(TAG, "forwarding ${sbn.packageName}: $title (${actions.size} actions)")
        NotificationTransport.sendNotification(NotificationTransport.NotificationData(sbn.key, app, title, body, iconBytes(n.smallIcon), actions))
    }

    private fun iconBytes(icon: Icon?): ByteArray? = try {
        val drawable = icon?.loadDrawable(this) ?: return null
        val bitmap = Bitmap.createBitmap(64, 64, Bitmap.Config.ARGB_8888)
        Canvas(bitmap).also { canvas -> drawable.setBounds(0, 0, canvas.width, canvas.height); drawable.draw(canvas) }
        ByteArrayOutputStream().use { out -> bitmap.compress(Bitmap.CompressFormat.PNG, 90, out); out.toByteArray() }
    } catch (_: Exception) { null }

    private fun performAction(key: String, index: Int, reply: String): Boolean = try {
        val action = activeNotifications?.firstOrNull { it.key == key }?.notification?.actions?.getOrNull(index) ?: return false
        val fillIn = Intent()
        if (reply.isNotEmpty()) {
            val inputs = action.remoteInputs ?: emptyArray()
            if (inputs.isNotEmpty()) {
                val results = Bundle().apply { inputs.forEach { putCharSequence(it.resultKey, reply) } }
                android.app.RemoteInput.addResultsToIntent(inputs, fillIn, results)
            }
        }
        action.actionIntent.send(this, 0, fillIn)
        true
    } catch (e: Exception) { Log.w(TAG, "notification action failed", e); false }

    companion object {
        private const val TAG = "QuavaNotifications"
        @Volatile private var instance: NotificationForwarder? = null
        fun performAction(key: String, index: Int, reply: String) = instance?.performAction(key, index, reply) ?: false
        fun dismiss(key: String) = try { instance?.cancelNotification(key); instance != null } catch (_: Exception) { false }
        fun forwardActive() { instance?.activeNotifications?.forEach { instance?.forward(it) } }
    }
}

object NotificationTransport {
    data class Action(val title: String, val hasRemoteInput: Boolean)
    data class NotificationData(val key: String, val app: String, val title: String, val body: String, val icon: ByteArray?, val actions: List<Action>)
    @Volatile var sender: ((NotificationData) -> Unit)? = null
    @Volatile var dismissSender: ((String) -> Unit)? = null
    fun sendNotification(data: NotificationData) {
        if (sender == null) Log.w("QuavaNotifications", "dropping notification: no active Quava session")
        sender?.invoke(data)
    }
    fun sendDismiss(key: String) { dismissSender?.invoke(key) }
}
