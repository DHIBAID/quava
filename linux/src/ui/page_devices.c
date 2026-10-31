#include <string.h>

#include "../includes/app.h"

#define MAX_LISTENERS 4

typedef struct {
	App* app;
	GtkWidget* root;
	GtkWidget* paired_box;
	GtkWidget* nearby_box;
	GtkWidget* find_button;
	gchar* snapshot;
	JsonObject* last;
	GHashTable* paired_ids;
	guint timer;
} DevicesPage;

static DevicesPage* page;
static DevicesListener listeners[MAX_LISTENERS];
static guint listener_count;

static gchar* short_id(const gchar* id) {
	return strlen(id) > 12 ? g_strdup_printf("%.6s…%s", id, id + strlen(id) - 4) : g_strdup(id);
}

/* ---- Shared with the Settings page ------------------------------------------------ */

static gboolean refresh_later(gpointer data) {
	(void)data;

	devices_refresh_now();

	return G_SOURCE_REMOVE;
}

static void action_done(QuavaIpcReply* reply, gpointer data) {
	gchar* success = data;
	const gchar* error = quava_ipc_error(reply);

	app_toast(page->app, error != NULL ? error : success);
	g_free(success);

	devices_refresh_now();
	/* Sessions come up asynchronously; look again once they have had time to. */
	g_timeout_add(1500, refresh_later, NULL);
}

void devices_action(const gchar* command, const gchar* id, const gchar* success) {
	JsonObject* request = quava_req_new(command);

	quava_req_string(request, "peer_device_id", id);
	quava_ipc_send(request, action_done, g_strdup(success));
}

void devices_subscribe(DevicesListener listener) {
	if (listener_count < MAX_LISTENERS)
		listeners[listener_count++] = listener;

	if (page != NULL && page->last != NULL)
		listener(page->last);
}

static void notify_listeners(JsonObject* reply) {
	for (guint i = 0; i < listener_count; i++)
		listeners[i](reply);
}

/* ---- Phone status ----------------------------------------------------------------------- */

static GtkWidget* stat_pill(GtkWidget* icon, const gchar* text, const gchar* css_class) {
	GtkWidget* pill = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 7);

	gtk_widget_add_css_class(pill, "q-stat");
	if (css_class != NULL)
		gtk_widget_add_css_class(pill, css_class);
	if (icon != NULL)
		gtk_box_append(GTK_BOX(pill), icon);
	gtk_box_append(GTK_BOX(pill), q_label(text, NULL));

	return pill;
}

static const gchar* level_word(gint64 level) {
	switch (level) {
	case 4:
		return "Excellent";
	case 3:
		return "Good";
	case 2:
		return "Fair";
	case 1:
		return "Weak";
	case 0:
		return "Very weak";
	default:
		return "";
	}
}

static GtkWidget* battery_pill(JsonObject* status) {
	gint64 percent = q_json_int(status, "battery_percent", -1);
	const gchar* state = q_json_string(status, "battery_state", "unknown");
	gboolean charging = g_strcmp0(state, "charging") == 0;
	gboolean full = g_strcmp0(state, "full") == 0;

	GtkWidget* icons = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
	GtkWidget* battery = quava_icon_new(Q_ICON_BATTERY, 22);
	quava_icon_set_level(battery, (gint)percent);
	gtk_box_append(GTK_BOX(icons), battery);
	if (charging) {
		GtkWidget* bolt = quava_icon_new(Q_ICON_BOLT, 14);
		gtk_widget_set_margin_start(bolt, 2);
		gtk_box_append(GTK_BOX(icons), bolt);
	}

	gchar* text;
	if (percent < 0)
		text = g_strdup("Battery unknown");
	else if (charging)
		text = g_strdup_printf("%" G_GINT64_FORMAT "%% · charging", percent);
	else if (full)
		text = g_strdup_printf("%" G_GINT64_FORMAT "%% · full", percent);
	else
		text = g_strdup_printf("%" G_GINT64_FORMAT "%%", percent);

	const gchar* css = NULL;
	if (charging || full)
		css = "good";
	else if (percent >= 0 && percent <= 15)
		css = "low";

	GtkWidget* pill = stat_pill(icons, text, css);
	g_free(text);

	return pill;
}

