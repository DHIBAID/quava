package com.dhibaid.quava

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.net.wifi.WifiManager
import android.util.Log

class MdnsAdvertiser(private val context: Context) {
    private val nsd = context.getSystemService(Context.NSD_SERVICE) as NsdManager
    private val wifi = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
    private var multicastLock: WifiManager.MulticastLock? = null
    private var listener: NsdManager.RegistrationListener? = null

    fun start(port: Int, instanceName: String = "Quava Android") {
        multicastLock = wifi.createMulticastLock("quava-mdns").apply {
            setReferenceCounted(false)
            acquire()
        }

        val info = NsdServiceInfo().apply {
            serviceName = instanceName
            serviceType = "_quava._udp." // NsdManager appends ".local."
            setPort(port)
        }

        listener = object : NsdManager.RegistrationListener {
            override fun onServiceRegistered(i: NsdServiceInfo) {
                Log.d(TAG, "mDNS registered as ${i.serviceName}")
            }

            override fun onRegistrationFailed(i: NsdServiceInfo, code: Int) {
                Log.e(TAG, "mDNS registration failed: $code")
            }

            override fun onServiceUnregistered(i: NsdServiceInfo) {
                Log.d(TAG, "mDNS unregistered")
            }

            override fun onUnregistrationFailed(i: NsdServiceInfo, code: Int) {
                Log.e(TAG, "mDNS unregistration failed: $code")
            }
        }


        nsd.registerService(info, NsdManager.PROTOCOL_DNS_SD, listener)
    }

    fun stop() {
        listener?.let { runCatching { nsd.unregisterService(it) } }
        listener = null
        multicastLock?.takeIf { it.isHeld }?.release()
        multicastLock = null
    }

    companion object {
        private const val TAG = "Quava"
    }
}
