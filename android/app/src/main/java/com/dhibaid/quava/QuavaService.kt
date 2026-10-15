package com.dhibaid.quava

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Binder
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import android.util.Log
import com.quava.android.services.ClipboardService

class QuavaService : Service() {

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    lateinit var server: PairingServer
        private set

    private lateinit var mdns: MdnsAdvertiser
    private lateinit var ringController: RingController
    private lateinit var clipboardService: ClipboardService
    private lateinit var hostNotificationManager: HostNotificationManager

    private val ringTimeoutHandler = Handler(Looper.getMainLooper())

    private val ringTimeout = Runnable {
        Log.i(TAG, "Ring timed out")
        stopRing()
    }

    inner class LocalBinder : Binder() {
        fun service() = this@QuavaService
    }

    private val binder = LocalBinder()

    override fun onBind(intent: Intent?): IBinder = binder

    override fun onCreate() {
        super.onCreate()

        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager

        nm.createNotificationChannel(
            NotificationChannel(
                CHANNEL,
                "Quava",
                NotificationManager.IMPORTANCE_LOW
            )
        )

        nm.createNotificationChannel(
            NotificationChannel(
                PING_CHANNEL,
                "Quava pings",
                NotificationManager.IMPORTANCE_DEFAULT
            )
        )

        nm.createNotificationChannel(
            NotificationChannel(
                RING_CHANNEL,
                "Quava rings",
                NotificationManager.IMPORTANCE_HIGH
            )
        )

        startForeground(
            FOREGROUND_NOTIFICATION_ID,
            foregroundNotification("Listening for connections")
        )

        server = PairingServer(scope, this)
        mdns = MdnsAdvertiser(this)
        ringController = RingController(this)
        hostNotificationManager = HostNotificationManager(this)
        NotificationTransport.sender =
            { notification -> scope.launch(Dispatchers.IO) { server.sendAndroidNotification(notification) } }
        NotificationTransport.dismissSender =
            { key -> scope.launch(Dispatchers.IO) { server.sendAndroidNotificationDismiss(key) } }
        NotificationForwarder.forwardActive()


        clipboardService = ClipboardService(applicationContext) { content ->
            Log.d("QuavaClipboard", "Local clipboard changed: ${content.size} bytes")
        }

        clipboardService.start()

        if (server.start(PORT)) {
            mdns.start(PORT, server.localDeviceIdHex())
        }
    }

    override fun onStartCommand(
        intent: Intent?,
        flags: Int,
        startId: Int
    ): Int {
        when (intent?.action) {
            ACTION_STOP_RING -> stopRing()
            ACTION_CLEAR_HOST_NOTIFICATION -> {
                val id = intent.getIntExtra(EXTRA_NOTIFICATION_ID, -1)
                if (id >= 0) {
                    Log.i(TAG, "Clearing host notification $id after Android dismissal")
                    scope.launch(Dispatchers.IO) {
                        server.sendNotificationClear(id)
                    }
                    try {
                        hostNotificationManager.dismiss(id)
                    } catch (_: Exception) {
                    }
                }
            }
        }

        return START_STICKY
    }

    fun onConnected(peerName: String) {
        val notif = foregroundNotification("Connected to: $peerName.")
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(FOREGROUND_NOTIFICATION_ID, notif)
    }

    fun onDisconnected() {
        val notif = foregroundNotification("Listening for connections")
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(FOREGROUND_NOTIFICATION_ID, notif)
    }