static void append_network_pills(GtkWidget* flow, JsonObject* status) {
	const gchar* network = q_json_string(status, "network", "none");
	const gchar* generation = q_json_string(status, "cell_generation", "");
	gint64 wifi_level = q_json_int(status, "wifi_level", -1);
	gint64 cell_level = q_json_int(status, "cell_level", -1);

	if (q_json_bool(status, "airplane")) {
		gtk_flow_box_append(GTK_FLOW_BOX(flow), stat_pill(NULL, "Airplane mode", "muted"));
		return;
	}

	if (g_strcmp0(network, "wifi") == 0) {
		GtkWidget* icon = quava_icon_new(Q_ICON_WIFI, 20);
		quava_icon_set_level(icon, (gint)wifi_level);

		gchar* text = wifi_level >= 0 ? g_strdup_printf("Wi-Fi · %s", level_word(wifi_level)) : g_strdup("Wi-Fi");
		gtk_flow_box_append(GTK_FLOW_BOX(flow), stat_pill(icon, text, NULL));
		g_free(text);
	} else if (g_strcmp0(network, "ethernet") == 0) {
		gtk_flow_box_append(GTK_FLOW_BOX(flow), stat_pill(NULL, "Ethernet", NULL));
	} else if (g_strcmp0(network, "none") == 0) {
		gtk_flow_box_append(GTK_FLOW_BOX(flow), stat_pill(NULL, "No network", "muted"));
	}

	/* The mobile radio is worth showing even while Wi-Fi carries the traffic. */
	if (cell_level >= 0 || *generation != '\0') {
		GtkWidget* icon = quava_icon_new(Q_ICON_SIGNAL, 20);
		quava_icon_set_level(icon, (gint)cell_level);

		gtk_flow_box_append(GTK_FLOW_BOX(flow), stat_pill(icon, *generation != '\0' ? generation : "Mobile", NULL));
	}
}

static GtkWidget* status_strip(JsonObject* status) {
	GtkWidget* flow = gtk_flow_box_new();

	gtk_flow_box_set_selection_mode(GTK_FLOW_BOX(flow), GTK_SELECTION_NONE);
	gtk_flow_box_set_homogeneous(GTK_FLOW_BOX(flow), FALSE);
	gtk_flow_box_set_column_spacing(GTK_FLOW_BOX(flow), 8);
	gtk_flow_box_set_row_spacing(GTK_FLOW_BOX(flow), 8);
	gtk_flow_box_set_max_children_per_line(GTK_FLOW_BOX(flow), 6);
	gtk_widget_set_halign(flow, GTK_ALIGN_START);

	gtk_flow_box_append(GTK_FLOW_BOX(flow), battery_pill(status));
	append_network_pills(flow, status);

	gboolean bluetooth = q_json_bool(status, "bluetooth");
	gtk_flow_box_append(GTK_FLOW_BOX(flow),
						stat_pill(quava_icon_new(Q_ICON_BLUETOOTH, 18), bluetooth ? "Bluetooth on" : "Bluetooth off", bluetooth ? NULL : "muted"));

	gint64 free_bytes = q_json_int(status, "storage_free", -1);
	if (free_bytes >= 0 && q_json_int(status, "storage_total", -1) > 0) {
		gchar* size = g_format_size((guint64)free_bytes);
		gchar* text = g_strdup_printf("%s free", size);
		gtk_flow_box_append(GTK_FLOW_BOX(flow), stat_pill(NULL, text, NULL));
		g_free(text);
		g_free(size);
	}

	return flow;
}

/* ---- Card --------------------------------------------------------------------------------- */

static void connect_clicked(GtkButton* button, gpointer data) {
	(void)data;

	gboolean connected = GPOINTER_TO_INT(g_object_get_data(G_OBJECT(button), "connected"));

	devices_action(connected ? "disconnect" : "connect", g_object_get_data(G_OBJECT(button), "device-id"),
				   connected ? "Disconnected" : "Connecting…");
}

