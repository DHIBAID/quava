#ifndef QUAVA_APP_H
#define QUAVA_APP_H

#include <gtk/gtk.h>
#include <json-glib/json-glib.h>

#include "ipc.h"

/* ---- Icons (hand-drawn with cairo; see ui/icons.c) -------------------- */

typedef enum {
	Q_ICON_PHONE,
	Q_ICON_MUSIC,
	Q_ICON_SPEAKER,
	Q_ICON_SPEAKER_MUTED,
	Q_ICON_BELL,
	Q_ICON_PLAY,
	Q_ICON_PAUSE,
	Q_ICON_NEXT,
	Q_ICON_PREV,
	Q_ICON_SHUFFLE,
	Q_ICON_REPEAT,
	Q_ICON_REPEAT_ONE,
	Q_ICON_REFRESH,
	Q_ICON_SEARCH,
	Q_ICON_CHECK,
	Q_ICON_CHEVRON,
	Q_ICON_CLIPBOARD,
	Q_ICON_PING,
	Q_ICON_BATTERY,
	Q_ICON_BOLT,
	Q_ICON_WIFI,
	Q_ICON_SIGNAL,
	Q_ICON_BLUETOOTH,
	Q_ICON_GEAR
} QuavaIconKind;

GtkWidget* quava_icon_new(QuavaIconKind kind, gint size);
void quava_icon_set_kind(GtkWidget* icon, QuavaIconKind kind);

/* Fill for level icons: battery 0..100, wifi and signal bars 0..4, -1 = unknown. */
void quava_icon_set_level(GtkWidget* icon, gint level);

/* ---- Application shell ------------------------------------------------- */

typedef struct {
	GtkApplication* gtk;
	GtkWidget* window;
	GtkWidget* stack;
	GtkWidget* toast_revealer;
	GtkWidget* toast_label;
	guint toast_timeout;
	GtkWidget* daemon_dot;
	GtkWidget* daemon_label;
	GtkWidget* progress; /* thin sweep bar under the title bar */
	guint progress_tick;
	guint progress_hide;
	gint64 progress_since;
} App;

void app_toast(App* app, const gchar* message);
/* Show / hide the yellow sweep under the title bar while something loads. */
void app_busy_begin(App* app);
void app_busy_end(App* app);
void app_set_daemon_state(App* app, gboolean reachable, guint connected);

void activate(GtkApplication* application, gpointer user_data);

/* ---- Theme -------------------------------------------------------------- */

void theme_install(GtkWidget* window);

/* ---- Pages --------------------------------------------------------------- */

GtkWidget* page_devices_new(App* app);
GtkWidget* page_media_new(App* app);
GtkWidget* page_audio_new(App* app);
GtkWidget* page_settings_new(App* app);

/* Re-query the device list now (after pairing, connecting, ...). */
void devices_refresh_now(void);

/* Same, but shows the sweep bar until the daemon answers (for the refresh button). */
void devices_refresh_interactive(void);

/* The paired-device list is polled once, by the Devices page, and shared. */
typedef void (*DevicesListener)(JsonObject* reply);

/* Called with each new device-list reply (and once now if one is already known). */
void devices_subscribe(DevicesListener listener);

/* Send a per-device command, toast the result, then refresh. */
void devices_action(const gchar* command, const gchar* id, const gchar* success);

/* Pairing flow: asks the daemon to pair, shows the verification code. */
void quava_pair(App* app, const gchar* peer_id, const gchar* name);

/* ---- Widget helpers ---------------------------------------------------- */

GtkWidget* q_label(const gchar* text, const gchar* css_class);
GtkWidget* q_wrapped_label(const gchar* text, const gchar* css_class);
GtkWidget* q_icon_button(QuavaIconKind kind, const gchar* tooltip, const gchar* css_class);
GtkWidget* q_text_button(const gchar* label, const gchar* css_class);
GtkWidget* q_page_header(const gchar* title, const gchar* subtitle, GtkWidget* action);
GtkWidget* q_empty_state(QuavaIconKind kind, const gchar* title, const gchar* hint);
GtkWidget* q_scrolled(GtkWidget* child);
void q_clear_box(GtkWidget* box);

/* Reads a string member, tolerating missing members and either key casing. */
const gchar* q_json_string(JsonObject* object, const gchar* name, const gchar* fallback);

gboolean q_json_bool(JsonObject* object, const gchar* name);
JsonArray* q_json_array(JsonObject* object, const gchar* name);
JsonObject* q_json_object(JsonObject* object, const gchar* name);
gint64 q_json_int(JsonObject* object, const gchar* name, gint64 fallback);

gchar* q_format_time(gint64 microseconds);

#endif
