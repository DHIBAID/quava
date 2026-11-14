#include "../includes/app.h"

typedef struct {
	App* app;
	GtkWidget* list;
	gchar* snapshot;
	gint64 hold_until; /* do not rebuild rows while the user is dragging */
	guint poll_timer;
	guint volume_timer;
	gchar* pending_device;
	gdouble pending_volume;
} AudioPage;

static AudioPage* page;

static void poll_now(void);

static void action_done(QuavaIpcReply* reply, gpointer data) {
	(void)data;

	const gchar* error = quava_ipc_error(reply);
	if (error != NULL)
		app_toast(page->app, error);

	g_clear_pointer(&page->snapshot, g_free);
	page->hold_until = 0;
	poll_now();
}

static gboolean send_volume(gpointer data) {
	(void)data;

	page->volume_timer = 0;
	if (page->pending_device == NULL)
		return G_SOURCE_REMOVE;

	JsonObject* request = quava_req_new("audio-volume");
	quava_req_string(request, "device", page->pending_device);
	quava_req_double(request, "volume", page->pending_volume);
	quava_ipc_send(request, action_done, NULL);

	return G_SOURCE_REMOVE;
}

static gboolean volume_changed(GtkRange* range, GtkScrollType scroll, gdouble value, gpointer data) {
	(void)scroll;
	(void)data;

	g_free(page->pending_device);
	page->pending_device = g_strdup(g_object_get_data(G_OBJECT(range), "device"));
	page->pending_volume = CLAMP(value, 0.0, 1.0);
	page->hold_until = g_get_monotonic_time() + 2500 * 1000;

	GtkWidget* label = g_object_get_data(G_OBJECT(range), "percent");
	gchar* text = g_strdup_printf("%d%%", (int)(page->pending_volume * 100 + 0.5));
	gtk_label_set_text(GTK_LABEL(label), text);
	g_free(text);

	if (page->volume_timer != 0)
		g_source_remove(page->volume_timer);
	page->volume_timer = g_timeout_add(150, send_volume, NULL);

	return FALSE;
}

static void mute_clicked(GtkButton* button, gpointer data) {
	(void)data;

	JsonObject* request = quava_req_new("audio-mute");
	quava_req_string(request, "device", g_object_get_data(G_OBJECT(button), "device"));
	quava_req_bool(request, "enabled", !GPOINTER_TO_INT(g_object_get_data(G_OBJECT(button), "muted")));
	quava_ipc_send(request, action_done, NULL);
}

static void make_default(GtkGestureClick* gesture, gint presses, gdouble x, gdouble y, gpointer data) {
	(void)presses;
	(void)x;
	(void)y;
	(void)data;

	GtkWidget* row = gtk_event_controller_get_widget(GTK_EVENT_CONTROLLER(gesture));
	if (GPOINTER_TO_INT(g_object_get_data(G_OBJECT(row), "default")))
		return;

	JsonObject* request = quava_req_new("audio-default");
	quava_req_string(request, "device", g_object_get_data(G_OBJECT(row), "device"));
	quava_ipc_send(request, action_done, NULL);
}

