package com.dhibaid.quava.ui


import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Menu
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
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberDrawerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.graphics.Color

import kotlinx.coroutines.launch

import com.dhibaid.quava.PairingServer


@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    server: PairingServer,
    onSettingsClick: () -> Unit = {},
    onUnpairClick: () -> Unit = {},
    onAboutClick: () -> Unit = {},
) {
    val state by server.state.collectAsStateWithLifecycle()

    val drawerState = rememberDrawerState(
        initialValue = DrawerValue.Closed
    )
    val scope = rememberCoroutineScope()

    ModalNavigationDrawer(
        drawerState = drawerState,
        drawerContent = {
            QuavaDrawer(
                hostName = state.hostName ?: "Quava",
                connected = state.connected,
                onClose = {
                    scope.launch {
                        drawerState.close()
                    }
                },
                onSettingsClick = {
                    scope.launch {
                        drawerState.close()
                    }
                    onSettingsClick()
                },
                onUnpairClick = {
                    scope.launch {
                        drawerState.close()
                    }
                    onUnpairClick()
                },
                onAboutClick = {
                    scope.launch {
                        drawerState.close()
                    }
                    onAboutClick()
                },
            )
        },
    ) {
        Scaffold(
            topBar = {
                QuavaTopBar(
                    hostName = state.hostName ?: "Quava",
                    connected = state.connected,
                    onMenuClick = {
                        scope.launch {
                            drawerState.open()
                        }
                    },
                )
            },
        ) { innerPadding ->

            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(innerPadding)
                    .padding(horizontal = 20.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {

                Spacer(Modifier.height(8.dp))

                when {
                    state.confirmationPending -> {
                        PairingRequestCard(
                            peerName = state.pendingPeerName,
                            pairingCode = state.pairingCode,
                            onReject = server::rejectPairing,
                            onConfirm = server::confirmPairing,
                        )
                    }

                    state.connected -> {
                        ActiveSessionCard(
                            status = state.status,
                        )
                    }

                    state.paired -> {
                        PairedOfflineCard(
                            status = state.status,
                        )
                    }

                    else -> {
                        WaitingCard()
                    }
                }

                Spacer(Modifier.weight(1f))

                Text(
                    text = "Quava",
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(bottom = 16.dp),
                    textAlign = TextAlign.Center,
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun QuavaTopBar(
    hostName: String,
    connected: Boolean,
    onMenuClick: () -> Unit,
) {
    TopAppBar(
        title = {
            Row(
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = hostName,
                    fontWeight = FontWeight.SemiBold,
                )

                Spacer(Modifier.size(10.dp))

                ConnectionIndicator(
                    connected = connected,
                )
            }
        },
        navigationIcon = {
            IconButton(
                onClick = onMenuClick,
            ) {
                Icon(
                    imageVector = Icons.Default.Menu,
                    contentDescription = "Menu",
                )
            }
        },
    )
}

@Composable
private fun ConnectionIndicator(
    connected: Boolean,
) {
    val color = if (connected) {
        Color(0xFF4CAF50) // Green color for connected state
    } else {
        Color(0xFF9E9E9E) // Gray color for disconnected state
    }

    Row(
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Spacer(
            modifier = Modifier
                .size(9.dp)
                .clip(CircleShape)
                .background(color)
        )

        Spacer(Modifier.size(6.dp))

    }
}

@Composable
private fun QuavaDrawer(
    hostName: String,
    connected: Boolean,
    onClose: () -> Unit,
    onSettingsClick: () -> Unit,
    onUnpairClick: () -> Unit,
    onAboutClick: () -> Unit,
) {
    ModalDrawerSheet(
        modifier = Modifier.fillMaxHeight(),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(20.dp),
        ) {
            Text(
                text = hostName,
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.SemiBold,
            )

            Spacer(Modifier.height(6.dp))

            Row(
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Spacer(
                    modifier = Modifier
                        .size(8.dp)
                        .clip(CircleShape)
                        .background(
                            if (connected) {
                                MaterialTheme.colorScheme.primary
                            } else {
                                MaterialTheme.colorScheme.outline
                            }
                        )
                )

                Spacer(Modifier.size(8.dp))

                Text(
                    text = if (connected) "Connected" else "Offline",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        HorizontalDivider()

        Spacer(Modifier.height(8.dp))

        NavigationDrawerItem(
            label = { Text("Files") },
            selected = false,
            onClick = {
                // TODO
                onClose()
            },
        )

        NavigationDrawerItem(
            label = { Text("Screen Sharing") },
            selected = false,
            onClick = {
                // TODO
                onClose()
            },
        )

        NavigationDrawerItem(
            label = { Text("Terminal") },
            selected = false,
            onClick = {
                // TODO
                onClose()
            },
        )

        Spacer(Modifier.height(8.dp))

        HorizontalDivider()

        Spacer(Modifier.height(8.dp))

        NavigationDrawerItem(
            label = { Text("Settings") },
            selected = false,
            onClick = onSettingsClick,
        )

        NavigationDrawerItem(
            label = { Text("Unpair") },
            selected = false,
            onClick = onUnpairClick,
        )

        NavigationDrawerItem(
            label = { Text("About Quava") },
            selected = false,
            onClick = onAboutClick,
        )
    }
}

@Composable
private fun PairingRequestCard(
    peerName: String,
    pairingCode: String,
    onReject: () -> Unit,
    onConfirm: () -> Unit,
) {
    val color = MaterialTheme.colorScheme

    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(
            containerColor = color.primaryContainer,
        ),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = "Pairing request",
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.SemiBold,
            )

            Spacer(Modifier.height(8.dp))

            Text(
                text = peerName,
                style = MaterialTheme.typography.titleMedium,
            )

            Spacer(Modifier.height(20.dp))

            Text(
                text = "Verify this code",
                style = MaterialTheme.typography.bodyMedium,
                color = color.onPrimaryContainer.copy(alpha = 0.75f),
            )

            Spacer(Modifier.height(8.dp))

            Surface(
                shape = RoundedCornerShape(16.dp),
                color = color.surface,
            ) {
                Text(
                    text = pairingCode,
                    modifier = Modifier.padding(
                        horizontal = 28.dp,
                        vertical = 16.dp,
                    ),
                    fontSize = 40.sp,
                    fontWeight = FontWeight.Bold,
                    letterSpacing = 4.sp,
                    color = color.onSurface,
                )
            }

            Spacer(Modifier.height(12.dp))

            Text(
                text = "Make sure this code matches the one shown on your other device.",
                style = MaterialTheme.typography.bodySmall,
                color = color.onPrimaryContainer.copy(alpha = 0.75f),
                textAlign = TextAlign.Center,
            )

            Spacer(Modifier.height(24.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                OutlinedButton(
                    onClick = onReject,
                    modifier = Modifier.weight(1f),
                ) {
                    Text("Reject")
                }

                Button(
                    onClick = onConfirm,
                    modifier = Modifier.weight(1f),
                ) {
                    Text("Confirm")
                }
            }
        }
    }
}

@Composable
private fun ActiveSessionCard(
    status: String,
) {
    val color = MaterialTheme.colorScheme

    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(24.dp),
    ) {
        Column(
            modifier = Modifier.padding(24.dp),
        ) {
            Text(
                text = "Active session",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold,
            )

            Spacer(Modifier.height(16.dp))

            HorizontalDivider()

            Spacer(Modifier.height(16.dp))

            Text(
                text = "Connected to your device.",
                style = MaterialTheme.typography.bodyLarge,
            )

            Spacer(Modifier.height(6.dp))

            Text(
                text = status,
                style = MaterialTheme.typography.bodyMedium,
                color = color.onSurfaceVariant,
            )

            Spacer(Modifier.height(20.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                SessionAction(
                    title = "Files",
                    modifier = Modifier.weight(1f),
                )

                SessionAction(
                    title = "Screen",
                    modifier = Modifier.weight(1f),
                )
            }
        }
    }
}

@Composable
private fun SessionAction(
    title: String,
    modifier: Modifier = Modifier,
) {
    OutlinedButton(
        onClick = {
            // TODO
        },
        modifier = modifier,
    ) {
        Text(title)
    }
}

@Composable
private fun PairedOfflineCard(
    status: String,
) {
    val color = MaterialTheme.colorScheme

    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(
            containerColor = color.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier.padding(24.dp),
        ) {
            Text(
                text = "Paired device",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold,
            )

            Spacer(Modifier.height(8.dp))

            Text(
                text = "The device is paired but there is currently no active session.",
                style = MaterialTheme.typography.bodyMedium,
                color = color.onSurfaceVariant,
            )

            Spacer(Modifier.height(12.dp))

            Text(
                text = status,
                style = MaterialTheme.typography.bodySmall,
                color = color.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun WaitingCard() {
    val color = MaterialTheme.colorScheme

    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(
            containerColor = color.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = "Waiting for a device",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold,
            )

            Spacer(Modifier.height(8.dp))

            Text(
                text = "Keep Quava open while another device discovers and pairs with this one.",
                style = MaterialTheme.typography.bodyMedium,
                color = color.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
        }
    }
}