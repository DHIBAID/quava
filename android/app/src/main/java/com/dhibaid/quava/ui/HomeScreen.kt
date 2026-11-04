package com.dhibaid.quava.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Computer
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.LinkOff
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material.icons.filled.MusicNote
import androidx.compose.material.icons.filled.Mouse
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.NotificationsActive
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.NavigationDrawerItem
import androidx.compose.material3.NavigationDrawerItemDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberDrawerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.app.NotificationManagerCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.dhibaid.quava.PairingServer
import com.dhibaid.quava.ui.theme.accentColor
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    server: PairingServer,
    onEnableNotificationAccess: () -> Unit = {},
    onSettingsClick: () -> Unit = {},
    onRemoteControlClick: () -> Unit = {},
    onMediaControlClick: () -> Unit = {},
    onTerminalClick: () -> Unit = {},
) {
    val state by server.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val drawerState = rememberDrawerState(initialValue = DrawerValue.Closed)
    val scope = rememberCoroutineScope()

    var confirmUnpair by remember { mutableStateOf(false) }
    var showAbout by remember { mutableStateOf(false) }

    // Re-check notification access whenever the user comes back from system settings.
    val lifecycleOwner = LocalLifecycleOwner.current
    var resumeCount by remember { mutableIntStateOf(0) }
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) resumeCount++
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }
    val notificationAccess = remember(resumeCount) {
        NotificationManagerCompat.getEnabledListenerPackages(context).contains(context.packageName)
    }

    fun closeDrawerThen(action: () -> Unit) {
        scope.launch { drawerState.close() }
        action()
    }

    ModalNavigationDrawer(
        drawerState = drawerState,
        drawerContent = {
            QuavaDrawer(
                hostName = state.hostName,
                connected = state.connected,
                paired = state.paired,
                onSettingsClick = { closeDrawerThen(onSettingsClick) },
                onUnpairClick = { closeDrawerThen { confirmUnpair = true } },
                onAboutClick = { closeDrawerThen { showAbout = true } },
            )
        },
    ) {
        Scaffold(
            topBar = {
                TopAppBar(
                    title = {
                        Text(
                            text = "Quava",
                            fontWeight = FontWeight.ExtraBold,
                        )
                    },
                    navigationIcon = {
                        IconButton(onClick = { scope.launch { drawerState.open() } }) {
                            Icon(Icons.Filled.Menu, contentDescription = "Menu")
                        }
                    },
                )
            },
        ) { innerPadding ->
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(innerPadding)
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 20.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                if (state.confirmationPending) {
                    PairingRequestCard(
                        peerName = state.pendingPeerName,
                        pairingCode = state.pairingCode,
                        onReject = server::rejectPairing,
                        onConfirm = server::confirmPairing,
                    )
                } else {
                    StatusCard(
                        paired = state.paired,
                        connected = state.connected,
                        hostName = state.hostName,
                        status = state.status,
                    )
                }

                if (state.connected) {
                    Text(
                        text = "CONTROL YOUR COMPUTER",
                        style = MaterialTheme.typography.labelMedium,
                        fontWeight = FontWeight.Bold,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        letterSpacing = 1.sp,
                        modifier = Modifier.padding(top = 4.dp, start = 4.dp),
                    )

                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        FeatureTile(
                            icon = Icons.Filled.MusicNote,
                            title = "Media",
                            caption = "Play, pause, skip",
                            onClick = onMediaControlClick,
                            modifier = Modifier.weight(1f),
                        )
                        FeatureTile(
                            icon = Icons.Filled.Mouse,
                            title = "Remote input",
                            caption = "Touchpad & keyboard",
                            onClick = onRemoteControlClick,
                            modifier = Modifier.weight(1f),
                        )
                    }
                    FeatureTile(
                        icon = Icons.Filled.Terminal,
                        title = "Terminal",
                        caption = "Run commands",
                        onClick = onTerminalClick,
                        modifier = Modifier.fillMaxWidth(),
                    )
                } else if (!state.confirmationPending && !state.paired) {
                    HowToPairCard()
                }

                if (!notificationAccess) {
                    NotificationAccessCard(onEnable = onEnableNotificationAccess)
                }

                Spacer(Modifier.height(8.dp))
            }
        }
    }

    if (confirmUnpair) {
        AlertDialog(
            onDismissRequest = { confirmUnpair = false },
            title = { Text("Unpair ${state.hostName ?: "this computer"}?") },
            text = {
                Text(
                    "Your phone and the computer will both forget each other. " +
                            "You'll need to pair again to reconnect."
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmUnpair = false
                        server.unpair()
                    },
                ) {
                    Text("Unpair", color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmUnpair = false }) { Text("Cancel") }
            },
        )
    }

    if (showAbout) {
        val version = remember {
            runCatching {
                context.packageManager.getPackageInfo(context.packageName, 0).versionName
            }.getOrNull() ?: "unknown"
        }
        AlertDialog(
            onDismissRequest = { showAbout = false },
            title = { Text("Quava") },
            text = { Text("Version $version\n\nConnect your phone to your Linux computer.") },
            confirmButton = {
                TextButton(onClick = { showAbout = false }) { Text("Close") }
            },
        )
    }
}

