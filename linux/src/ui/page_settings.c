#include "../includes/app.h"

typedef struct {
	App* app;
	GtkWidget* list;
	gchar* snapshot;
} SettingsPage;

static SettingsPage* page;

static const gchar* SYNC_VALUES[] = {"toHost", "both", "toClient", "none"};
static const gchar* SYNC_COMMANDS[] = {"notif-sync-tohost", "notif-sync-both", "notif-sync-toclient", "notif-sync-stop"};
static const gchar* SYNC_LABELS[] = {"Phone to computer", "Both ways", "Computer to phone", "Off", NULL};

/* ---- Helpers ---------------------------------------------------------------------- */

static void tag(GtkWidget* widget, const gchar* id) {
	g_object_set_data_full(G_OBJECT(widget), "device-id", g_strdup(id), g_free);
}

static const gchar* card_id(gpointer widget) {
	return g_object_get_data(G_OBJECT(widget), "device-id");
}

static GtkWidget* divider(void) {
	GtkWidget* line = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);

	gtk_widget_add_css_class(line, "q-divider");
	return line;
}

static GtkWidget* setting_row(const gchar* title, const gchar* description, GtkWidget* control) {
	GtkWidget* row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
	GtkWidget* text = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);

	gtk_widget_set_hexpand(text, TRUE);
	gtk_widget_set_valign(text, GTK_ALIGN_CENTER);
	gtk_box_append(GTK_BOX(text), q_label(title, NULL));
	gtk_box_append(GTK_BOX(text), q_wrapped_label(description, "q-dim"));
	gtk_box_append(GTK_BOX(row), text);

	gtk_widget_set_valign(control, GTK_ALIGN_CENTER);
	gtk_box_append(GTK_BOX(row), control);

	return row;
}

static GtkWidget* section_label(const gchar* text) {
	GtkWidget* label = q_label(text, "q-section-title");

	gtk_widget_set_margin_top(label, 4);
	return label;
}

static gboolean setting_flag(JsonObject* settings, const gchar* name, gboolean fallback) {
	if (settings == NULL || !json_object_has_member(settings, name))
		return fallback;

	return q_json_bool(settings, name);
}

/* ---- Handlers ------------------------------------------------------------------------ */

static void autoconnect_switched(GObject* widget, GParamSpec* pspec, gpointer data) {
	(void)pspec;
	(void)data;

	gboolean active = gtk_switch_get_active(GTK_SWITCH(widget));

	devices_action(active ? "autoconnect-on" : "autoconnect-off", card_id(widget), active ? "Will connect automatically" : "Won't connect automatically");
}

static void sync_selected(GObject* widget, GParamSpec* pspec, gpointer data) {
	(void)pspec;
	(void)data;

	guint selected = gtk_drop_down_get_selected(GTK_DROP_DOWN(widget));
	if (selected < G_N_ELEMENTS(SYNC_COMMANDS))
		devices_action(SYNC_COMMANDS[selected], card_id(widget), "Notification sync updated");
}

static void clipboard_switched(GObject* widget, GParamSpec* pspec, gpointer data) {
	(void)pspec;
	(void)data;

	gboolean active = gtk_switch_get_active(GTK_SWITCH(widget));

	devices_action(active ? "clipboard-sync-on" : "clipboard-sync-off", card_id(widget), active ? "Clipboard sync is on" : "Clipboard sync is off");
}

static void push_clicked(GtkButton* button, gpointer data) {
	(void)data;
	devices_action("clipboard-push", card_id(button), "Clipboard sent to your phone");
}

static void input_switched(GObject* widget, GParamSpec* pspec, gpointer data) {
	(void)pspec;
	(void)data;

	gboolean active = gtk_switch_get_active(GTK_SWITCH(widget));

	devices_action(active ? "input-enable" : "input-disable", card_id(widget), active ? "Your phone can control this computer" : "Remote input is off");
}

static void input_auto_switched(GObject* widget, GParamSpec* pspec, gpointer data) {
	(void)pspec;
	(void)data;

	gboolean active = gtk_switch_get_active(GTK_SWITCH(widget));

	devices_action(active ? "input-auto-on" : "input-auto-off", card_id(widget), active ? "Remote input will turn on whenever this phone connects" : "Remote input will need turning on each time");
}

static void ping_clicked(GtkButton* button, gpointer data) {
	(void)data;
	devices_action("ping", card_id(button), "Ping sent");
}

static void ring_clicked(GtkButton* button, gpointer data) {
	(void)data;
	devices_action("ring", card_id(button), "Your phone should be ringing");
}

typedef struct {
	gchar* id;
	gchar* name;
} UnpairTarget;

static void unpair_answered(GObject* source, GAsyncResult* result, gpointer data) {
	UnpairTarget* target = data;
	GError* error = NULL;
	int choice = gtk_alert_dialog_choose_finish(GTK_ALERT_DIALOG(source), result, &error);

	if (error != NULL)
		g_clear_error(&error);
	else if (choice == 1)
		devices_action("unpair", target->id, "Device unpaired");

	g_free(target->id);
	g_free(target->name);
	g_free(target);
}