static GtkWidget* device_card(JsonObject* device, JsonObject* status, gboolean connected) {
	const gchar* id = q_json_string(device, "peer_device_id", "");
	const gchar* name = q_json_string(device, "peer_device_name", "Unnamed device");

	GtkWidget* card = gtk_box_new(GTK_ORIENTATION_VERTICAL, 16);
	gtk_widget_add_css_class(card, "q-card");
	if (connected)
		gtk_widget_add_css_class(card, "connected");

	GtkWidget* top = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 14);
	GtkWidget* avatar = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
	gtk_widget_add_css_class(avatar, "q-avatar");
	gtk_box_append(GTK_BOX(avatar), quava_icon_new(Q_ICON_PHONE, 24));
	gtk_widget_set_valign(avatar, GTK_ALIGN_CENTER);
	gtk_widget_set_halign(avatar, GTK_ALIGN_CENTER);
	gtk_widget_set_hexpand(avatar, FALSE);
	gtk_widget_set_vexpand(avatar, FALSE);
	gtk_widget_set_size_request(avatar, 46, 46);
	gtk_widget_set_vexpand(gtk_widget_get_first_child(avatar), TRUE);
	gtk_box_append(GTK_BOX(top), avatar);

	GtkWidget* text = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
	gtk_widget_set_hexpand(text, TRUE);
	gtk_widget_set_valign(text, GTK_ALIGN_CENTER);
	GtkWidget* title = q_label(name, "q-title");
	gtk_label_set_ellipsize(GTK_LABEL(title), PANGO_ELLIPSIZE_END);
	gtk_box_append(GTK_BOX(text), title);

	GtkWidget* meta = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
	GtkWidget* chip = q_label(connected ? "Connected" : "Offline", "q-chip");
	if (connected)
		gtk_widget_add_css_class(chip, "on");
	gtk_box_append(GTK_BOX(meta), chip);

	const gchar* model = q_json_string(status, "model", NULL);
	if (model != NULL) {
		const gchar* version = q_json_string(status, "android_version", NULL);
		gchar* line = version != NULL ? g_strdup_printf("%s · Android %s", model, version) : g_strdup(model);
		gtk_box_append(GTK_BOX(meta), q_label(line, "q-dim"));
		g_free(line);
	} else {
		gchar* short_identifier = short_id(id);
		gtk_box_append(GTK_BOX(meta), q_label(short_identifier, "q-mono"));
		g_free(short_identifier);
	}
	gtk_box_append(GTK_BOX(text), meta);
	gtk_box_append(GTK_BOX(top), text);

	GtkWidget* connect = q_text_button(connected ? "Disconnect" : "Connect", connected ? NULL : "primary");
	g_object_set_data(G_OBJECT(connect), "connected", GINT_TO_POINTER(connected));
	g_object_set_data_full(G_OBJECT(connect), "device-id", g_strdup(id), g_free);
	g_signal_connect(connect, "clicked", G_CALLBACK(connect_clicked), NULL);
	gtk_box_append(GTK_BOX(top), connect);
	gtk_box_append(GTK_BOX(card), top);

	if (connected) {
		GtkWidget* divider = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
		gtk_widget_add_css_class(divider, "q-divider");
		gtk_box_append(GTK_BOX(card), divider);

		if (status != NULL)
			gtk_box_append(GTK_BOX(card), status_strip(status));
		else
			gtk_box_append(GTK_BOX(card), q_label("Waiting for the phone to report its status…", "q-dim"));
	}

	return card;
}

/* ---- Device list -------------------------------------------------------------------------- */

