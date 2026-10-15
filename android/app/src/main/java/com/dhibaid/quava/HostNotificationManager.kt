package com.dhibaid.quava

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.graphics.drawable.Icon
import android.graphics.BitmapFactory
import android.os.Build

data class HostNotification(
    val id: Int,
    val appName: String,
    val title: String,
    val body: String,
    val icon: String = "",
    val iconData: ByteArray? = null,
)

class HostNotificationManager(
    private val context: Context,
    private val onDismiss: ((Int) -> Unit)? = null,
) {
    private val manager =
        context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager

    companion object {
        private const val CHANNEL_ID = "host_notifications"
    }

    init {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "Host notifications",
                NotificationManager.IMPORTANCE_DEFAULT
            )
            manager.createNotificationChannel(channel)
        }
    }

    fun show(notification: HostNotification) {
        val hostIcon = notification.iconData?.let { data ->
            BitmapFactory.decodeByteArray(data, 0, data.size)
        }
        val builder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(context, CHANNEL_ID)
        } else {
            Notification.Builder(context)
        }
            .setSmallIcon(
                hostIcon?.let(Icon::createWithBitmap) ?: Icon.createWithResource(
                    context,
                    android.R.drawable.ic_dialog_info
                )
            )
            .setContentTitle(notification.title)
            .setContentText(notification.body)
            .setSubText(notification.appName)
            .setPriority(Notification.PRIORITY_DEFAULT)
            .setAutoCancel(true)
            .setDeleteIntent(
                clearPendingIntent(notification.id)
            )

        hostIcon?.let(builder::setLargeIcon)

        manager.notify(notification.id, builder.build())
    }

    private fun clearPendingIntent(id: Int): PendingIntent {
        val intent = Intent(context, QuavaService::class.java).apply {
            action = QuavaService.ACTION_CLEAR_HOST_NOTIFICATION
            putExtra(QuavaService.EXTRA_NOTIFICATION_ID, id)
        }
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            PendingIntent.getForegroundService(context, id, intent, flags)
        } else {
            PendingIntent.getService(context, id, intent, flags)
        }
    }

    fun dismiss(id: Int): Boolean {
        val active = manager.activeNotifications.any { it.id == id }
        manager.cancel(id)
        return active
    }

    fun dismissAll() {
        manager.cancelAll()
    }
}
