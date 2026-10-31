#include <string.h>

#include "../includes/app.h"

typedef struct {
	App* app;
	GtkWidget* root;
	GtkWidget* body;          /* GtkStack: "player" / "empty" */
	GtkWidget* player_button; /* GtkMenuButton */
	GtkWidget* player_label;
	GtkWidget* player_menu;
	GtkWidget* art_stack;
	GtkWidget* art_picture;
	GtkWidget* title;
	GtkWidget* subtitle;
	GtkWidget* seek;
	GtkWidget* time_now;
	GtkWidget* time_total;
	GtkWidget* shuffle;
	GtkWidget* previous;
	GtkWidget* play;
	GtkWidget* play_icon;
	GtkWidget* next;
	GtkWidget* repeat;
	GtkWidget* repeat_icon;
	GtkWidget* volume;

	gchar* bus_name;
	gchar* art_url;
	gchar* loop_status;
	gboolean shuffle_on;
	gboolean updating;
	gint64 duration_us;
	gint64 hold_seek_until; /* ignore polled positions after the user seeks */
	gint64 hold_volume_until;
	guint seek_timer;
	guint volume_timer;
	guint poll_timer;
	guint polls;
} MediaPage;

static MediaPage* page;

/* ---- Commands --------------------------------------------------------------- */

static void poll_now(void);

static void command_done(QuavaIpcReply* reply, gpointer data) {
	(void)data;

	const gchar* error = quava_ipc_error(reply);
	if (error != NULL)
		app_toast(page->app, error);

	poll_now();
}

static void send_media(const gchar* command, JsonObject* request) {
	if (request == NULL)
		request = quava_req_new(command);
	if (page->bus_name != NULL)
		quava_req_string(request, "bus_name", page->bus_name);

	quava_ipc_send(request, command_done, NULL);
}

static void play_clicked(GtkButton* button, gpointer data) {
	(void)button;
	(void)data;
	send_media("media-play-pause", NULL);
}

static void next_clicked(GtkButton* button, gpointer data) {
	(void)button;
	(void)data;
	send_media("media-next", NULL);
}

static void previous_clicked(GtkButton* button, gpointer data) {
	(void)button;
	(void)data;
	send_media("media-previous", NULL);
}

static void shuffle_clicked(GtkButton* button, gpointer data) {
	(void)button;
	(void)data;

	JsonObject* request = quava_req_new("media-shuffle");
	quava_req_bool(request, "enabled", !page->shuffle_on);
	send_media("media-shuffle", request);
}

static void repeat_clicked(GtkButton* button, gpointer data) {
	(void)button;
	(void)data;

	/* Same cycle as the Android client: none -> track -> playlist. */
	const gchar* next = "Track";
	if (g_strcmp0(page->loop_status, "Track") == 0)
		next = "Playlist";
	else if (g_strcmp0(page->loop_status, "Playlist") == 0)
		next = "None";

	JsonObject* request = quava_req_new("media-loop");
	quava_req_string(request, "loop_status", next);
	send_media("media-loop", request);
}

static gboolean send_seek(gpointer data) {
	(void)data;

	page->seek_timer = 0;
	JsonObject* request = quava_req_new("media-seek-to");
	quava_req_int(request, "position_ms", (gint64)gtk_range_get_value(GTK_RANGE(page->seek)));
	send_media("media-seek-to", request);

	return G_SOURCE_REMOVE;
}

static gboolean seek_changed(GtkRange* range, GtkScrollType scroll, gdouble value, gpointer data) {
	(void)range;
	(void)scroll;
	(void)data;

	if (page->updating)
		return FALSE;

	page->hold_seek_until = g_get_monotonic_time() + 1500 * 1000;

	gchar* text = q_format_time((gint64)value * 1000);
	gtk_label_set_text(GTK_LABEL(page->time_now), text);
	g_free(text);

	/* Dragging fires continuously; only send once the pointer settles. */
	if (page->seek_timer != 0)
		g_source_remove(page->seek_timer);
	page->seek_timer = g_timeout_add(140, send_seek, NULL);

	return FALSE;
}

static gboolean send_volume(gpointer data) {
	(void)data;

	page->volume_timer = 0;
	JsonObject* request = quava_req_new("media-volume");
	quava_req_double(request, "volume", gtk_range_get_value(GTK_RANGE(page->volume)));
	send_media("media-volume", request);

	return G_SOURCE_REMOVE;
}

