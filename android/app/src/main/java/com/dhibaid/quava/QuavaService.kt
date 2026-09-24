package com.dhibaid.quava

import android.app.*
import android.content.Context
import android.content.Intent
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
        val notif = NotificationCompat.Builder(this, CHANNEL)
            .setContentTitle("Quava")
            .setContentText("Listening for pairing requests")
            .setSmallIcon(android.R.drawable.stat_sys_data_bluetooth)
            .setOngoing(true)
            .build()
        startForeground(1, notif)

        server = PairingServer(scope)
        mdns = MdnsAdvertiser(this)
        if (server.start(PORT)) mdns.start(PORT)
    }

    override fun onDestroy() {
        mdns.stop()
        server.stop()
        scope.cancel()
        super.onDestroy()
    }

    companion object {
        const val PORT = 48273
        private const val CHANNEL = "quava"
    }
}
