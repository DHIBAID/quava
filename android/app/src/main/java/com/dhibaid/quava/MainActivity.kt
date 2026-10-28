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
import androidx.activity.compose.BackHandler
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.*
import androidx.core.content.ContextCompat
import com.dhibaid.quava.ui.HomeScreen
import com.dhibaid.quava.ui.RemoteControlScreen
import com.dhibaid.quava.ui.MediaControlScreen
import com.dhibaid.quava.ui.theme.QuavaTheme

class MainActivity : ComponentActivity() {
    private var service by mutableStateOf<QuavaService?>(null)
    private var openMediaRequested by mutableStateOf(false)

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
        if (intent.getBooleanExtra(QuavaService.EXTRA_OPEN_MEDIA, false)) {
            openMediaRequested = true
            intent.removeExtra(QuavaService.EXTRA_OPEN_MEDIA)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        openMediaRequested = intent.getBooleanExtra(QuavaService.EXTRA_OPEN_MEDIA, false)

        val serviceIntent = Intent(this, QuavaService::class.java)
        startForegroundService(serviceIntent)
        bindService(serviceIntent, conn, BIND_AUTO_CREATE)

        setContent {
            QuavaTheme {
                var settingsOpen by remember { mutableStateOf(false) }
                var remoteControlOpen by remember { mutableStateOf(false) }
                var mediaControlOpen by remember {
                    mutableStateOf(
                        intent.getBooleanExtra(
                            QuavaService.EXTRA_OPEN_MEDIA,
                            false
                        )
                    )
                }
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

                LaunchedEffect(openMediaRequested) {
                    if (openMediaRequested) {
                        mediaControlOpen = true
                        openMediaRequested = false
                    }
                }

                service?.let {
                    when {
                        remoteControlOpen -> {
                            BackHandler { remoteControlOpen = false }
                            RemoteControlScreen(server = it.server, onBack = { remoteControlOpen = false })
                        }

                        mediaControlOpen -> {
                            BackHandler { mediaControlOpen = false }
                            MediaControlScreen(server = it.server, onBack = { mediaControlOpen = false })
                        }

                        settingsOpen -> {
                            com.dhibaid.quava.ui.NotificationSettingsScreen(
                                onBack = { settingsOpen = false }
                            )
                        }

                        else -> HomeScreen(
                            server = it.server,
                            onSettingsClick = { settingsOpen = true },
                            onRemoteControlClick = { remoteControlOpen = true },
                            onMediaControlClick = { mediaControlOpen = true },
                            onEnableNotificationAccess = {
                                startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))
                            },
                        )
                    }
                }
            }
        }
    }

    override fun onDestroy() {
        unbindService(conn)
        super.onDestroy()
    }
}
