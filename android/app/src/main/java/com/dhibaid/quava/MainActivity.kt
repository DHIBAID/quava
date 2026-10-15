package com.dhibaid.quava

import android.Manifest
import android.content.*
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.os.IBinder
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.*
import androidx.core.content.ContextCompat
import com.dhibaid.quava.ui.HomeScreen

class MainActivity : ComponentActivity() {
    private var service by mutableStateOf<QuavaService?>(null)

    private val conn = object : ServiceConnection {
        override fun onServiceConnected(n: ComponentName, b: IBinder) {
            service = (b as QuavaService.LocalBinder).service()

            if (intent.getBooleanExtra("STOP_RING", false)) {
                service?.stopRing()
                intent.removeExtra("STOP_RING")
            }
        }

        override fun onServiceDisconnected(n: ComponentName) {
            service = null
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)

        if (intent.getBooleanExtra("STOP_RING", false)) {
            service?.stopRing()
            intent.removeExtra("STOP_RING")
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val serviceIntent = Intent(this, QuavaService::class.java)
        startForegroundService(serviceIntent)
        bindService(serviceIntent, conn, BIND_AUTO_CREATE)

        setContent {
            var settingsOpen by remember { mutableStateOf(false) }
            val launcher = rememberLauncherForActivityResult(
                ActivityResultContracts.RequestPermission()
            ) { granted ->
                if (!granted) {
                    // Permission denied.
                }
            }

            LaunchedEffect(Unit) {
                if (
                    Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                    ContextCompat.checkSelfPermission(
                        this@MainActivity,
                        Manifest.permission.POST_NOTIFICATIONS
                    ) != PackageManager.PERMISSION_GRANTED
                ) {
                    launcher.launch(Manifest.permission.POST_NOTIFICATIONS)
                }
            }

            service?.let {
                if (settingsOpen) {
                    com.dhibaid.quava.ui.NotificationSettingsScreen(
                        onBack = { settingsOpen = false }
                    )
                } else HomeScreen(
                    server = it.server,
                    onSettingsClick = { settingsOpen = true },
                    onEnableNotificationAccess = {
                        startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))
                    },
                )
            }
        }
    }

    override fun onDestroy() {
        unbindService(conn)
        super.onDestroy()
    }
}
