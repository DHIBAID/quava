package com.dhibaid.quava

data class MediaControl(
    val action: String,
    val busName: String = "",
    val positionMs: Long = 0, // absolute target for SEEK
    val enabled: Boolean = false,
    val loopStatus: String = "",
    val volume: Double = 0.0,
)

data class HostMediaState(
    val available: Boolean = false,
    val error: String? = null,
    val playerId: String = "",
    val identity: String = "Host media",
    val title: String = "Nothing is playing",
    val artists: String = "Open a media player on your computer",
    val album: String = "",
    val artworkUrl: String = "",
    val artwork: ByteArray? = null,
    val playbackStatus: String = "Stopped",
    val positionUs: Long = 0,
    val durationUs: Long = 0,
    val volume: Double = 0.5,
    val shuffle: Boolean = false,
    val loopStatus: String = "None",
    val canGoPrevious: Boolean = true,
    val canGoNext: Boolean = true,
    val canSeek: Boolean = true,
    val systemVolume: Double = 0.5,
    val systemMuted: Boolean = false,
    val outputDevice: String = "",
    val outputDevices: List<AudioOutputDevice> = emptyList(),
)

object MediaActions {
    const val STATE = "state"
    const val PLAY_PAUSE = "play-pause"
    const val PLAY = "play"
    const val PAUSE = "pause"
    const val STOP = "stop"
    const val NEXT = "next"
    const val PREVIOUS = "previous"
    const val SEEK = "seek"
    const val SEEK_TO = "seek-to"
    const val SHUFFLE = "shuffle"
    const val LOOP = "loop"
    const val VOLUME = "volume"
    const val SYSTEM_VOLUME = "system-volume"
    const val SYSTEM_MUTE = "system-mute"
    const val SYSTEM_AUDIO_STATE = "system-audio-state"
    const val SYSTEM_AUDIO_VOLUME = "system-audio-volume"
    const val SYSTEM_AUDIO_MUTE = "system-audio-mute"
    const val SYSTEM_AUDIO_DEFAULT = "system-audio-default"
}

data class AudioOutputDevice(
    val name: String,
    val description: String,
    val volume: Double,
    val muted: Boolean,
    val isDefault: Boolean,
)


