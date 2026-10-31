package com.dhibaid.quava.ui

import android.content.Context
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.dhibaid.quava.NotificationSettings

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotificationSettingsScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val prefs = remember {
        context.getSharedPreferences("quava-notification-settings", Context.MODE_PRIVATE)
    }

    var disableGroups by remember { mutableStateOf(prefs.getBoolean("disable_group_notifications", false)) }
    var useAppIcon by remember { mutableStateOf(prefs.getBoolean("use_app_icon", false)) }
    var silentPolicy by remember {
        mutableStateOf(prefs.getString(NotificationSettings.SILENT_POLICY, NotificationSettings.SILENT_DROP)
            ?: NotificationSettings.SILENT_DROP)
    }
    var mutedCategories by remember {
        mutableStateOf(prefs.getStringSet(NotificationSettings.MUTED_CATEGORIES, emptySet()).orEmpty().toSet())
    }
    var mutedApps by remember {
        mutableStateOf(prefs.getStringSet(NotificationSettings.MUTED_APPS, emptySet()).orEmpty().toSet())
    }
    // "package<TAB>label", sorted by label.
    val seenApps = remember {
        prefs.getStringSet(NotificationSettings.SEEN_APPS, emptySet()).orEmpty()
            .map { it.substringBefore('\t') to it.substringAfter('\t', it) }
            .sortedBy { it.second.lowercase() }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Notification settings") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(horizontal = 20.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            item { SectionHeader("General") }
            item {
                SwitchRow(
                    title = "Disable group notifications",
                    subtitle = "Ignore Android group-summary notifications.",
                    checked = disableGroups,
                ) {
                    disableGroups = it
                    prefs.edit().putBoolean("disable_group_notifications", it).apply()
                }
            }
            item {
                SwitchRow(
                    title = "Use app icon",
                    subtitle = "Use the app icon instead of the contact avatar or notification image.",
                    checked = useAppIcon,
                ) {
                    useAppIcon = it
                    prefs.edit().putBoolean("use_app_icon", it).apply()
                }
            }

            item { SectionHeader("Silent notifications") }
            item {
                Text(
                    "Notifications Android shows without sound or a pop-up.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            val policies = listOf(
                NotificationSettings.SILENT_DROP to "Don't send",
                NotificationSettings.SILENT_PRESERVE to "Send, show quietly on the computer",
                NotificationSettings.SILENT_NORMAL to "Send, show like any other",
            )
            items(policies) { (value, label) ->
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable {
                            silentPolicy = value
                            prefs.edit().putString(NotificationSettings.SILENT_POLICY, value).apply()
                        },
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    RadioButton(
                        selected = silentPolicy == value,
                        onClick = {
                            silentPolicy = value
                            prefs.edit().putString(NotificationSettings.SILENT_POLICY, value).apply()
                        },
                    )
                    Text(label)
                }
            }

            item { SectionHeader("Send these types") }
            items(NotificationSettings.CATEGORIES) { (category, label) ->
                SwitchRow(
                    title = label,
                    subtitle = null,
                    checked = category !in mutedCategories,
                ) { send ->
                    mutedCategories = if (send) mutedCategories - category else mutedCategories + category
                    prefs.edit()
                        .putStringSet(NotificationSettings.MUTED_CATEGORIES, HashSet(mutedCategories))
                        .apply()
                }
            }

            item { SectionHeader("Send from these apps") }
            if (seenApps.isEmpty()) {
                item {
                    Text(
                        "Apps appear here after they post a notification.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            items(seenApps, key = { it.first }) { (pkg, label) ->
                SwitchRow(
                    title = label,
                    subtitle = if (label == pkg) null else pkg,
                    checked = pkg !in mutedApps,
                ) { send ->
                    mutedApps = if (send) mutedApps - pkg else mutedApps + pkg
                    prefs.edit()
                        .putStringSet(NotificationSettings.MUTED_APPS, HashSet(mutedApps))
                        .apply()
                }
            }
            item { Column(modifier = Modifier.padding(bottom = 24.dp)) {} }
        }
    }
}

@Composable
private fun SectionHeader(text: String) {
    Column(modifier = Modifier.padding(top = 16.dp)) {
        Text(
            text,
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.primary,
        )
        HorizontalDivider(modifier = Modifier.padding(top = 4.dp))
    }
}

@Composable
private fun SwitchRow(
    title: String,
    subtitle: String?,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title)
            if (subtitle != null) {
                Text(
                    subtitle,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}
