package com.dhibaid.quava

import android.Manifest
import android.content.*
import android.os.Build
import android.os.Bundle
import android.os.IBinder
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.*
import com.dhibaid.quava.ui.PairingScreen

class MainActivity : ComponentActivity() {
    private var service by mutableStateOf<QuavaService?>(null)

    private val conn = object : ServiceConnection {
        override fun onServiceConnected(n: ComponentName, b: IBinder) {
            service = (b as QuavaService.LocalBinder).service()
        }

        override fun onServiceDisconnected(n: ComponentName) {
            service = null
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val intent = Intent(this, QuavaService::class.java)
        startForegroundService(intent)
        bindService(intent, conn, BIND_AUTO_CREATE)

        setContent {
            // Android 13+ needs runtime permission for the notification
            val launcher = rememberLauncherForActivityResult(
                ActivityResultContracts.RequestPermission()
            ) {}
            LaunchedEffect(Unit) {
                if (Build.VERSION.SDK_INT >= 33) {
                    launcher.launch(Manifest.permission.POST_NOTIFICATIONS)
                }
            }

            service?.let { PairingScreen(it.server) }
        }
    }

    override fun onDestroy() {
        unbindService(conn)
        super.onDestroy()
    }
}