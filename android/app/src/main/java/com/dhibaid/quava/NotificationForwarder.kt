package com.dhibaid.quava

import android.app.Notification
import android.app.NotificationManager
import android.app.RemoteInput
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

/**
 * Forwards Android notifications to Linux and executes commands
 * received from the Linux client.
 */
class NotificationForwarder : NotificationListenerService() {

    override fun onListenerConnected() {
        super.onListenerConnected()
        instance = this

        Log.i(TAG, "Notification listener connected")
        activeNotifications?.forEach(::forward)
    }

    override fun onDestroy() {
        if (instance === this) {
            instance = null
        }
        super.onDestroy()
    }

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        forward(sbn)
    }

    override fun onNotificationRemoved(sbn: StatusBarNotification) {
        if (sbn.packageName != packageName) {
            NotificationTransport.sendDismiss(sbn.key)
        }
    }

    private fun forward(sbn: StatusBarNotification) {
        // Never mirror Quava's own notifications.
        if (sbn.packageName == packageName) return

        val notification = sbn.notification
        val prefs = getSharedPreferences("quava-notification-settings", MODE_PRIVATE)

        if (
            prefs.getBoolean("disable_group_notifications", false) &&
            notification.flags and Notification.FLAG_GROUP_SUMMARY != 0
        ) {
            Log.d(TAG, "Ignoring group summary from ${sbn.packageName}")
            return
        }

        // Ignore media playback notifications, including notifications
        // that expose a media-session token.
        if (
            notification.category == Notification.CATEGORY_TRANSPORT ||
            notification.extras.containsKey(Notification.EXTRA_MEDIA_SESSION)
        ) {
            Log.d(TAG, "Ignoring media notification from ${sbn.packageName}")
            return
        }

        // Ignore notifications configured not to alert.
        if (isSilent(sbn)) {
            Log.d(TAG, "Ignoring silent notification from ${sbn.packageName}")
            return
        }

        val extras = notification.extras
        val title = extras
            .getCharSequence(Notification.EXTRA_TITLE)
            ?.toString()
            .orEmpty()

        val body = extras
            .getCharSequence(Notification.EXTRA_BIG_TEXT)
            ?.toString()
            ?.takeIf { it.isNotBlank() }
            ?: extras.getCharSequence(Notification.EXTRA_TEXT)
                ?.toString()
                .orEmpty()

        val app = try {
            packageManager.getApplicationLabel(
                packageManager.getApplicationInfo(sbn.packageName, 0)
            ).toString()
        } catch (_: Exception) {
            sbn.packageName
        }

        val actions = notification.actions?.map { action ->
            NotificationTransport.Action(
                title = action.title?.toString().orEmpty(),
                hasRemoteInput = action.remoteInputs?.isNotEmpty() == true
            )
        }.orEmpty()

        Log.d(TAG, "Forwarding $app: $title (${actions.size} actions)")

        NotificationTransport.sendNotification(
            NotificationTransport.NotificationData(
                key = sbn.key,
                app = app,
                title = title,
                body = body,
                icon = notificationArtwork(sbn),
                actions = actions
            )
        )
    }

    /**
     * Extracts the most relevant image for a notification.
     *
     * For MessagingStyle notifications, prefer the latest message sender's
     * Person icon. This is typically the contact avatar shown by KDE Connect
     * and GSConnect, rather than the messaging application's logo.
     *
     * Fallbacks are the notification large icon, expanded picture, and
     * finally the small application/status icon.
     */
    private fun notificationArtwork(sbn: StatusBarNotification): ByteArray? {
        val notification = sbn.notification
        val extras = notification.extras

        return try {
            val useAppIcon = getSharedPreferences(
                "quava-notification-settings", MODE_PRIVATE
            ).getBoolean("use_app_icon", false)

            if (useAppIcon) {
                try {
                    val appIcon = packageManager.getApplicationIcon(sbn.packageName)
                    drawableToPng(appIcon, 128)?.let { return it }
                } catch (e: Exception) {
                    Log.d(TAG, "Could not load application icon", e)
                }
            } else {
                val senderIcon = messagingSenderIcon(notification)
                if (senderIcon != null) {
                    iconToPng(senderIcon)?.let { return it }
                }

                extraImage(extras, Notification.EXTRA_LARGE_ICON)?.let {
                    return bitmapToPng(it)
                }

                extraImage(extras, Notification.EXTRA_PICTURE)?.let {
                    return bitmapToPng(it)
                }
            }

            val smallIcon = notification.smallIcon
            if (smallIcon != null) {
                drawableToPng(smallIcon.loadDrawable(this), 64)?.let { return it }
            }

            null
        } catch (e: Exception) {
            Log.w(TAG, "Failed to extract notification artwork", e)
            null
        }
    }

    /**
     * Returns the newest available sender avatar from a MessagingStyle
     * notification. Person icons are available from Android 9 (API 28).
     */
    private fun messagingSenderIcon(notification: Notification): Icon? {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.P) return null

        return try {
            val extras = notification.extras ?: return null
            val messages = extras.getParcelableArray(Notification.EXTRA_MESSAGES)
                ?: return null

            for (entry in messages.reversed()) {
                val message = entry as? Bundle ?: continue
                val person = message.getParcelable("sender_person")
                        as? android.app.Person ?: continue

                person.icon?.let { return it }
            }

            null
        } catch (e: Exception) {
            Log.d(TAG, "No MessagingStyle sender icon available", e)
            null
        }
    }

    /**
     * Android extras can contain either Bitmap or Icon depending on the API
     * level and the app that created the notification.
     */
    private fun extraImage(extras: Bundle, key: String): Bitmap? {
        return try {
            when (val value = extras.get(key)) {
                is Bitmap -> value
                is Icon -> {
                    val drawable = value.loadDrawable(this) ?: return null
                    drawableToBitmap(drawable, 256)
                }

                else -> null
            }
        } catch (e: Exception) {
            Log.d(TAG, "Notification extra '$key' is not a usable image", e)
            null
        }
    }

    private fun iconToPng(icon: Icon): ByteArray? {
        return try {
            val drawable = icon.loadDrawable(this) ?: return null
            drawableToPng(drawable, 128)
        } catch (e: Exception) {
            Log.w(TAG, "Failed to load notification sender icon", e)
            null
        }
    }

    private fun drawableToPng(
        drawable: android.graphics.drawable.Drawable?,
        size: Int
    ): ByteArray? {
        if (drawable == null) return null

        val bitmap = drawableToBitmap(drawable, size)
        return try {
            bitmapToPng(bitmap)
        } finally {
            bitmap.recycle()
        }
    }

    private fun drawableToBitmap(
        drawable: android.graphics.drawable.Drawable,
        size: Int
    ): Bitmap {
        val bitmap = Bitmap.createBitmap(
            size,
            size,
            Bitmap.Config.ARGB_8888
        )

        Canvas(bitmap).also { canvas ->
            drawable.setBounds(0, 0, size, size)
            drawable.draw(canvas)
        }

        return bitmap
    }

    private fun bitmapToPng(bitmap: Bitmap): ByteArray {
        return ByteArrayOutputStream().use { output ->
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)
            output.toByteArray()
        }
    }

    /**
     * Best-effort detection of notifications configured not to alert.
     *
     * This cannot reliably detect every notification silenced by
     * Do Not Disturb, device settings, or app-specific behavior.
     */
    private fun isSilent(sbn: StatusBarNotification): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return false
        }

        return try {
            val notification = sbn.notification
            val manager = getSystemService(NotificationManager::class.java)
            val channel = manager.getNotificationChannel(notification.channelId)

            channel != null &&
                    channel.importance < NotificationManager.IMPORTANCE_DEFAULT
        } catch (e: Exception) {
            Log.w(TAG, "Could not determine whether notification is silent", e)
            false
        }
    }

    private fun performAction(
        key: String,
        index: Int,
        reply: String
    ): Boolean {
        return try {
            val notification = activeNotifications
                ?.firstOrNull { it.key == key }
                ?.notification
                ?: return false

            val action = notification.actions?.getOrNull(index)
                ?: return false

            val fillIn = Intent()

            if (reply.isNotEmpty()) {
                val inputs = action.remoteInputs.orEmpty()

                if (inputs.isNotEmpty()) {
                    val results = Bundle().apply {
                        inputs.forEach { input ->
                            putCharSequence(input.resultKey, reply)
                        }
                    }

                    RemoteInput.addResultsToIntent(inputs, fillIn, results)
                }
            }

            action.actionIntent.send(this, 0, fillIn)
            true
        } catch (e: Exception) {
            Log.w(TAG, "Notification action failed", e)
            false
        }
    }

    companion object {
        private const val TAG = "QuavaNotifications"

        @Volatile
        private var instance: NotificationForwarder? = null

        fun performAction(
            key: String,
            index: Int,
            reply: String
        ): Boolean {
            return instance?.performAction(key, index, reply) ?: false
        }

        fun dismiss(key: String): Boolean {
            val service = instance ?: return false

            return try {
                service.cancelNotification(key)
                true
            } catch (e: Exception) {
                Log.w(TAG, "Failed to dismiss notification", e)
                false
            }
        }

        fun forwardActive() {
            instance?.activeNotifications?.forEach { sbn ->
                instance?.forward(sbn)
            }
        }
    }
}

object NotificationTransport {

    data class Action(
        val title: String,
        val hasRemoteInput: Boolean
    )

    data class NotificationData(
        val key: String,
        val app: String,
        val title: String,
        val body: String,
        val icon: ByteArray?,
        val actions: List<Action>
    )

    @Volatile
    var sender: ((NotificationData) -> Unit)? = null

    @Volatile
    var dismissSender: ((String) -> Unit)? = null

    fun sendNotification(data: NotificationData) {
        val currentSender = sender

        if (currentSender == null) {
            Log.w(TAG, "Dropping notification: no active Quava session")
            return
        }

        try {
            currentSender(data)
        } catch (e: Exception) {
            Log.e(TAG, "Failed to send notification", e)
        }
    }

    fun sendDismiss(key: String) {
        try {
            dismissSender?.invoke(key)
        } catch (e: Exception) {
            Log.e(TAG, "Failed to send notification dismissal", e)
        }
    }

    private const val TAG = "QuavaNotifications"
}