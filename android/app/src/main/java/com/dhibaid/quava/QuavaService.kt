package com.dhibaid.quava

import android.app.*
import android.content.Context
import android.content.Intent
import android.app.PendingIntent
import android.os.Binder
import android.os.IBinder
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.*

class QuavaService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    lateinit var server: PairingServer
        private set
    private lateinit var mdns: MdnsAdvertiser

    inner class LocalBinder : Binder() {
        fun service() = this@QuavaService
    }

    private val binder = LocalBinder()

    override fun onBind(intent: Intent?): IBinder = binder

    override fun onCreate() {
        super.onCreate()
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.createNotificationChannel(
            NotificationChannel(CHANNEL, "Quava", NotificationManager.IMPORTANCE_LOW)
        )
        nm.createNotificationChannel(
            NotificationChannel(PING_CHANNEL, "Quava pings", NotificationManager.IMPORTANCE_DEFAULT)
        )
        val notif = foregroundNotification("Listening for connections")
        startForeground(1, notif)

        server = PairingServer(scope, this)
        mdns = MdnsAdvertiser(this)
        if (server.start(PORT)) mdns.start(PORT, server.localDeviceIdHex())
    }

    public fun onConnected(peerName: String) {
        // update contents of the notification to indicate that we are connected to a peer
        val notif = foregroundNotification("Connected to $peerName")
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(1, notif)
    }

    
    public fun onDisconnected() {
        // update contents of the notification to indicate that we are connected to a peer
        val notif = foregroundNotification("Listening for connections")
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(1, notif)
    }

    public fun onPing(peerName: String) {
        val notification = NotificationCompat.Builder(this, PING_CHANNEL)
            .setContentTitle("Quava ping received")
            .setContentText("Ping received from $peerName")
            .setSmallIcon(android.R.drawable.stat_sys_data_bluetooth)
            .setAutoCancel(true)
            .setContentIntent(homePendingIntent())
            .build()
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(PING_NOTIFICATION_ID, notification)
    }
    
    override fun onDestroy() {
        mdns.stop()
        server.stop()
        scope.cancel()
        super.onDestroy()
    }

    private fun foregroundNotification(text: String) = NotificationCompat.Builder(this, CHANNEL)
        .setContentTitle("Quava")
        .setContentText(text)
        .setSmallIcon(android.R.drawable.stat_sys_data_bluetooth)
        .setOngoing(true)
        .setContentIntent(homePendingIntent())
        .build()

    private fun homePendingIntent(): PendingIntent = PendingIntent.getActivity(
        this,
        0,
        Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP
        },
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )

    companion object {
        const val PORT = 48273
        private const val CHANNEL = "quava"
        private const val PING_CHANNEL = "quava_pings"
        private const val PING_NOTIFICATION_ID = 2
    }
}