static void unpair_clicked(GtkButton* button, gpointer data) {
	(void)data;

	UnpairTarget* target = g_new0(UnpairTarget, 1);
	target->id = g_strdup(card_id(button));
	target->name = g_strdup(g_object_get_data(G_OBJECT(button), "name"));

	GtkAlertDialog* dialog = gtk_alert_dialog_new("Unpair %s?", target->name);
	const gchar* buttons[] = {"Cancel", "Unpair", NULL};

	gtk_alert_dialog_set_detail(dialog, "This computer and the phone will both forget each other. You'll need to pair again to reconnect.");
	gtk_alert_dialog_set_buttons(dialog, buttons);
	gtk_alert_dialog_set_cancel_button(dialog, 0);
	gtk_alert_dialog_set_default_button(dialog, 0);
	gtk_alert_dialog_choose(dialog, GTK_WINDOW(page->app->window), NULL, unpair_answered, target);
	g_object_unref(dialog);
}

/* ---- Per-device card ------------------------------------------------------------------ */

static GtkWidget* switch_for(const gchar* id, gboolean active, GCallback handler) {
	GtkWidget* toggle = gtk_switch_new();

	gtk_switch_set_active(GTK_SWITCH(toggle), active);
	/* Never let a switch stretch to the height of whatever sits next to it. */
	gtk_widget_set_valign(toggle, GTK_ALIGN_CENTER);
	gtk_widget_set_halign(toggle, GTK_ALIGN_END);
	tag(toggle, id);
	/* Connected after the initial state is set, so building the card sends nothing. */
	g_signal_connect(toggle, "notify::active", handler, NULL);

	return toggle;
}

static GtkWidget* device_settings_card(JsonObject* device, JsonObject* settings, gboolean connected) {
	const gchar* id = q_json_string(device, "peer_device_id", "");
	const gchar* name = q_json_string(device, "peer_device_name", "Unnamed device");

	GtkWidget* card = gtk_box_new(GTK_ORIENTATION_VERTICAL, 14);
	gtk_widget_add_css_class(card, "q-card");

	/* Header */
	GtkWidget* head = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
	GtkWidget* title = q_label(name, "q-title");
	gtk_widget_set_hexpand(title, TRUE);
	gtk_box_append(GTK_BOX(head), title);
	GtkWidget* chip = q_label(connected ? "Connected" : "Offline", "q-chip");
	if (connected)
		gtk_widget_add_css_class(chip, "on");
	gtk_box_append(GTK_BOX(head), chip);
	gtk_box_append(GTK_BOX(card), head);

	/* Connection */
	gtk_box_append(GTK_BOX(card), divider());
	gtk_box_append(GTK_BOX(card), section_label("CONNECTION"));
	gtk_box_append(GTK_BOX(card),
				   setting_row("Connect automatically", "Reconnect whenever this phone is nearby", switch_for(id, setting_flag(settings, "auto_connect", TRUE), G_CALLBACK(autoconnect_switched))));

	/* Sync */
	gtk_box_append(GTK_BOX(card), divider());
	gtk_box_append(GTK_BOX(card), section_label("SYNC"));

	GtkWidget* direction = gtk_drop_down_new_from_strings(SYNC_LABELS);
	const gchar* current = q_json_string(settings, "notification_sync", "toHost");
	guint selected = 0;
	for (guint i = 0; i < G_N_ELEMENTS(SYNC_VALUES); i++)
		if (g_strcmp0(current, SYNC_VALUES[i]) == 0)
			selected = i;
	gtk_drop_down_set_selected(GTK_DROP_DOWN(direction), selected);
	tag(direction, id);
	g_signal_connect(direction, "notify::selected", G_CALLBACK(sync_selected), NULL);
	gtk_box_append(GTK_BOX(card), setting_row("Notifications", "Which way notifications travel", direction));

	GtkWidget* clipboard_controls = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 4);
	GtkWidget* push = q_icon_button(Q_ICON_CLIPBOARD, "Send clipboard now", NULL);
	gtk_widget_set_sensitive(push, connected);
	tag(push, id);
	g_signal_connect(push, "clicked", G_CALLBACK(push_clicked), NULL);
	gtk_box_append(GTK_BOX(clipboard_controls), push);
	GtkWidget* clipboard = switch_for(id, setting_flag(settings, "clipboard_sync", FALSE), G_CALLBACK(clipboard_switched));
	gtk_box_append(GTK_BOX(clipboard_controls), clipboard);
	gtk_box_append(GTK_BOX(card), setting_row("Clipboard sync", "Share copied text automatically", clipboard_controls));

	/* Remote input */
	gtk_box_append(GTK_BOX(card), divider());
	gtk_box_append(GTK_BOX(card), section_label("REMOTE INPUT"));
	gtk_box_append(GTK_BOX(card),
				   setting_row("Remote input", "Let this phone act as a touchpad and keyboard", switch_for(id, setting_flag(settings, "input_enabled", FALSE), G_CALLBACK(input_switched))));
	gtk_box_append(GTK_BOX(card),
				   setting_row("Always allow remote input", "Turn it on automatically whenever this phone connects", switch_for(id, setting_flag(settings, "input_auto", FALSE), G_CALLBACK(input_auto_switched))));

	/* Find my phone */
	gtk_box_append(GTK_BOX(card), divider());
	gtk_box_append(GTK_BOX(card), section_label("FIND MY PHONE"));
	GtkWidget* find = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
	GtkWidget* ping = q_icon_button(Q_ICON_PING, "Ping", NULL);
	GtkWidget* ring = q_icon_button(Q_ICON_BELL, "Make the phone ring", NULL);
	gtk_widget_set_sensitive(ping, connected);
	gtk_widget_set_sensitive(ring, connected);
	tag(ping, id);
	tag(ring, id);
	g_signal_connect(ping, "clicked", G_CALLBACK(ping_clicked), NULL);
	g_signal_connect(ring, "clicked", G_CALLBACK(ring_clicked), NULL);
	gtk_box_append(GTK_BOX(find), ping);
	gtk_box_append(GTK_BOX(find), ring);
	gtk_box_append(GTK_BOX(card), setting_row("Ping or ring", "Needs the phone to be connected", find));

	/* Unpair */
	gtk_box_append(GTK_BOX(card), divider());
	GtkWidget* unpair = q_text_button("Unpair", "danger");
	tag(unpair, id);
	g_object_set_data_full(G_OBJECT(unpair), "name", g_strdup(name), g_free);
	g_signal_connect(unpair, "clicked", G_CALLBACK(unpair_clicked), NULL);
	gtk_box_append(GTK_BOX(card), setting_row("Unpair", "Forget this phone on both devices", unpair));

	return card;
}