static gboolean volume_changed(GtkRange* range, GtkScrollType scroll, gdouble value, gpointer data) {
	(void)range;
	(void)scroll;
	(void)value;
	(void)data;

	if (page->updating)
		return FALSE;

	page->hold_volume_until = g_get_monotonic_time() + 1500 * 1000;
	if (page->volume_timer != 0)
		g_source_remove(page->volume_timer);
	page->volume_timer = g_timeout_add(140, send_volume, NULL);

	return FALSE;
}

/* ---- Artwork ------------------------------------------------------------------ */

static void show_art(GdkTexture* texture) {
	if (texture != NULL) {
		gtk_picture_set_paintable(GTK_PICTURE(page->art_picture), GDK_PAINTABLE(texture));
		gtk_stack_set_visible_child_name(GTK_STACK(page->art_stack), "picture");
	} else {
		gtk_stack_set_visible_child_name(GTK_STACK(page->art_stack), "placeholder");
	}
}

static void art_loaded(GObject* source, GAsyncResult* result, gpointer data) {
	gchar* url = data;
	GBytes* bytes = g_file_load_bytes_finish(G_FILE(source), result, NULL, NULL);

	/* Ignore a slow download if the track has changed meanwhile. */
	if (bytes != NULL && g_strcmp0(url, page->art_url) == 0) {
		GdkTexture* texture = gdk_texture_new_from_bytes(bytes, NULL);
		show_art(texture);
		if (texture != NULL)
			g_object_unref(texture);
	}

	if (bytes != NULL)
		g_bytes_unref(bytes);
	g_free(url);
}

static void load_art(const gchar* url) {
	if (g_strcmp0(url, page->art_url) == 0)
		return;

	g_free(page->art_url);
	page->art_url = g_strdup(url);

	if (url == NULL || *url == '\0') {
		show_art(NULL);
		return;
	}

	GFile* file = g_file_new_for_uri(url);

	if (g_str_has_prefix(url, "file://")) {
		GdkTexture* texture = gdk_texture_new_from_file(file, NULL);
		show_art(texture);
		if (texture != NULL)
			g_object_unref(texture);
	} else {
		show_art(NULL);
		g_file_load_bytes_async(file, NULL, art_loaded, g_strdup(url));
	}

	g_object_unref(file);
}

/* ---- State ----------------------------------------------------------------------- */

static void set_toggle_class(GtkWidget* widget, gboolean on) {
	if (on)
		gtk_widget_add_css_class(widget, "active");
	else
		gtk_widget_remove_css_class(widget, "active");
}

