package com.dhibaid.quava

import android.util.Log
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import androidx.core.app.NotificationCompat
import androidx.media.app.NotificationCompat.MediaStyle
import android.support.v4.media.MediaMetadataCompat
import android.support.v4.media.session.MediaSessionCompat
import android.support.v4.media.session.PlaybackStateCompat

/**
 * Publishes the host computer's media as an Android media session.
 *
 * Keep one HostMediaNotification instance for the lifetime of QuavaService.
 * Update its metadata and playback state as host-media messages arrive.
 */
class HostMediaNotification(private val context: Context) {
    private val notificationManager =
        context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager

    private val mediaSession = MediaSessionCompat(context, "QuavaHostMedia").apply {
        setCallback(object : MediaSessionCompat.Callback() {
            // Quava currently exposes a toggle action, so play and pause both
            // forward that action. The host's updated state should then call
            // updatePlaybackState().
            override fun onPlay() =
                dispatchMediaAction(MediaActions.PLAY)

            override fun onPause() =
                dispatchMediaAction(MediaActions.PAUSE)

            override fun onSkipToPrevious() =
                dispatchMediaAction(MediaActions.PREVIOUS)

            override fun onSkipToNext() =
                dispatchMediaAction(MediaActions.NEXT)
        })
        setFlags(
            MediaSessionCompat.FLAG_HANDLES_MEDIA_BUTTONS or
                    MediaSessionCompat.FLAG_HANDLES_TRANSPORT_CONTROLS
        )
    }

    private var title: String = "Host media"
    private var artist: String = "Control media playing on your connected computer"
    private var artwork: Bitmap? = null
    private var isPlaying: Boolean = false

    fun show() {
        mediaSession.isActive = true
        publishPlaybackState()

        val notification = NotificationCompat.Builder(context, QuavaService.MEDIA_CHANNEL)
            .setSmallIcon(android.R.drawable.ic_media_play)
            .setContentTitle(title.ifBlank { "Host media" })
            .setContentText(artist)
            .setContentIntent(openAppIntent())
            .setCategory(NotificationCompat.CATEGORY_TRANSPORT)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setLargeIcon(artwork)
            .addAction(
                android.R.drawable.ic_media_previous,
                "Previous",
                commandIntent(MediaActions.PREVIOUS, 10)
            )
            .addAction(
                if (isPlaying) android.R.drawable.ic_media_pause else android.R.drawable.ic_media_play,
                if (isPlaying) "Pause" else "Play",
                commandIntent(MediaActions.PLAY_PAUSE, 11)
            )
            .addAction(
                android.R.drawable.ic_media_next,
                "Next",
                commandIntent(MediaActions.NEXT, 12)
            )
            .setStyle(
                MediaStyle()
                    .setMediaSession(mediaSession.sessionToken)
                    .setShowActionsInCompactView(0, 1, 2)
            )
            .build()

        notificationManager.notify(QuavaService.MEDIA_NOTIFICATION_ID, notification)
    }

    /**
     * Call when Quava receives new host track metadata.
     * artworkBytes may contain JPEG, PNG, or WebP image data.
     */
    fun updateMetadata(
        trackTitle: String,
        trackArtist: String,
        artworkBytes: ByteArray? = null,
        durationUs: Long = 0L,
    ) {
        title = trackTitle
        artist = trackArtist

        artwork = artworkBytes?.let { bytes ->
            runCatching {
                BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
            }.getOrNull()
        }

        val metadata = MediaMetadataCompat.Builder()
            .putString(MediaMetadataCompat.METADATA_KEY_TITLE, title)
            .putString(MediaMetadataCompat.METADATA_KEY_ARTIST, artist)
            .putString(MediaMetadataCompat.METADATA_KEY_DISPLAY_TITLE, title)
            .putString(MediaMetadataCompat.METADATA_KEY_DISPLAY_SUBTITLE, artist)
            .putLong(MediaMetadataCompat.METADATA_KEY_DURATION, durationUs / 1000L)

        artwork?.let {
            metadata.putBitmap(MediaMetadataCompat.METADATA_KEY_ART, it)
            metadata.putBitmap(MediaMetadataCompat.METADATA_KEY_ALBUM_ART, it)
        }

        Log.d(
            "QuavaMedia",
            "Publishing metadata: title=$title, durationMs=${durationUs / 1000L}"
        )

        mediaSession.setMetadata(metadata.build())
        show()
    }

    /** Call whenever the host's play/pause state changes. */
    private var durationMs: Long = 0L
    private var positionMs: Long = 0L

    fun updatePlaybackState(
        playing: Boolean,
        positionUs: Long = 0L,
        durationUs: Long = 0L,
    ) {
        isPlaying = playing
        positionMs = (positionUs / 1000L).coerceAtLeast(0L)
        durationMs = (durationUs / 1000L).coerceAtLeast(0L)

        publishPlaybackState()
        show()
    }

    fun dismiss() {
        notificationManager.cancel(QuavaService.MEDIA_NOTIFICATION_ID)
        mediaSession.isActive = false
    }

    private fun publishPlaybackState() {
        val actions = PlaybackStateCompat.ACTION_PLAY or
                PlaybackStateCompat.ACTION_PAUSE or
                PlaybackStateCompat.ACTION_PLAY_PAUSE or
                PlaybackStateCompat.ACTION_SKIP_TO_PREVIOUS or
                PlaybackStateCompat.ACTION_SKIP_TO_NEXT

        mediaSession.setPlaybackState(
            PlaybackStateCompat.Builder()
                .setActions(actions)
                .setState(
                    if (isPlaying) PlaybackStateCompat.STATE_PLAYING
                    else PlaybackStateCompat.STATE_PAUSED,
                    positionMs,
                    if (isPlaying) 1.0f else 0.0f,
                )
                .build()
        )
    }

    private fun dispatchMediaAction(action: String) {
        val intent = Intent(context, QuavaService::class.java).apply {
            this.action = QuavaService.ACTION_MEDIA_CONTROL
            putExtra(QuavaService.EXTRA_MEDIA_ACTION, action)
        }
        context.startService(intent)
    }

    private fun commandIntent(action: String, requestCode: Int): PendingIntent =
        PendingIntent.getService(
            context,
            requestCode,
            Intent(context, QuavaService::class.java).apply {
                this.action = QuavaService.ACTION_MEDIA_CONTROL
                putExtra(QuavaService.EXTRA_MEDIA_ACTION, action)
            },
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )

    private fun openAppIntent(): PendingIntent = PendingIntent.getActivity(
        context,
        13,
        Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP
            putExtra(QuavaService.EXTRA_OPEN_MEDIA, true)
        },
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )
}
