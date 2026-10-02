package com.dhibaid.quava

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioManager
import android.media.Ringtone
import android.media.RingtoneManager
import android.os.Handler
import android.os.Looper

class RingController(context: Context) {

    private val appContext = context.applicationContext

    private val audioManager =
        appContext.getSystemService(Context.AUDIO_SERVICE)
                as AudioManager

    private var ringtone: Ringtone? = null
    private var previousVolume: Int? = null

    private val handler = Handler(Looper.getMainLooper())

    private val stopRunnable = Runnable {
        stop()
    }

    fun start() {
        if (ringtone?.isPlaying == true) return

        val uri = RingtoneManager.getDefaultUri(
            RingtoneManager.TYPE_ALARM
        ) ?: return

        val player = RingtoneManager.getRingtone(
            appContext,
            uri
        ) ?: return

        previousVolume = audioManager.getStreamVolume(
            AudioManager.STREAM_ALARM
        )

        try {
            audioManager.setStreamVolume(
                AudioManager.STREAM_ALARM,
                audioManager.getStreamMaxVolume(
                    AudioManager.STREAM_ALARM
                ),
                0
            )

            player.audioAttributes = AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_ALARM)
                .setContentType(
                    AudioAttributes.CONTENT_TYPE_SONIFICATION
                )
                .build()

            player.isLooping = true

            ringtone = player
            player.play()

            handler.postDelayed(stopRunnable, 60_000L)

        } catch (e: Exception) {
            player.stop()
            ringtone = null
            restoreVolume()
        }
    }

    fun stop() {
        handler.removeCallbacks(stopRunnable)

        ringtone?.stop()
        ringtone = null

        restoreVolume()
    }

    private fun restoreVolume() {
        previousVolume?.let { volume ->
            audioManager.setStreamVolume(
                AudioManager.STREAM_ALARM,
                volume,
                0
            )
        }

        previousVolume = null
    }
}