    fun onPing(peerName: String) {
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

    fun onRing(peerName: String) {
        Log.i(TAG, "Ring request from: $peerName")

        ringTimeoutHandler.removeCallbacks(ringTimeout)
        ringController.start()

        // Stop ringing after 30 seconds if the notification is ignored.
        ringTimeoutHandler.postDelayed(ringTimeout, RING_TIMEOUT_MS)

        val openIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_CLEAR_TOP or
                    Intent.FLAG_ACTIVITY_SINGLE_TOP
            putExtra(EXTRA_STOP_RING, true)
        }

        val pendingIntent = PendingIntent.getActivity(
            this,
            RING_NOTIFICATION_ID,
            openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        // Stop ringing if the notification is swiped away.
        val dismissIntent = Intent(this, QuavaService::class.java).apply {
            action = ACTION_STOP_RING
        }

        val dismissPendingIntent = PendingIntent.getService(
            this,
            RING_NOTIFICATION_ID + 10,
            dismissIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val notification = NotificationCompat.Builder(this, RING_CHANNEL)
            .setContentTitle("Quava is ringing")
            .setContentText("Tap to stop ringing from $peerName")
            .setSmallIcon(android.R.drawable.ic_lock_silent_mode_off)
            .setContentIntent(pendingIntent)
            .setDeleteIntent(dismissPendingIntent)
            .setAutoCancel(true)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .build()

        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(RING_NOTIFICATION_ID, notification)
    }

    fun stopRing() {
        ringTimeoutHandler.removeCallbacks(ringTimeout)

        if (::ringController.isInitialized) {
            ringController.stop()
        }

        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        nm.cancel(RING_NOTIFICATION_ID)
    }

    override fun onDestroy() {
        stopRing()

        if (::mdns.isInitialized) {
            mdns.stop()
        }

        if (::server.isInitialized) {
            server.stop()
        }

        scope.cancel()
        NotificationTransport.sender = null
        NotificationTransport.dismissSender = null
        super.onDestroy()
    }

    internal suspend fun onClipboardSync(message: PairingProtocol.Message) {
        val contentType = message.payload[1] as? String ?: return
        val content = message.payload[2] as? ByteArray ?: return

        if (contentType == "text") {
            clipboardService.applyRemoteClipboard(content)
        }
    }

    internal suspend fun onNotificationSync(message: PairingProtocol.Message) {
        Log.d("QuavaNotif", "Notification sync received")

        val notification = decodeHostNotification(message)
        if (notification == null) {
            Log.e("QuavaNotif", "Failed to decode notification")
            return
        }

        Log.d(
            "QuavaNotif",
            "Decoded: app=${notification.appName}, " +
                    "title=${notification.title}, body=${notification.body}, " +
                    "id=${notification.id}"
        )

        hostNotificationManager.show(notification)
        Log.d("QuavaNotif", "show() invoked")
    }

    internal suspend fun onNotificationClear(message: PairingProtocol.Message) {
        val id = (message.payload[1] as? Number)?.toInt() ?: -1
        Log.d("QuavaNotif", "Notification clear received id=$id")
        if (id >= 0) {
            if (!hostNotificationManager.dismiss(id)) {
                hostNotificationManager.dismissAll()
            }
        } else {
            hostNotificationManager.dismissAll()
        }
    }

    internal suspend fun onAndroidNotificationAction(message: PairingProtocol.Message) {
        val key = message.payload[1] as? String ?: return
        val action = (message.payload[2] as? Number)?.toInt() ?: return
        val performed = NotificationForwarder.performAction(key, action, message.payload[3] as? String ?: "")
        Log.i("QuavaNotif", "Android notification action key=$key action=$action performed=$performed")
    }

    internal suspend fun onAndroidNotificationDismiss(message: PairingProtocol.Message) {
        val key = message.payload[1] as? String ?: return
        Log.i("QuavaNotif", "Android notification dismissal key=$key performed=${NotificationForwarder.dismiss(key)}")
    }

    private fun decodeHostNotification(
        message: PairingProtocol.Message
    ): HostNotification? {
        val payload = message.payload

        fun get(key: Int): Any? = payload[key]

        val appName = get(1) as? String
        val replacesId = (get(2) as? Number)?.toInt() ?: 0
        val appIcon = get(3) as? String ?: ""
        val iconData = get(9) as? ByteArray
        val summary = get(4) as? String
        val body = get(5) as? String ?: ""

        if (appName == null || summary == null) {
            Log.e(
                "QuavaNotif",
                "Decode failed: appName=${get(1)?.let { it::class.simpleName }}, " +
                        "summary=${get(4)?.let { it::class.simpleName }}, " +
                        "keys=${payload.keys}"
            )
            return null
        }

        return HostNotification(
            id = replacesId,
            appName = appName,
            title = summary,
            body = body,
            icon = appIcon,
            iconData = iconData,
        )
    }

    private fun foregroundNotification(text: String) =
        NotificationCompat.Builder(this, CHANNEL)
            .setContentTitle("Quava")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.stat_sys_data_bluetooth)
            .setOngoing(true)
            .setContentIntent(homePendingIntent())
            .build()

    private fun homePendingIntent(): PendingIntent =
        PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_CLEAR_TOP or
                        Intent.FLAG_ACTIVITY_SINGLE_TOP
            },
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )


    companion object {
        const val PORT = 48273

        const val EXTRA_STOP_RING = "STOP_RING"
        const val EXTRA_NOTIFICATION_ID = "NOTIFICATION_ID"

        private const val TAG = "QuavaRing"
        private const val ACTION_STOP_RING = "com.dhibaid.quava.STOP_RING"
        const val ACTION_CLEAR_HOST_NOTIFICATION = "com.dhibaid.quava.CLEAR_HOST_NOTIFICATION"

        private const val CHANNEL = "quava"
        private const val PING_CHANNEL = "quava_pings"
        private const val RING_CHANNEL = "quava_rings"

        private const val FOREGROUND_NOTIFICATION_ID = 1
        private const val PING_NOTIFICATION_ID = 2
        private const val RING_NOTIFICATION_ID = 3

        private const val RING_TIMEOUT_MS = 30_000L
    }
}