/* ---- Page -------------------------------------------------------------------------------- */

/* What this page shows. Battery and signal changes are left out on purpose:
 * they arrive every few seconds and must not rebuild (and so close) the menus. */
static gchar* relevant_snapshot(JsonObject* reply) {
	if (reply == NULL)
		return g_strdup("offline");

	const gchar* members[] = {"devices", "device_settings", "connected_device_ids"};
	GString* text = g_string_new(NULL);

	for (guint i = 0; i < G_N_ELEMENTS(members); i++) {
		if (!json_object_has_member(reply, members[i]))
			continue;

		gchar* part = json_to_string(json_object_get_member(reply, members[i]), FALSE);
		g_string_append(text, part);
		g_free(part);
	}

	return g_string_free(text, FALSE);
}

static void on_devices(JsonObject* reply) {
	gchar* snapshot = relevant_snapshot(reply);
	if (g_strcmp0(snapshot, page->snapshot) == 0) {
		g_free(snapshot);
		return;
	}
	g_free(page->snapshot);
	page->snapshot = snapshot;

	q_clear_box(page->list);

	JsonArray* devices = NULL;
	if (reply != NULL && json_object_has_member(reply, "devices") && JSON_NODE_HOLDS_ARRAY(json_object_get_member(reply, "devices")))
		devices = json_object_get_array_member(reply, "devices");

	if (devices == NULL || json_array_get_length(devices) == 0) {
		gtk_box_append(GTK_BOX(page->list),
					   q_empty_state(Q_ICON_GEAR, "No paired devices", reply == NULL ? "The Quava daemon isn't running." : "Pair a phone from the Devices page first."));
		return;
	}

	JsonObject* settings = q_json_object(reply, "device_settings");
	GHashTable* connected = g_hash_table_new_full(g_str_hash, g_str_equal, g_free, NULL);
	if (json_object_has_member(reply, "connected_device_ids") && JSON_NODE_HOLDS_ARRAY(json_object_get_member(reply, "connected_device_ids"))) {
		JsonArray* ids = json_object_get_array_member(reply, "connected_device_ids");
		for (guint i = 0; i < json_array_get_length(ids); i++) {
			const gchar* id = json_array_get_string_element(ids, i);
			if (id != NULL)
				g_hash_table_add(connected, g_strdup(id));
		}
	}

	for (guint i = 0; i < json_array_get_length(devices); i++) {
		JsonObject* device = json_array_get_object_element(devices, i);
		const gchar* id = q_json_string(device, "peer_device_id", NULL);

		if (device != NULL && id != NULL)
			gtk_box_append(GTK_BOX(page->list), device_settings_card(device, q_json_object(settings, id), g_hash_table_contains(connected, id)));
	}

	g_hash_table_unref(connected);
}

GtkWidget* page_settings_new(App* app) {
	page = g_new0(SettingsPage, 1);
	page->app = app;

	GtkWidget* root = gtk_box_new(GTK_ORIENTATION_VERTICAL, 20);

	gtk_box_append(GTK_BOX(root), q_page_header("Settings", "Options for each paired phone.", NULL));
	page->list = gtk_box_new(GTK_ORIENTATION_VERTICAL, 14);
	gtk_box_append(GTK_BOX(root), page->list);

	/* Rebuilt only when the daemon reports a change, so open menus stay put. */
	devices_subscribe(on_devices);

	return root;
}
