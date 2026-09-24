package com.dhibaid.quava.ui

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.dhibaid.quava.PairingServer

@Composable
fun PairingScreen(server: PairingServer) {
    val s by server.state.collectAsStateWithLifecycle()

    MaterialTheme {
        Surface(Modifier.fillMaxSize()) {
            Column(
                Modifier.fillMaxSize().padding(24.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp, Alignment.CenterVertically),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text("Quava", style = MaterialTheme.typography.headlineLarge)
                Text(s.status, style = MaterialTheme.typography.bodyMedium)

                if (s.confirmationPending) {
                    Text("Pairing request from ${s.pendingPeerName}")
                    Text(s.pairingCode, fontSize = 48.sp)
                    Text("Confirm this code matches the one on your other device")
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        OutlinedButton(onClick = server::rejectPairing) { Text("Reject") }
                        Button(onClick = server::confirmPairing) { Text("Confirm") }
                    }
                }

                if (s.paired) Text("Paired ✓", style = MaterialTheme.typography.titleMedium)
            }
        }
    }
}