static void apply_player(JsonObject* player) {
	JsonObject* metadata = NULL;
	if (json_object_has_member(player, "metadata") && JSON_NODE_HOLDS_OBJECT(json_object_get_member(player, "metadata")))
		metadata = json_object_get_object_member(player, "metadata");

	g_free(page->bus_name);
	page->bus_name = g_strdup(q_json_string(player, "bus_name", NULL));

	gtk_label_set_text(GTK_LABEL(page->player_label), q_json_string(player, "identity", "Media player"));
	gtk_label_set_text(GTK_LABEL(page->title), q_json_string(metadata, "title", "Unknown title"));

	GString* line = g_string_new(NULL);
	if (metadata != NULL && json_object_has_member(metadata, "artists") &&
		JSON_NODE_HOLDS_ARRAY(json_object_get_member(metadata, "artists"))) {
		JsonArray* artists = json_object_get_array_member(metadata, "artists");
		for (guint i = 0; i < json_array_get_length(artists); i++) {
			if (line->len > 0)
				g_string_append(line, ", ");
			g_string_append(line, json_array_get_string_element(artists, i));
		}
	}
	const gchar* album = q_json_string(metadata, "album", NULL);
	if (album != NULL) {
		if (line->len > 0)
			g_string_append(line, "  ·  ");
		g_string_append(line, album);
	}
	gtk_label_set_text(GTK_LABEL(page->subtitle), line->len > 0 ? line->str : "Unknown artist");
	g_string_free(line, TRUE);

	load_art(q_json_string(metadata, "artwork_url", NULL));

	gboolean playing = g_strcmp0(q_json_string(player, "playback_status", ""), "Playing") == 0;
	quava_icon_set_kind(page->play_icon, playing ? Q_ICON_PAUSE : Q_ICON_PLAY);

	gboolean can_seek = q_json_bool(player, "can_seek");
	gtk_widget_set_sensitive(page->previous, q_json_bool(player, "can_go_previous"));
	gtk_widget_set_sensitive(page->next, q_json_bool(player, "can_go_next"));

	page->shuffle_on = q_json_bool(player, "shuffle");
	set_toggle_class(page->shuffle, page->shuffle_on);

	g_free(page->loop_status);
	page->loop_status = g_strdup(q_json_string(player, "loop_status", "None"));
	quava_icon_set_kind(page->repeat_icon, g_strcmp0(page->loop_status, "Track") == 0 ? Q_ICON_REPEAT_ONE : Q_ICON_REPEAT);
	set_toggle_class(page->repeat, g_strcmp0(page->loop_status, "None") != 0);

	gint64 now = g_get_monotonic_time();
	gint64 duration = metadata != NULL && json_object_has_member(metadata, "duration")
						  ? json_object_get_int_member(metadata, "duration")
						  : 0;
	gint64 position = json_object_has_member(player, "position") ? json_object_get_int_member(player, "position") : 0;
	page->duration_us = duration;

	page->updating = TRUE;
	gtk_widget_set_sensitive(page->seek, can_seek && duration > 0);
	gtk_range_set_range(GTK_RANGE(page->seek), 0, duration > 0 ? (double)duration / 1000.0 : 1);
	if (now >= page->hold_seek_until) {
		gtk_range_set_value(GTK_RANGE(page->seek), (double)position / 1000.0);

		gchar* current = q_format_time(position);
		gtk_label_set_text(GTK_LABEL(page->time_now), current);
		g_free(current);
	}
	gchar* total = q_format_time(duration);
	gtk_label_set_text(GTK_LABEL(page->time_total), total);
	g_free(total);

	if (now >= page->hold_volume_until && json_object_has_member(player, "volume"))
		gtk_range_set_value(GTK_RANGE(page->volume), json_object_get_double_member(player, "volume"));
	page->updating = FALSE;

	gtk_stack_set_visible_child_name(GTK_STACK(page->body), "player");
}

static void on_active(QuavaIpcReply* reply, gpointer data) {
	(void)data;

	const gchar* error = quava_ipc_error(reply);
	if (error != NULL || !json_object_has_member(reply->object, "player")) {
		g_clear_pointer(&page->bus_name, g_free);
		gtk_stack_set_visible_child_name(GTK_STACK(page->body), "empty");
		gtk_label_set_text(GTK_LABEL(page->player_label), "No player");
		return;
	}

	apply_player(json_object_get_object_member(reply->object, "player"));
}

/* ---- Player menu ----------------------------------------------------------------------- */

static void player_chosen(GtkButton* button, gpointer data) {
	(void)data;

	const gchar* bus = g_object_get_data(G_OBJECT(button), "bus");
	JsonObject* request = quava_req_new(bus != NULL ? "media-set-active" : "media-auto");

	if (bus != NULL)
		quava_req_string(request, "bus_name", bus);

	gtk_menu_button_popdown(GTK_MENU_BUTTON(page->player_button));
	quava_ipc_send(request, command_done, NULL);
}

static GtkWidget* menu_item(const gchar* title, const gchar* subtitle, const gchar* bus, gboolean selected) {
	GtkWidget* button = gtk_button_new();
	GtkWidget* row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
	GtkWidget* text = gtk_box_new(GTK_ORIENTATION_VERTICAL, 1);

	gtk_widget_add_css_class(button, "q-icon-button");
	gtk_widget_set_size_request(button, 220, 44);
	gtk_widget_set_hexpand(text, TRUE);
	gtk_box_append(GTK_BOX(text), q_label(title, NULL));
	if (subtitle != NULL)
		gtk_box_append(GTK_BOX(text), q_label(subtitle, "q-dim"));
	gtk_box_append(GTK_BOX(row), text);
	if (selected)
		gtk_box_append(GTK_BOX(row), quava_icon_new(Q_ICON_CHECK, 16));
	gtk_widget_set_margin_start(row, 10);
	gtk_widget_set_margin_end(row, 10);
	gtk_button_set_child(GTK_BUTTON(button), row);

	if (bus != NULL)
		g_object_set_data_full(G_OBJECT(button), "bus", g_strdup(bus), g_free);
	g_signal_connect(button, "clicked", G_CALLBACK(player_chosen), NULL);

	return button;
}