static GtkWidget* device_row(JsonObject* device) {
	const gchar* name = q_json_string(device, "name", "");
	const gchar* description = q_json_string(device, "description", name);
	gboolean is_default = q_json_bool(device, "is_default");
	gboolean muted = q_json_bool(device, "muted");
	gdouble volume = json_object_has_member(device, "volume") ? json_object_get_double_member(device, "volume") : 0;

	GtkWidget* row = gtk_box_new(GTK_ORIENTATION_VERTICAL, 10);
	gtk_widget_add_css_class(row, "q-row");
	if (is_default)
		gtk_widget_add_css_class(row, "default");
	g_object_set_data_full(G_OBJECT(row), "device", g_strdup(name), g_free);
	g_object_set_data(G_OBJECT(row), "default", GINT_TO_POINTER(is_default));

	GtkGesture* click = gtk_gesture_click_new();
	g_signal_connect(click, "pressed", G_CALLBACK(make_default), NULL);
	gtk_widget_add_controller(row, GTK_EVENT_CONTROLLER(click));

	GtkWidget* top = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
	GtkWidget* radio = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
	gtk_widget_add_css_class(radio, "q-radio");
	gtk_widget_set_valign(radio, GTK_ALIGN_CENTER);
	gtk_box_append(GTK_BOX(top), radio);

	GtkWidget* text = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);
	gtk_widget_set_hexpand(text, TRUE);
	GtkWidget* title = q_label(description, "q-title");
	gtk_label_set_ellipsize(GTK_LABEL(title), PANGO_ELLIPSIZE_END);
	gtk_box_append(GTK_BOX(text), title);
	gtk_box_append(GTK_BOX(text), q_label(is_default ? "Default output" : "Tap to use this output", "q-dim"));
	gtk_box_append(GTK_BOX(top), text);

	GtkWidget* mute = q_icon_button(muted ? Q_ICON_SPEAKER_MUTED : Q_ICON_SPEAKER, muted ? "Unmute" : "Mute", muted ? "active" : NULL);
	g_object_set_data_full(G_OBJECT(mute), "device", g_strdup(name), g_free);
	g_object_set_data(G_OBJECT(mute), "muted", GINT_TO_POINTER(muted));
	g_signal_connect(mute, "clicked", G_CALLBACK(mute_clicked), NULL);
	gtk_box_append(GTK_BOX(top), mute);
	gtk_box_append(GTK_BOX(row), top);

	GtkWidget* slider_row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
	GtkWidget* scale = gtk_scale_new_with_range(GTK_ORIENTATION_HORIZONTAL, 0, 1, 0.01);
	gtk_scale_set_draw_value(GTK_SCALE(scale), FALSE);
	gtk_range_set_value(GTK_RANGE(scale), volume);
	gtk_widget_set_hexpand(scale, TRUE);
	gtk_widget_set_margin_start(scale, 32);
	g_object_set_data_full(G_OBJECT(scale), "device", g_strdup(name), g_free);

	gchar* percent = g_strdup_printf("%d%%", (int)(volume * 100 + 0.5));
	GtkWidget* percent_label = q_label(percent, "q-time");
	g_free(percent);
	gtk_widget_set_size_request(percent_label, 40, -1);
	gtk_label_set_xalign(GTK_LABEL(percent_label), 1);
	g_object_set_data(G_OBJECT(scale), "percent", percent_label);
	g_signal_connect(scale, "change-value", G_CALLBACK(volume_changed), NULL);

	gtk_box_append(GTK_BOX(slider_row), scale);
	gtk_box_append(GTK_BOX(slider_row), percent_label);
	gtk_box_append(GTK_BOX(row), slider_row);

	return row;
}

static void on_devices(QuavaIpcReply* reply, gpointer data) {
	(void)data;

	if (g_get_monotonic_time() < page->hold_until)
		return;

	const gchar* error = quava_ipc_error(reply);
	if (error != NULL || !json_object_has_member(reply->object, "audio_devices")) {
		if (g_strcmp0(page->snapshot, "none") != 0) {
			g_free(page->snapshot);
			page->snapshot = g_strdup("none");
			q_clear_box(page->list);
			gtk_box_append(GTK_BOX(page->list), q_empty_state(Q_ICON_SPEAKER, "No audio outputs found", error != NULL ? error : "Check that PipeWire or PulseAudio is running."));
		}
		return;
	}

	JsonNode* node = json_object_get_member(reply->object, "audio_devices");
	gchar* snapshot = json_to_string(node, FALSE);
	if (g_strcmp0(snapshot, page->snapshot) == 0) {
		g_free(snapshot);
		return;
	}
	g_free(page->snapshot);
	page->snapshot = snapshot;

	q_clear_box(page->list);
	JsonArray* devices = json_node_get_array(node);

	/* Default output first, then the rest in the order the host reports. */
	for (int pass = 0; pass < 2; pass++) {
		for (guint i = 0; i < json_array_get_length(devices); i++) {
			JsonObject* device = json_array_get_object_element(devices, i);

			if (device != NULL && q_json_bool(device, "is_default") == (pass == 0))
				gtk_box_append(GTK_BOX(page->list), device_row(device));
		}
	}
}

static void poll_now(void) {
	quava_ipc_send(quava_req_new("audio-list"), on_devices, NULL);
}

static gboolean poll_tick(gpointer data) {
	(void)data;

	poll_now();

	return G_SOURCE_CONTINUE;
}

static void page_mapped(GtkWidget* widget, gpointer data) {
	(void)widget;
	(void)data;

	poll_now();
	if (page->poll_timer == 0)
		page->poll_timer = g_timeout_add_seconds(2, poll_tick, NULL);
}

static void page_unmapped(GtkWidget* widget, gpointer data) {
	(void)widget;
	(void)data;

	if (page->poll_timer != 0) {
		g_source_remove(page->poll_timer);
		page->poll_timer = 0;
	}
}

GtkWidget* page_audio_new(App* app) {
	page = g_new0(AudioPage, 1);
	page->app = app;

	GtkWidget* root = gtk_box_new(GTK_ORIENTATION_VERTICAL, 20);

	gtk_box_append(GTK_BOX(root), q_page_header("Audio", "Choose where this computer plays sound.", NULL));
	page->list = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
	gtk_box_append(GTK_BOX(root), page->list);

	g_signal_connect(root, "map", G_CALLBACK(page_mapped), NULL);
	g_signal_connect(root, "unmap", G_CALLBACK(page_unmapped), NULL);

	return root;
}