static void on_devices(QuavaIpcReply* reply, gpointer data) {
	if (GPOINTER_TO_INT(data))
		app_busy_end(page->app);

	const gchar* error = quava_ipc_error(reply);
	if (error != NULL) {
		app_set_daemon_state(page->app, FALSE, 0);

		if (g_strcmp0(page->snapshot, "offline") != 0) {
			g_free(page->snapshot);
			page->snapshot = g_strdup("offline");
			g_clear_pointer(&page->last, json_object_unref);
			q_clear_box(page->paired_box);
			gtk_box_append(GTK_BOX(page->paired_box),
						   q_empty_state(Q_ICON_PHONE, "Can't reach the Quava daemon",
										 "Start it with: systemctl --user start quavad"));
			notify_listeners(NULL);
		}
		return;
	}

	JsonArray* connected_ids = q_json_array(reply->object, "connected_device_ids");
	app_set_daemon_state(page->app, TRUE, connected_ids != NULL ? json_array_get_length(connected_ids) : 0);

	/* Rebuild only when something changed, so open menus are not torn down. */
	JsonNode* node = json_node_new(JSON_NODE_OBJECT);
	json_node_set_object(node, reply->object);
	gchar* snapshot = json_to_string(node, FALSE);
	json_node_free(node);

	if (g_strcmp0(snapshot, page->snapshot) == 0) {
		g_free(snapshot);
		return;
	}
	g_free(page->snapshot);
	page->snapshot = snapshot;

	g_clear_pointer(&page->last, json_object_unref);
	page->last = json_object_ref(reply->object);

	GHashTable* connected = g_hash_table_new_full(g_str_hash, g_str_equal, g_free, NULL);
	for (guint i = 0; connected_ids != NULL && i < json_array_get_length(connected_ids); i++) {
		const gchar* id = json_array_get_string_element(connected_ids, i);
		if (id != NULL)
			g_hash_table_add(connected, g_strdup(id));
	}

	g_hash_table_remove_all(page->paired_ids);
	q_clear_box(page->paired_box);

	JsonArray* devices = q_json_array(reply->object, "devices");
	JsonObject* statuses = q_json_object(reply->object, "device_status");

	for (guint i = 0; devices != NULL && i < json_array_get_length(devices); i++) {
		JsonObject* device = json_array_get_object_element(devices, i);
		const gchar* id = q_json_string(device, "peer_device_id", NULL);

		if (device == NULL || id == NULL)
			continue;

		g_hash_table_add(page->paired_ids, g_strdup(id));
		gtk_box_append(GTK_BOX(page->paired_box),
					   device_card(device, q_json_object(statuses, id), g_hash_table_contains(connected, id)));
	}

	if (g_hash_table_size(page->paired_ids) == 0)
		gtk_box_append(GTK_BOX(page->paired_box),
					   q_empty_state(Q_ICON_PHONE, "No paired devices yet",
									 "Open Quava on your phone, then use Find devices below."));

	g_hash_table_unref(connected);
	notify_listeners(page->last);
}

void devices_refresh_now(void) {
	if (page == NULL)
		return;

	quava_ipc_send(quava_req_new("devices"), on_devices, GINT_TO_POINTER(FALSE));
}

void devices_refresh_interactive(void) {
	if (page == NULL)
		return;

	app_busy_begin(page->app);

	/* A refresh should always show something, even if nothing changed. */
	g_clear_pointer(&page->snapshot, g_free);
	quava_ipc_send(quava_req_new("devices"), on_devices, GINT_TO_POINTER(TRUE));
}

/* ---- Nearby ---------------------------------------------------------------------------- */

static void pair_clicked(GtkButton* button, gpointer data) {
	(void)data;

	quava_pair(page->app, g_object_get_data(G_OBJECT(button), "device-id"), g_object_get_data(G_OBJECT(button), "name"));
}

static GtkWidget* nearby_row(JsonObject* device) {
	const gchar* id = q_json_string(device, "peer_device_id", q_json_string(device, "PeerDeviceID", ""));
	const gchar* name = q_json_string(device, "Name", q_json_string(device, "name", "Unnamed device"));
	const gchar* address = q_json_string(device, "Address", "");

	GtkWidget* row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
	GtkWidget* text = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);

	gtk_widget_add_css_class(row, "q-row");
	gtk_widget_set_hexpand(text, TRUE);
	gtk_widget_set_valign(text, GTK_ALIGN_CENTER);
	gtk_box_append(GTK_BOX(text), q_label(name, "q-title"));
	gtk_box_append(GTK_BOX(text), q_label(*address != '\0' ? address : id, "q-mono"));
	gtk_box_append(GTK_BOX(row), text);

	if (g_hash_table_contains(page->paired_ids, id)) {
		gtk_box_append(GTK_BOX(row), q_label("Already paired", "q-chip"));
	} else if (*id != '\0') {
		GtkWidget* pair = q_text_button("Pair", "primary");

		g_object_set_data_full(G_OBJECT(pair), "device-id", g_strdup(id), g_free);
		g_object_set_data_full(G_OBJECT(pair), "name", g_strdup(name), g_free);
		g_signal_connect(pair, "clicked", G_CALLBACK(pair_clicked), NULL);
		gtk_box_append(GTK_BOX(row), pair);
	}

	return row;
}

