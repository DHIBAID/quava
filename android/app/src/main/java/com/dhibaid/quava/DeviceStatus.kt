package com.dhibaid.quava

import android.Manifest
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.wifi.WifiManager
import android.os.BatteryManager
import android.os.Build
import android.os.Environment
import android.os.StatFs
import android.provider.Settings
import android.telephony.SignalStrength
import android.telephony.TelephonyCallback
import android.telephony.TelephonyDisplayInfo
import android.telephony.TelephonyManager
import androidx.annotation.RequiresApi
import androidx.core.content.ContextCompat

/**
 * What the phone tells the computer about itself. Numbers that could not be
 * read are -1, so "no signal" (0) and "unknown" stay different. Signal levels
 * are quantized to 0..4 so ordinary dBm jitter does not cause a new report.
 */
data class DeviceStatus(
    val batteryPercent: Int = -1,
    val batteryState: String = "unknown", // charging, discharging, full, unknown
    val plug: String = "",                // ac, usb, wireless
    val batteryTempC: Double = 0.0,
    val network: String = "none",         // wifi, mobile, ethernet, none
    val wifiLevel: Int = -1,
    val wifiRssi: Int = 0,
    val cellGeneration: String = "",      // 2G, 3G, 4G, 5G
    val cellLevel: Int = -1,
    val bluetooth: Boolean = false,
    val airplane: Boolean = false,
    val storageFree: Long = -1,
    val storageTotal: Long = -1,
    val model: String = Build.MODEL,
    val androidVersion: String = Build.VERSION.RELEASE.orEmpty(),
)

class DeviceStatusCollector(context: Context) {
    private val context = context.applicationContext

    private var onChange: (() -> Unit)? = null

