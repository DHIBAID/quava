package com.dhibaid.quava.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.IconButton
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotificationSettingsScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val prefs = remember {
        context.getSharedPreferences(
            "quava-notification-settings",
            android.content.Context.MODE_PRIVATE
        )
    }

    var disableGroups by remember {
        mutableStateOf(prefs.getBoolean("disable_group_notifications", false))
    }
    var useAppIcon by remember {
        mutableStateOf(prefs.getBoolean("use_app_icon", false))
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Notification settings") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        androidx.compose.material3.Icon(
                            Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "Back"
                        )
                    }
                }
            )
        }
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text("Disable group notifications")
                    Text("Ignore Android group-summary notifications.")
                }
                Switch(
                    checked = disableGroups,
                    onCheckedChange = {
                        disableGroups = it
                        prefs.edit()
                            .putBoolean("disable_group_notifications", it)
                            .apply()
                    }
                )
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text("Use app icon")
                    Text("Use the app icon instead of the contact avatar or notification image.")
                }
                Switch(
                    checked = useAppIcon,
                    onCheckedChange = {
                        useAppIcon = it
                        prefs.edit().putBoolean("use_app_icon", it).apply()
                    }
                )
            }
        }
    }
}