static void on_discovered(QuavaIpcReply* reply, gpointer data) {
	(void)data;

	const gchar* error = quava_ipc_error(reply);
	q_clear_box(page->nearby_box);
	gtk_widget_set_sensitive(page->find_button, TRUE);

	if (error != NULL) {
		gtk_box_append(GTK_BOX(page->nearby_box), q_empty_state(Q_ICON_SEARCH, "Search failed", error));
		return;
	}

	JsonArray* found = q_json_array(reply->object, "discovered");
	if (found == NULL || json_array_get_length(found) == 0) {
		gtk_box_append(GTK_BOX(page->nearby_box),
					   q_empty_state(Q_ICON_SEARCH, "Nothing found",
									 "Make sure Quava is open on your phone and both are on the same network."));
		return;
	}

	for (guint i = 0; i < json_array_get_length(found); i++) {
		JsonObject* device = json_array_get_object_element(found, i);
		if (device != NULL)
			gtk_box_append(GTK_BOX(page->nearby_box), nearby_row(device));
	}
}

static void find_clicked(GtkButton* button, gpointer data) {
	(void)button;
	(void)data;

	q_clear_box(page->nearby_box);

	GtkWidget* searching = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
	GtkWidget* spinner = gtk_spinner_new();
	gtk_widget_set_halign(searching, GTK_ALIGN_CENTER);
	gtk_widget_set_margin_top(searching, 20);
	gtk_widget_set_margin_bottom(searching, 20);
	gtk_spinner_start(GTK_SPINNER(spinner));
	gtk_box_append(GTK_BOX(searching), spinner);
	gtk_box_append(GTK_BOX(searching), q_label("Looking on your network…", "q-body"));
	gtk_box_append(GTK_BOX(page->nearby_box), searching);
	gtk_widget_set_sensitive(page->find_button, FALSE);

	JsonObject* request = quava_req_new("discover");
	quava_req_int(request, "timeout_ms", 8000);
	quava_ipc_send(request, on_discovered, NULL);
}

/* ---- Lifecycle -------------------------------------------------------------------------- */

static gboolean poll_devices(gpointer data) {
	(void)data;

	devices_refresh_now();

	return G_SOURCE_CONTINUE;
}

static void page_mapped(GtkWidget* widget, gpointer data) {
	(void)widget;
	(void)data;

	devices_refresh_now();

	/* Keep polling on every page: the sidebar's connection pill depends on it. */
	if (page->timer == 0)
		page->timer = g_timeout_add_seconds(4, poll_devices, NULL);
}

GtkWidget* page_devices_new(App* app) {
	page = g_new0(DevicesPage, 1);
	page->app = app;
	page->paired_ids = g_hash_table_new_full(g_str_hash, g_str_equal, g_free, NULL);

	GtkWidget* root = gtk_box_new(GTK_ORIENTATION_VERTICAL, 22);
	page->root = root;

	gtk_box_append(GTK_BOX(root), q_page_header("Devices", "Phones paired with this computer.", NULL));

	page->paired_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 14);
	gtk_box_append(GTK_BOX(root), page->paired_box);

	page->find_button = q_text_button("Find devices", "primary");
	g_signal_connect(page->find_button, "clicked", G_CALLBACK(find_clicked), NULL);

	GtkWidget* nearby_head = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
	GtkWidget* nearby_title = q_label("NEARBY", "q-section-title");
	gtk_widget_set_hexpand(nearby_title, TRUE);
	gtk_widget_set_valign(nearby_title, GTK_ALIGN_CENTER);
	gtk_box_append(GTK_BOX(nearby_head), nearby_title);
	gtk_box_append(GTK_BOX(nearby_head), page->find_button);
	gtk_box_append(GTK_BOX(root), nearby_head);

	page->nearby_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 10);
	gtk_box_append(GTK_BOX(page->nearby_box),
				   q_empty_state(Q_ICON_SEARCH, "Find a new phone", "Open Quava on it, then search this network."));
	gtk_box_append(GTK_BOX(root), page->nearby_box);

	g_signal_connect(root, "map", G_CALLBACK(page_mapped), NULL);

	return root;
}
