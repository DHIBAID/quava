package com.dhibaid.quava

/** SharedPreferences keys and values for notification forwarding ("quava-notification-settings"). */
object NotificationSettings {
    const val MUTED_APPS = "muted_apps"             // Set<String> of package names
    const val MUTED_CATEGORIES = "muted_categories" // Set<String> of Notification.category values
    const val SEEN_APPS = "seen_apps"               // Set<String> of "package<TAB>label"
    const val SILENT_POLICY = "silent_policy"

    const val SILENT_DROP = "drop"
    const val SILENT_PRESERVE = "preserve"
    const val SILENT_NORMAL = "normal"

    /** Standard Android notification categories, with readable names. */
    val CATEGORIES = listOf(
        "msg" to "Messages",
        "email" to "Email",
        "call" to "Calls",
        "social" to "Social",
        "promo" to "Promotions",
        "recommendation" to "Recommendations",
        "event" to "Calendar events",
        "reminder" to "Reminders",
        "alarm" to "Alarms",
        "progress" to "Progress",
        "service" to "Background services",
        "status" to "Device status",
        "sys" to "System",
        "err" to "Errors",
    )
}
