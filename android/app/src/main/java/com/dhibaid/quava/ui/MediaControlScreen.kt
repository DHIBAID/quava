package com.dhibaid.quava.ui

import android.os.SystemClock
import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.compose.AsyncImage
import com.dhibaid.quava.AudioOutputDevice
import com.dhibaid.quava.HostMediaState
import com.dhibaid.quava.MediaActions
import com.dhibaid.quava.MediaControl
import com.dhibaid.quava.PairingServer
import kotlinx.coroutines.delay
import kotlin.math.roundToLong
import androidx.compose.foundation.basicMarquee
import androidx.compose.foundation.MarqueeAnimationMode
import androidx.compose.foundation.border
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MediaControlScreen(
    server: PairingServer,
    onBack: () -> Unit,
) {
    val state by server.mediaState.collectAsStateWithLifecycle()
    var showSystemAudio by remember { mutableStateOf(false) }

    if (showSystemAudio) {
        SystemAudioScreen(
            server = server,
            state = state,
            onBack = { showSystemAudio = false },
        )
        return
    }

    var progress by remember { mutableFloatStateOf(0f) }
    var seeking by remember { mutableStateOf(false) }
    var volume by remember {
        mutableFloatStateOf(state.volume.toFloat().coerceIn(0f, 1f))
    }

    val artworkModifier = Modifier
        .size(240.dp)
        .clip(RoundedCornerShape(6.dp))
        .border(
            width = 2.dp,
            color = MaterialTheme.colorScheme.outlineVariant,
            shape = RoundedCornerShape(6.dp),
        )

    val send: (MediaControl) -> Unit = { server.sendMediaControl(it) }

    // Request a snapshot when the screen opens. Subsequent state comes from
    // the daemon's MPRIS event stream.
    LaunchedEffect(server) {
        send(MediaControl(MediaActions.STATE))
    }

    LaunchedEffect(
        state.positionUs,
        state.durationUs,
        state.playbackStatus,
        state.title,
        state.artists,
        state.album,
        seeking,
    ) {
        if (seeking) return@LaunchedEffect
        if (state.durationUs <= 0L) {
            if (progress != 0f) progress = 0f
            return@LaunchedEffect
        }

        val basePositionUs = state.positionUs
            .coerceIn(0L, state.durationUs)
        val startTime = SystemClock.elapsedRealtime()

        if (state.playbackStatus != "Playing") {
            val nextProgress = (
                basePositionUs.toDouble() / state.durationUs
                ).toFloat().coerceIn(0f, 1f)
            if (progress != nextProgress) progress = nextProgress
            return@LaunchedEffect
        }

        while (true) {
            val elapsedUs =
                (SystemClock.elapsedRealtime() - startTime) * 1_000L

            val currentPositionUs = (basePositionUs + elapsedUs)
                .coerceAtMost(state.durationUs)
            val nextProgress = (
                currentPositionUs.toDouble() / state.durationUs
                ).toFloat().coerceIn(0f, 1f)
            if (progress != nextProgress) progress = nextProgress

            delay(250)
        }
    }

    LaunchedEffect(state.volume) {
        volume = state.volume.toFloat().coerceIn(0f, 1f)
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Host Media") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "Back",
                        )
                    }
                },
                actions = {
                    IconButton(onClick = { showSystemAudio = true }) {
                        Icon(
                            imageVector = Icons.Filled.VolumeUp,
                            contentDescription = "System audio and output devices",
                        )
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(horizontal = 24.dp, vertical = 16.dp)
                .verticalScroll(rememberScrollState()),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            // The current model contains one player identity.
            // Replace this with a player selector once the backend
            // provides a collection of available players.
            Text(
                text = state.identity,
                style = MaterialTheme.typography.labelLarge,
                color = MaterialTheme.colorScheme.primary,
            )

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(240.dp),
                contentAlignment = Alignment.Center,
            ) {
                val artwork = remember(state.artwork) {
                    state.artwork?.let { bytes ->
                        BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                            ?.asImageBitmap()
                    }
                }

                when {
                    artwork != null -> {
                        Image(
                            bitmap = artwork,
                            modifier = artworkModifier,
                            contentDescription = "Album artwork",
                            contentScale = ContentScale.Crop,
                        )
                    }

                    state.artworkUrl.startsWith("https://") ||
                            state.artworkUrl.startsWith("http://") -> {
                        AsyncImage(
                            model = state.artworkUrl,
                            modifier = artworkModifier,
                            contentDescription = "Album artwork",
                            contentScale = ContentScale.Crop,
                        )
                    }

                    else -> {
                        Icon(
                            imageVector = Icons.Filled.MusicNote,
                            contentDescription = "No album artwork",
                            modifier = Modifier.size(96.dp),
                            tint = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }

            Text(
                text = listOf(state.title, state.artists)
                    .filter { it.isNotBlank() }
                    .joinToString(" • "),
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
                textAlign = TextAlign.Center,
                modifier = Modifier.basicMarquee(
                    iterations = Int.MAX_VALUE,
                    animationMode = MarqueeAnimationMode.Immediately,
                ),
            )

            if (state.error != null) {
                Text(
                    text = state.error.orEmpty(),
                    color = MaterialTheme.colorScheme.error,
                    textAlign = TextAlign.Center,
                    style = MaterialTheme.typography.bodyMedium,
                )
            } else if (!state.available) {
                Text(
                    text = "No active media player was reported by the host.",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }

            Slider(
                value = progress,
                enabled = state.available &&
                        state.canSeek &&
                        state.durationUs > 0L,
                onValueChange = {
                    seeking = true
                    progress = it
                },
                onValueChangeFinished = {
                    val targetUs =
                        (state.durationUs.toDouble() * progress).roundToLong()

                    send(
                        MediaControl(
                            action = MediaActions.SEEK_TO,
                            positionMs = targetUs / 1_000,
                        ),
                    )
                    seeking = false
                },
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Text(
                    formatTime((state.durationUs * progress).toLong()),
                    style = MaterialTheme.typography.labelSmall,
                )
                Text(
                    formatTime(state.durationUs),
                    style = MaterialTheme.typography.labelSmall,
                )
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceEvenly,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IconButton(
                    onClick = {
                        send(
                            MediaControl(
                                MediaActions.SHUFFLE,
                                enabled = !state.shuffle,
                            ),
                        )
                    },
                    enabled = state.available,
                ) {
                    Icon(
                        Icons.Filled.Shuffle,
                        contentDescription = "Shuffle",
                        tint = if (state.shuffle) {
                            MaterialTheme.colorScheme.primary
                        } else {
                            MaterialTheme.colorScheme.onSurface
                        },
                    )
                }

                IconButton(
                    onClick = { send(MediaControl(MediaActions.PREVIOUS)) },
                    enabled = state.available && state.canGoPrevious,
                    modifier = Modifier.size(56.dp),
                ) {
                    Icon(
                        Icons.Filled.SkipPrevious,
                        contentDescription = "Previous",
                        modifier = Modifier.size(36.dp),
                    )
                }

                IconButton(
                    onClick = { send(MediaControl(MediaActions.PLAY_PAUSE)) },
                    enabled = state.available,
                    modifier = Modifier
                        .size(76.dp)
                        .clip(RoundedCornerShape(38.dp))
                        .background(MaterialTheme.colorScheme.primary),
                ) {
                    Icon(
                        imageVector = if (state.playbackStatus == "Playing") {
                            Icons.Filled.Pause
                        } else {
                            Icons.Filled.PlayArrow
                        },
                        contentDescription = "Play or pause",
                        modifier = Modifier.size(42.dp),
                        tint = MaterialTheme.colorScheme.secondary,
                    )
                }

                IconButton(
                    onClick = { send(MediaControl(MediaActions.NEXT)) },
                    enabled = state.available && state.canGoNext,
                    modifier = Modifier.size(56.dp),
                ) {
                    Icon(
                        Icons.Filled.SkipNext,
                        contentDescription = "Next",
                        modifier = Modifier.size(36.dp),
                    )
                }

                IconButton(
                    onClick = {
                        send(
                            MediaControl(
                                MediaActions.LOOP,
                                loopStatus = nextLoop(state.loopStatus),
                            ),
                        )
                    },
                    enabled = state.available,
                ) {
                    Icon(
                        imageVector = if (state.loopStatus == "Track") {
                            Icons.Filled.RepeatOne
                        } else {
                            Icons.Filled.Repeat
                        },
                        contentDescription = "Loop: ${state.loopStatus}",
                        tint = if (state.loopStatus != "None") {
                            MaterialTheme.colorScheme.primary
                        } else {
                            MaterialTheme.colorScheme.onSurface
                        },
                    )
                }
            }

            Text(
                text = "Player volume ${(volume * 100).toInt()}%",
                style = MaterialTheme.typography.labelMedium,
            )

            Slider(
                value = volume,
                enabled = state.available,
                onValueChange = { volume = it },
                onValueChangeFinished = {
                    send(
                        MediaControl(
                            action = MediaActions.VOLUME,
                            volume = volume.toDouble(),
                        ),
                    )
                },
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SystemAudioScreen(
    server: PairingServer,
    state: HostMediaState,
    onBack: () -> Unit,
) {
    val send: (MediaControl) -> Unit = { server.sendMediaControl(it) }

    var showSystemAudio by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        send(MediaControl(MediaActions.SYSTEM_AUDIO_STATE))
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Host media") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "Back",
                        )
                    }
                },
                actions = {
                    IconButton(onClick = { showSystemAudio = true }) {
                        Icon(
                            imageVector = Icons.Filled.VolumeUp,
                            contentDescription = "System audio and output devices",
                        )
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(horizontal = 20.dp, vertical = 12.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                text = "Manage the host computer's audio outputs.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            if (state.outputDevices.isEmpty()) {
                Text(
                    text = "No output devices found. Refresh the list and check that the host audio service is running.",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }

            state.outputDevices.forEach { device ->
                OutputDeviceCard(
                    device = device,
                    onVolume = { newVolume ->
                        send(
                            MediaControl(
                                action = MediaActions.SYSTEM_AUDIO_VOLUME,
                                busName = device.name,
                                volume = newVolume,
                            ),
                        )
                    },
                    onMute = { muted ->
                        send(
                            MediaControl(
                                action = MediaActions.SYSTEM_AUDIO_MUTE,
                                busName = device.name,
                                enabled = muted,
                            ),
                        )
                    },
                    onSetDefault = {
                        send(
                            MediaControl(
                                action = MediaActions.SYSTEM_AUDIO_DEFAULT,
                                busName = device.name,
                            ),
                        )
                    },
                )
            }
        }
    }
}

@Composable
private fun OutputDeviceCard(
    device: AudioOutputDevice,
    onVolume: (Double) -> Unit,
    onMute: (Boolean) -> Unit,
    onSetDefault: () -> Unit,
) {
    var volume by remember(device.name, device.volume) {
        mutableFloatStateOf(device.volume.toFloat().coerceIn(0f, 1f))
    }

    Card(
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(
                    imageVector = if (device.muted) {
                        Icons.Filled.VolumeOff
                    } else {
                        Icons.Filled.Speaker
                    },
                    contentDescription = null,
                    tint = if (device.isDefault) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                )

                Spacer(Modifier.width(12.dp))

                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = device.description,
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        text = if (device.isDefault) {
                            "Default output"
                        } else {
                            "Available output"
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }

                if (device.isDefault) {
                    Icon(
                        Icons.Filled.CheckCircle,
                        contentDescription = "Default device",
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
            }

            Text(
                text = "Volume ${(volume * 100).toInt()}%",
                style = MaterialTheme.typography.labelMedium,
            )

            Slider(
                value = volume,
                onValueChange = { volume = it },
                onValueChangeFinished = {
                    onVolume(volume.toDouble())
                },
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                TextButton(
                    onClick = { onMute(!device.muted) },
                ) {
                    Icon(
                        imageVector = if (device.muted) {
                            Icons.Filled.VolumeUp
                        } else {
                            Icons.Filled.VolumeOff
                        },
                        contentDescription = null,
                    )
                    Spacer(Modifier.width(6.dp))
                    Text(if (device.muted) "Unmute" else "Mute")
                }

                if (!device.isDefault) {
                    TextButton(onClick = onSetDefault) {
                        Text("Set as default")
                    }
                }
            }
        }
    }
}

private fun nextLoop(current: String): String = when (current) {
    "None" -> "Track"
    "Track" -> "Playlist"
    else -> "None"
}

private fun formatTime(microseconds: Long): String {
    val seconds = (microseconds / 1_000_000).coerceAtLeast(0)
    return "%d:%02d".format(seconds / 60, seconds % 60)
}