static void on_list(QuavaIpcReply* reply, gpointer data) {
	(void)data;

	q_clear_box(page->player_menu);
	gtk_box_append(GTK_BOX(page->player_menu), menu_item("Automatic", "Follow whatever is playing", NULL, FALSE));

	if (quava_ipc_error(reply) != NULL || !json_object_has_member(reply->object, "players"))
		return;

	JsonArray* players = json_object_get_array_member(reply->object, "players");
	for (guint i = 0; i < json_array_get_length(players); i++) {
		JsonObject* player = json_array_get_object_element(players, i);
		if (player == NULL)
			continue;

		gboolean active = json_object_has_member(player, "is_active") && json_object_get_boolean_member(player, "is_active");
		gtk_box_append(GTK_BOX(page->player_menu),
					   menu_item(q_json_string(player, "identity", "Player"), q_json_string(player, "playback_status", NULL),
								 q_json_string(player, "bus_name", NULL), active));
	}
}

/* ---- Polling ---------------------------------------------------------------------------- */

static void poll_now(void) {
	quava_ipc_send(quava_req_new("media-active"), on_active, NULL);
	if (page->polls++ % 4 == 0)
		quava_ipc_send(quava_req_new("media-list"), on_list, NULL);
}

static gboolean poll_tick(gpointer data) {
	(void)data;

	poll_now();

	return G_SOURCE_CONTINUE;
}

static void page_mapped(GtkWidget* widget, gpointer data) {
	(void)widget;
	(void)data;

	page->polls = 0;
	poll_now();
	if (page->poll_timer == 0)
		page->poll_timer = g_timeout_add_seconds(1, poll_tick, NULL);
}

static void page_unmapped(GtkWidget* widget, gpointer data) {
	(void)widget;
	(void)data;

	if (page->poll_timer != 0) {
		g_source_remove(page->poll_timer);
		page->poll_timer = 0;
	}
}

/* ---- Layout ------------------------------------------------------------------------------- */

static GtkWidget* control_button(QuavaIconKind kind, const gchar* tooltip, GCallback callback, GtkWidget** icon_out) {
	GtkWidget* button = q_icon_button(kind, tooltip, NULL);

	if (icon_out != NULL)
		*icon_out = gtk_button_get_child(GTK_BUTTON(button));
	g_signal_connect(button, "clicked", callback, NULL);

	return button;
}