// ---- Cards ---------------------------------------------------------------------------

@Composable
private fun StatusCard(
    paired: Boolean,
    connected: Boolean,
    hostName: String?,
    status: String,
) {
    val colors = MaterialTheme.colorScheme

    val headline = when {
        connected -> hostName ?: "Connected"
        paired -> hostName ?: "Paired computer"
        else -> "Not paired yet"
    }
    val label = when {
        connected -> "Connected"
        paired -> "Offline"
        else -> "Waiting"
    }

    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(28.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surfaceContainerLow),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(22.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Box(
                    modifier = Modifier
                        .size(56.dp)
                        .clip(CircleShape)
                        .background(if (connected) colors.primary else colors.surfaceContainerHighest),
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(
                        imageVector = Icons.Filled.Computer,
                        contentDescription = null,
                        tint = if (connected) colors.onPrimary else colors.onSurfaceVariant,
                        modifier = Modifier.size(28.dp),
                    )
                }

                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = headline,
                        style = MaterialTheme.typography.titleLarge,
                        fontWeight = FontWeight.Bold,
                        maxLines = 1,
                    )
                    Spacer(Modifier.height(6.dp))
                    StatusPill(label = label, active = connected)
                }
            }

            HorizontalDivider(color = colors.outlineVariant.copy(alpha = 0.4f))

            Text(
                text = status,
                style = MaterialTheme.typography.bodyMedium,
                color = colors.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun StatusPill(label: String, active: Boolean) {
    val colors = MaterialTheme.colorScheme

    Surface(
        shape = CircleShape,
        color = if (active) colors.primary.copy(alpha = 0.18f) else colors.surfaceContainerHighest,
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 5.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                modifier = Modifier
                    .size(8.dp)
                    .clip(CircleShape)
                    .background(if (active) colors.primary else colors.outline),
            )
            Spacer(Modifier.size(8.dp))
            Text(
                text = label,
                style = MaterialTheme.typography.labelLarge,
                fontWeight = FontWeight.SemiBold,
                color = if (active) accentColor() else colors.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun FeatureTile(
    icon: ImageVector,
    title: String,
    caption: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = MaterialTheme.colorScheme

    Card(
        onClick = onClick,
        modifier = modifier,
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surfaceContainerHigh),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            Box(
                modifier = Modifier
                    .size(44.dp)
                    .clip(CircleShape)
                    .background(colors.primary.copy(alpha = 0.18f)),
                contentAlignment = Alignment.Center,
            ) {
                Icon(icon, contentDescription = null, tint = accentColor())
            }

            Column {
                Text(
                    text = title,
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.Bold,
                )
                Text(
                    text = caption,
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun NotificationAccessCard(onEnable: () -> Unit) {
    val colors = MaterialTheme.colorScheme

    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surfaceContainerLow),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(18.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            Icon(
                imageVector = Icons.Filled.NotificationsActive,
                contentDescription = null,
                tint = colors.tertiary,
                modifier = Modifier.size(28.dp),
            )

            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = "Show phone notifications on your computer",
                    style = MaterialTheme.typography.titleSmall,
                    fontWeight = FontWeight.SemiBold,
                )
                Spacer(Modifier.height(2.dp))
                Text(
                    text = "Quava needs notification access to forward them.",
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.onSurfaceVariant,
                )
            }

            OutlinedButton(onClick = onEnable) { Text("Enable") }
        }
    }
}

@Composable
private fun HowToPairCard() {
    val colors = MaterialTheme.colorScheme

    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surfaceContainerLow),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(22.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(
                text = "Pair with your computer",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Bold,
            )

            listOf(
                "Keep Quava open on this phone.",
                "On your computer, open Quava and choose Find devices.",
                "Pick this phone, then check that the codes match.",
            ).forEachIndexed { index, step ->
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(
                        text = "${index + 1}",
                        style = MaterialTheme.typography.labelLarge,
                        fontWeight = FontWeight.Bold,
                        color = accentColor(),
                    )
                    Text(
                        text = step,
                        style = MaterialTheme.typography.bodyMedium,
                        color = colors.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Composable
private fun PairingRequestCard(
    peerName: String,
    pairingCode: String,
    onReject: () -> Unit,
    onConfirm: () -> Unit,
) {
    val colors = MaterialTheme.colorScheme

    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(28.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surfaceContainerHigh),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = "Pairing request",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold,
            )

            Spacer(Modifier.height(4.dp))

            Text(
                text = peerName,
                style = MaterialTheme.typography.bodyLarge,
                color = colors.onSurfaceVariant,
            )

            Spacer(Modifier.height(20.dp))

            Surface(
                shape = RoundedCornerShape(20.dp),
                color = colors.primary.copy(alpha = 0.16f),
            ) {
                Text(
                    text = pairingCode,
                    modifier = Modifier.padding(horizontal = 28.dp, vertical = 16.dp),
                    fontSize = 38.sp,
                    fontWeight = FontWeight.ExtraBold,
                    letterSpacing = 4.sp,
                    color = accentColor(),
                )
            }

            Spacer(Modifier.height(14.dp))

            Text(
                text = "Make sure this code matches the one on your computer.",
                style = MaterialTheme.typography.bodySmall,
                color = colors.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )

            Spacer(Modifier.height(22.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                OutlinedButton(onClick = onReject, modifier = Modifier.weight(1f)) {
                    Text("Reject")
                }
                Button(onClick = onConfirm, modifier = Modifier.weight(1f)) {
                    Text("Confirm")
                }
            }
        }
    }
}

// ---- Drawer ---------------------------------------------------------------------------

@Composable
private fun QuavaDrawer(
    hostName: String?,
    connected: Boolean,
    paired: Boolean,
    onSettingsClick: () -> Unit,
    onUnpairClick: () -> Unit,
    onAboutClick: () -> Unit,
) {
    val colors = MaterialTheme.colorScheme

    ModalDrawerSheet {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 24.dp, vertical = 28.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(
                text = hostName ?: "Quava",
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
            )
            StatusPill(
                label = when {
                    connected -> "Connected"
                    paired -> "Offline"
                    else -> "Not paired"
                },
                active = connected,
            )
        }

        HorizontalDivider(color = colors.outlineVariant.copy(alpha = 0.4f))
        Spacer(Modifier.height(8.dp))

        NavigationDrawerItem(
            icon = { Icon(Icons.Filled.Notifications, contentDescription = null) },
            label = { Text("Notification settings") },
            selected = false,
            onClick = onSettingsClick,
            modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding),
        )

        NavigationDrawerItem(
            icon = { Icon(Icons.Filled.Info, contentDescription = null) },
            label = { Text("About Quava") },
            selected = false,
            onClick = onAboutClick,
            modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding),
        )

        if (paired) {
            Spacer(Modifier.height(8.dp))
            HorizontalDivider(color = colors.outlineVariant.copy(alpha = 0.4f))
            Spacer(Modifier.height(8.dp))

            NavigationDrawerItem(
                icon = {
                    Icon(
                        Icons.Filled.LinkOff,
                        contentDescription = null,
                        tint = colors.error,
                    )
                },
                label = { Text("Unpair", color = colors.error) },
                selected = false,
                onClick = onUnpairClick,
                modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding),
            )
        }
    }
}
