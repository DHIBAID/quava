package com.dhibaid.quava

import android.content.Context
import android.os.Build
import android.telephony.PhoneStateListener
import android.telephony.TelephonyCallback
import android.telephony.TelephonyManager
import androidx.annotation.RequiresApi
import androidx.core.content.ContextCompat

/**
 * Watches the phone's active-call state and reports whether a call is
 * actually connected - answered or dialing out, not just ringing - so the
 * host can mute its own audio for the duration. Fires immediately on each
 * change instead of going through the slower, diffed DeviceStatus loop,
 * since muting late defeats the point.
 */
class CallStateWatcher(context: Context) {
    private val context = context.applicationContext
    private var onChange: ((Boolean) -> Unit)? = null
    private var telephonyCallback: Any? = null

    @Suppress("DEPRECATION")
    private var legacyListener: PhoneStateListener? = null

    fun start(changed: (inCall: Boolean) -> Unit) {
        stop()
        onChange = changed
        val telephony = telephony() ?: return

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            registerCallback(telephony)
        } else {
            registerLegacyListener(telephony)
        }
    }

    fun stop() {
        val telephony = telephony()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            (telephonyCallback as? TelephonyCallback)?.let { callback ->
                telephony?.let { runCatching { it.unregisterTelephonyCallback(callback) } }
            }
            telephonyCallback = null
        } else {
            legacyListener?.let { listener ->
                @Suppress("DEPRECATION")
                telephony?.let { runCatching { it.listen(listener, PhoneStateListener.LISTEN_NONE) } }
            }
            legacyListener = null
        }
        onChange = null
    }

    private fun notify(state: Int) {
        onChange?.invoke(state == TelephonyManager.CALL_STATE_OFFHOOK)
    }

    @RequiresApi(Build.VERSION_CODES.S)
    private inner class CallWatcher : TelephonyCallback(), TelephonyCallback.CallStateListener {
        override fun onCallStateChanged(state: Int) = notify(state)
    }

    @RequiresApi(Build.VERSION_CODES.S)
    private fun registerCallback(telephony: TelephonyManager) {
        runCatching {
            CallWatcher().also {
                telephony.registerTelephonyCallback(ContextCompat.getMainExecutor(context), it)
                telephonyCallback = it
            }
        }
    }

    @Suppress("DEPRECATION")
    private fun registerLegacyListener(telephony: TelephonyManager) {
        runCatching {
            object : PhoneStateListener() {
                override fun onCallStateChanged(state: Int, phoneNumber: String?) = notify(state)
            }.also {
                telephony.listen(it, PhoneStateListener.LISTEN_CALL_STATE)
                legacyListener = it
            }
        }
    }

    private fun telephony(): TelephonyManager? =
        context.getSystemService(Context.TELEPHONY_SERVICE) as? TelephonyManager
}