GtkWidget* page_media_new(App* app) {
	page = g_new0(MediaPage, 1);
	page->app = app;

	GtkWidget* root = gtk_box_new(GTK_ORIENTATION_VERTICAL, 20);
	page->root = root;

	/* Player picker */
	page->player_button = gtk_menu_button_new();
	GtkWidget* picker = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
	page->player_label = q_label("No player", NULL);
	gtk_box_append(GTK_BOX(picker), page->player_label);
	gtk_box_append(GTK_BOX(picker), quava_icon_new(Q_ICON_CHEVRON, 16));
	gtk_menu_button_set_child(GTK_MENU_BUTTON(page->player_button), picker);
	gtk_widget_add_css_class(page->player_button, "q-button");
	gtk_widget_set_valign(page->player_button, GTK_ALIGN_CENTER);

	GtkWidget* popover = gtk_popover_new();
	page->player_menu = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);
	gtk_popover_set_child(GTK_POPOVER(popover), page->player_menu);
	gtk_menu_button_set_popover(GTK_MENU_BUTTON(page->player_button), popover);

	gtk_box_append(GTK_BOX(root), q_page_header("Media", "Control what's playing on this computer.", page->player_button));

	/* Body: player / empty */
	page->body = gtk_stack_new();
	gtk_stack_set_transition_type(GTK_STACK(page->body), GTK_STACK_TRANSITION_TYPE_CROSSFADE);

	GtkWidget* player = gtk_box_new(GTK_ORIENTATION_VERTICAL, 18);

	page->art_stack = gtk_stack_new();
	gtk_widget_add_css_class(page->art_stack, "q-art");
	gtk_widget_set_halign(page->art_stack, GTK_ALIGN_CENTER);
	gtk_widget_set_overflow(page->art_stack, GTK_OVERFLOW_HIDDEN);
	page->art_picture = gtk_picture_new();
	gtk_picture_set_content_fit(GTK_PICTURE(page->art_picture), GTK_CONTENT_FIT_COVER);
	gtk_stack_add_named(GTK_STACK(page->art_stack), page->art_picture, "picture");
	gtk_stack_add_named(GTK_STACK(page->art_stack), quava_icon_new(Q_ICON_MUSIC, 84), "placeholder");
	gtk_stack_set_visible_child_name(GTK_STACK(page->art_stack), "placeholder");
	gtk_box_append(GTK_BOX(player), page->art_stack);

	page->title = q_label("", "q-track-title");
	gtk_label_set_xalign(GTK_LABEL(page->title), 0.5);
	gtk_label_set_ellipsize(GTK_LABEL(page->title), PANGO_ELLIPSIZE_END);
	page->subtitle = q_label("", "q-body");
	gtk_label_set_xalign(GTK_LABEL(page->subtitle), 0.5);
	gtk_label_set_ellipsize(GTK_LABEL(page->subtitle), PANGO_ELLIPSIZE_END);
	gtk_box_append(GTK_BOX(player), page->title);
	gtk_box_append(GTK_BOX(player), page->subtitle);

	GtkWidget* seek_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);
	page->seek = gtk_scale_new_with_range(GTK_ORIENTATION_HORIZONTAL, 0, 1, 1);
	gtk_scale_set_draw_value(GTK_SCALE(page->seek), FALSE);
	g_signal_connect(page->seek, "change-value", G_CALLBACK(seek_changed), NULL);
	gtk_box_append(GTK_BOX(seek_box), page->seek);

	GtkWidget* times = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
	page->time_now = q_label("0:00", "q-time");
	page->time_total = q_label("0:00", "q-time");
	gtk_widget_set_hexpand(page->time_now, TRUE);
	gtk_box_append(GTK_BOX(times), page->time_now);
	gtk_box_append(GTK_BOX(times), page->time_total);
	gtk_box_append(GTK_BOX(seek_box), times);
	gtk_box_append(GTK_BOX(player), seek_box);

	GtkWidget* controls = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
	gtk_widget_set_halign(controls, GTK_ALIGN_CENTER);
	page->shuffle = control_button(Q_ICON_SHUFFLE, "Shuffle", G_CALLBACK(shuffle_clicked), NULL);
	page->previous = control_button(Q_ICON_PREV, "Previous", G_CALLBACK(previous_clicked), NULL);

	page->play = gtk_button_new();
	page->play_icon = quava_icon_new(Q_ICON_PLAY, 28);
	gtk_button_set_child(GTK_BUTTON(page->play), page->play_icon);
	gtk_widget_add_css_class(page->play, "q-play");
	gtk_widget_set_tooltip_text(page->play, "Play / pause");
	g_signal_connect(page->play, "clicked", G_CALLBACK(play_clicked), NULL);

	page->next = control_button(Q_ICON_NEXT, "Next", G_CALLBACK(next_clicked), NULL);
	page->repeat = control_button(Q_ICON_REPEAT, "Repeat", G_CALLBACK(repeat_clicked), &page->repeat_icon);
	gtk_box_append(GTK_BOX(controls), page->shuffle);
	gtk_box_append(GTK_BOX(controls), page->previous);
	gtk_box_append(GTK_BOX(controls), page->play);
	gtk_box_append(GTK_BOX(controls), page->next);
	gtk_box_append(GTK_BOX(controls), page->repeat);
	gtk_box_append(GTK_BOX(player), controls);

	GtkWidget* volume_row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
	gtk_widget_set_margin_top(volume_row, 6);
	gtk_box_append(GTK_BOX(volume_row), quava_icon_new(Q_ICON_SPEAKER, 20));
	page->volume = gtk_scale_new_with_range(GTK_ORIENTATION_HORIZONTAL, 0, 1, 0.01);
	gtk_scale_set_draw_value(GTK_SCALE(page->volume), FALSE);
	gtk_widget_set_hexpand(page->volume, TRUE);
	g_signal_connect(page->volume, "change-value", G_CALLBACK(volume_changed), NULL);
	gtk_box_append(GTK_BOX(volume_row), page->volume);
	gtk_box_append(GTK_BOX(player), volume_row);

	gtk_stack_add_named(GTK_STACK(page->body), player, "player");
	gtk_stack_add_named(GTK_STACK(page->body), q_empty_state(Q_ICON_MUSIC, "Nothing is playing", "Start a song or video on this computer and it will show up here."), "empty");
	gtk_stack_set_visible_child_name(GTK_STACK(page->body), "empty");
	gtk_box_append(GTK_BOX(root), page->body);

	g_signal_connect(root, "map", G_CALLBACK(page_mapped), NULL);
	g_signal_connect(root, "unmap", G_CALLBACK(page_unmapped), NULL);

	return root;
}