    /** Latest "display info" override, which is how Android tells 5G NSA apart from plain LTE. */
    @Volatile
    private var displayOverride = 0

    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            onChange?.invoke()
        }
    }

    private val networkCallback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) = notifyChange()
        override fun onLost(network: Network) = notifyChange()
        override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) = notifyChange()
    }

    private var signalCallback: Any? = null
    private var displayCallback: Any? = null

    private fun notifyChange() {
        onChange?.invoke()
    }

    /**
     * Starts watching for changes (battery, connectivity, Bluetooth, airplane
     * mode, mobile signal and network type) and calls [changed] when any
     * happens. Readings are still taken with [snapshot]; this only says "look
     * again now" so updates are not stuck waiting for a timer.
     */
    fun start(changed: () -> Unit) {
        stop()
        onChange = changed

        val filter = IntentFilter().apply {
            addAction(Intent.ACTION_BATTERY_CHANGED)
            addAction(BluetoothAdapter.ACTION_STATE_CHANGED)
            addAction(Intent.ACTION_AIRPLANE_MODE_CHANGED)
        }
        ContextCompat.registerReceiver(context, receiver, filter, ContextCompat.RECEIVER_NOT_EXPORTED)

        runCatching {
            context.getSystemService(ConnectivityManager::class.java).registerDefaultNetworkCallback(networkCallback)
        }

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) startTelephonyWatchers()
    }

    fun stop() {
        onChange = null
        runCatching { context.unregisterReceiver(receiver) }
        runCatching { context.getSystemService(ConnectivityManager::class.java).unregisterNetworkCallback(networkCallback) }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) stopTelephonyWatchers()
    }

    @RequiresApi(Build.VERSION_CODES.S)
    private inner class SignalWatcher : TelephonyCallback(), TelephonyCallback.SignalStrengthsListener {
        override fun onSignalStrengthsChanged(signalStrength: SignalStrength) = notifyChange()
    }

    @RequiresApi(Build.VERSION_CODES.S)
    private inner class DisplayWatcher : TelephonyCallback(), TelephonyCallback.DisplayInfoListener {
        override fun onDisplayInfoChanged(telephonyDisplayInfo: TelephonyDisplayInfo) {
            displayOverride = telephonyDisplayInfo.overrideNetworkType
            notifyChange()
        }
    }

    @RequiresApi(Build.VERSION_CODES.S)
    private fun startTelephonyWatchers() {
        val telephony = telephony() ?: return
        val executor = ContextCompat.getMainExecutor(context)

        // Signal bars need no permission; the display info (which says "5G")
        // needs the phone-state permission, so it is tried on its own.
        runCatching {
            SignalWatcher().also {
                telephony.registerTelephonyCallback(executor, it)
                signalCallback = it
            }
        }
        runCatching {
            DisplayWatcher().also {
                telephony.registerTelephonyCallback(executor, it)
                displayCallback = it
            }
        }
    }

    @RequiresApi(Build.VERSION_CODES.S)
    private fun stopTelephonyWatchers() {
        val telephony = telephony() ?: return

        (signalCallback as? TelephonyCallback)?.let { runCatching { telephony.unregisterTelephonyCallback(it) } }
        (displayCallback as? TelephonyCallback)?.let { runCatching { telephony.unregisterTelephonyCallback(it) } }
        signalCallback = null
        displayCallback = null
    }

    fun snapshot(): DeviceStatus {
        val battery = battery()
        val (network, wifiRssi) = activeNetwork()

        return battery.copy(
            network = network,
            wifiLevel = if (wifiRssi != null) wifiLevel(wifiRssi) else -1,
            wifiRssi = wifiRssi ?: 0,
            cellGeneration = cellGeneration(),
            cellLevel = cellLevel(),
            bluetooth = bluetoothOn(),
            airplane = airplaneOn(),
            storageFree = storage { availableBytes },
            storageTotal = storage { totalBytes },
        )
    }

    private fun battery(): DeviceStatus {
        // The battery broadcast is sticky: passing a null receiver reads the
        // latest value without registering anything.
        val intent: Intent = context.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
            ?: return DeviceStatus()

        val level = intent.getIntExtra(BatteryManager.EXTRA_LEVEL, -1)
        val scale = intent.getIntExtra(BatteryManager.EXTRA_SCALE, 100)
        val percent = if (level >= 0 && scale > 0) level * 100 / scale else -1

        val state = when (intent.getIntExtra(BatteryManager.EXTRA_STATUS, -1)) {
            BatteryManager.BATTERY_STATUS_CHARGING -> "charging"
            BatteryManager.BATTERY_STATUS_FULL -> "full"
            BatteryManager.BATTERY_STATUS_DISCHARGING, BatteryManager.BATTERY_STATUS_NOT_CHARGING -> "discharging"
            else -> "unknown"
        }

        val plug = when (intent.getIntExtra(BatteryManager.EXTRA_PLUGGED, 0)) {
            BatteryManager.BATTERY_PLUGGED_AC -> "ac"
            BatteryManager.BATTERY_PLUGGED_USB -> "usb"
            BatteryManager.BATTERY_PLUGGED_WIRELESS -> "wireless"
            else -> ""
        }

        val tenths = intent.getIntExtra(BatteryManager.EXTRA_TEMPERATURE, Int.MIN_VALUE)

        return DeviceStatus(
            batteryPercent = percent,
            batteryState = state,
            plug = plug,
            // Whole degrees: tenths of a degree would trigger a report every few minutes.
            batteryTempC = if (tenths == Int.MIN_VALUE) 0.0 else Math.round(tenths / 10.0).toDouble(),
        )
    }

    /** The kind of network in use, and the Wi-Fi signal in dBm when it is Wi-Fi. */
    private fun activeNetwork(): Pair<String, Int?> {
        return try {
            val manager = context.getSystemService(ConnectivityManager::class.java)
            val capabilities = manager.getNetworkCapabilities(manager.activeNetwork)

            when {
                capabilities == null -> "none" to null
                capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi" to wifiRssi()
                capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "mobile" to null
                capabilities.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet" to null
                else -> "none" to null
            }
        } catch (_: Exception) {
            "none" to null
        }
    }

    @Suppress("DEPRECATION")
    private fun wifiRssi(): Int? = try {
        val manager = context.getSystemService(Context.WIFI_SERVICE) as WifiManager
        manager.connectionInfo?.rssi?.takeIf { it > -127 }
    } catch (_: Exception) {
        null
    }

    private fun wifiLevel(rssi: Int): Int = when {
        rssi >= -55 -> 4
        rssi >= -65 -> 3
        rssi >= -75 -> 2
        rssi >= -85 -> 1
        else -> 0
    }

    private fun telephony(): TelephonyManager? =
        context.getSystemService(Context.TELEPHONY_SERVICE) as? TelephonyManager

    private fun cellGeneration(): String {
        // Reading the data network type needs the phone-state permission on
        // Android 11+; without it the computer just shows "Mobile".
        val allowed = ContextCompat.checkSelfPermission(context, Manifest.permission.READ_PHONE_STATE) ==
                PackageManager.PERMISSION_GRANTED
        if (!allowed) return ""

        return try {
            // 5G NSA looks like LTE to the plain network type; the display info
            // override is what carries the "5G" the status bar shows.
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R &&
                (displayOverride == TelephonyDisplayInfo.OVERRIDE_NETWORK_TYPE_NR_NSA ||
                        displayOverride == TelephonyDisplayInfo.OVERRIDE_NETWORK_TYPE_NR_ADVANCED)
            ) {
                return "5G"
            }

            when (telephony()?.dataNetworkType) {
                TelephonyManager.NETWORK_TYPE_NR -> "5G"
                TelephonyManager.NETWORK_TYPE_LTE, 19 /* LTE_CA */ -> "4G"
                TelephonyManager.NETWORK_TYPE_HSPA, TelephonyManager.NETWORK_TYPE_HSPAP,
                TelephonyManager.NETWORK_TYPE_HSDPA, TelephonyManager.NETWORK_TYPE_HSUPA,
                TelephonyManager.NETWORK_TYPE_UMTS, TelephonyManager.NETWORK_TYPE_EVDO_0,
                TelephonyManager.NETWORK_TYPE_EVDO_A, TelephonyManager.NETWORK_TYPE_EVDO_B,
                TelephonyManager.NETWORK_TYPE_TD_SCDMA -> "3G"
                TelephonyManager.NETWORK_TYPE_GPRS, TelephonyManager.NETWORK_TYPE_EDGE,
                TelephonyManager.NETWORK_TYPE_CDMA, TelephonyManager.NETWORK_TYPE_1xRTT,
                TelephonyManager.NETWORK_TYPE_IDEN -> "2G"
                else -> ""
            }
        } catch (_: SecurityException) {
            ""
        }
    }

    /** Cellular signal bars (0..4) from the radio, even while Wi-Fi is the active network. */
    private fun cellLevel(): Int = try {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) telephony()?.signalStrength?.level ?: -1 else -1
    } catch (_: SecurityException) {
        -1
    }

    private fun bluetoothOn(): Boolean = try {
        (context.getSystemService(Context.BLUETOOTH_SERVICE) as? BluetoothManager)?.adapter?.isEnabled == true
    } catch (_: SecurityException) {
        false
    }

    private fun airplaneOn(): Boolean =
        Settings.Global.getInt(context.contentResolver, Settings.Global.AIRPLANE_MODE_ON, 0) == 1

    private fun storage(read: StatFs.() -> Long): Long = try {
        StatFs(Environment.getDataDirectory().path).read()
    } catch (_: Exception) {
        -1
    }
}
