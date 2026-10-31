#include <math.h>

#include "../includes/app.h"

typedef struct {
	const gchar* id;
	const gchar* title;
	QuavaIconKind icon;
} NavItem;

static const NavItem NAV[] = {
	{"devices", "Devices", Q_ICON_PHONE},
	{"media", "Media", Q_ICON_MUSIC},
	{"audio", "Audio", Q_ICON_SPEAKER},
	{"settings", "Settings", Q_ICON_GEAR},
};

static gboolean toast_expired(gpointer data) {
	App* app = data;

	app->toast_timeout = 0;
	gtk_revealer_set_reveal_child(GTK_REVEALER(app->toast_revealer), FALSE);

	return G_SOURCE_REMOVE;
}

void app_toast(App* app, const gchar* message) {
	if (app == NULL || message == NULL)
		return;

	/* Errors come in lowercase ("cannot reach quavad: ...") from every layer;
	 * capitalize here, once, so a toast always reads as a sentence. */
	gchar* sentence = g_strdup(message);
	if (*sentence != '\0') {
		gchar* first = g_utf8_strup(sentence, g_utf8_next_char(sentence) - sentence);
		gchar* rest = g_strdup(g_utf8_next_char(sentence));
		g_free(sentence);
		sentence = g_strconcat(first, rest, NULL);
		g_free(first);
		g_free(rest);
	}
	gtk_label_set_text(GTK_LABEL(app->toast_label), sentence);
	g_free(sentence);
	gtk_revealer_set_reveal_child(GTK_REVEALER(app->toast_revealer), TRUE);

	if (app->toast_timeout != 0)
		g_source_remove(app->toast_timeout);
	app->toast_timeout = g_timeout_add_seconds(4, toast_expired, app);
}

void app_set_daemon_state(App* app, gboolean reachable, guint connected) {
	if (app == NULL || app->daemon_label == NULL)
		return;

	gchar* text;
	if (!reachable)
		text = g_strdup("Daemon offline");
	else if (connected == 0)
		text = g_strdup("Nothing connected");
	else
		text = g_strdup_printf("%u connected", connected);

	gtk_label_set_text(GTK_LABEL(app->daemon_label), text);
	g_free(text);

	gtk_widget_remove_css_class(app->daemon_dot, "ok");
	gtk_widget_remove_css_class(app->daemon_dot, "bad");
	if (!reachable)
		gtk_widget_add_css_class(app->daemon_dot, "bad");
	else if (connected > 0)
		gtk_widget_add_css_class(app->daemon_dot, "ok");
}

static void nav_toggled(GtkToggleButton* toggle, gpointer data) {
	App* app = data;

	if (gtk_toggle_button_get_active(toggle))
		gtk_stack_set_visible_child_name(GTK_STACK(app->stack), NAV[GPOINTER_TO_UINT(g_object_get_data(G_OBJECT(toggle), "page"))].id);
}

static GtkWidget* nav_button(App* app, const NavItem* item, GtkToggleButton* group) {
	GtkWidget* button = gtk_toggle_button_new();
	GtkWidget* row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);

	gtk_box_append(GTK_BOX(row), quava_icon_new(item->icon, 20));
	gtk_box_append(GTK_BOX(row), q_label(item->title, NULL));
	gtk_button_set_child(GTK_BUTTON(button), row);
	gtk_widget_add_css_class(button, "q-nav");
	g_object_set_data(G_OBJECT(button), "page", GUINT_TO_POINTER((guint)(item - NAV)));

	if (group != NULL)
		gtk_toggle_button_set_group(GTK_TOGGLE_BUTTON(button), group);

	g_signal_connect(button, "toggled", G_CALLBACK(nav_toggled), app);

	return button;
}

/* ---- Sweep bar ------------------------------------------------------------------ */

#define SWEEP_PERIOD_US (1100 * 1000)
#define SWEEP_MIN_VISIBLE_US (650 * 1000)

static void progress_draw(GtkDrawingArea* area, cairo_t* cr, int width, int height, gpointer data) {
	App* app = data;
	GdkRGBA color;
	gint64 elapsed = g_get_monotonic_time() - app->progress_since;
	double phase = (double)(elapsed % SWEEP_PERIOD_US) / SWEEP_PERIOD_US; /* 0..1, left to right */
	double span = width * 0.38;
	double head = -span + phase * (width + span);
	/* A soft pulse on top of the sweep makes it read as "working". */
	double pulse = 0.7 + 0.3 * sin((double)elapsed / 1e6 * 2 * G_PI * 2.2);

	gtk_widget_get_color(GTK_WIDGET(area), &color);

	cairo_pattern_t* gradient = cairo_pattern_create_linear(head, 0, head + span, 0);
	cairo_pattern_add_color_stop_rgba(gradient, 0.0, color.red, color.green, color.blue, 0.0);
	cairo_pattern_add_color_stop_rgba(gradient, 0.7, color.red, color.green, color.blue, pulse);
	cairo_pattern_add_color_stop_rgba(gradient, 1.0, color.red, color.green, color.blue, 0.0);
	cairo_set_source(cr, gradient);
	cairo_rectangle(cr, 0, 0, width, height);
	cairo_fill(cr);
	cairo_pattern_destroy(gradient);
}

static gboolean progress_animate(GtkWidget* widget, GdkFrameClock* clock, gpointer data) {
	(void)clock;
	(void)data;

	gtk_widget_queue_draw(widget);

	return G_SOURCE_CONTINUE;
}

static gboolean progress_hide_now(gpointer data) {
	App* app = data;

	app->progress_hide = 0;
	if (app->progress_tick != 0) {
		gtk_widget_remove_tick_callback(app->progress, app->progress_tick);
		app->progress_tick = 0;
	}
	gtk_widget_set_visible(app->progress, FALSE);

	return G_SOURCE_REMOVE;
}

void app_busy_begin(App* app) {
	if (app == NULL || app->progress == NULL)
		return;

	if (app->progress_hide != 0) {
		g_source_remove(app->progress_hide);
		app->progress_hide = 0;
	}
	if (app->progress_tick != 0)
		return; /* already sweeping */

	app->progress_since = g_get_monotonic_time();
	gtk_widget_set_visible(app->progress, TRUE);
	app->progress_tick = gtk_widget_add_tick_callback(app->progress, progress_animate, NULL, NULL);
}

void app_busy_end(App* app) {
	if (app == NULL || app->progress == NULL || app->progress_tick == 0 || app->progress_hide != 0)
		return;

	/* Quick answers would otherwise flash by unseen; let the bar finish a sweep. */
	gint64 remaining = SWEEP_MIN_VISIBLE_US - (g_get_monotonic_time() - app->progress_since);
	if (remaining <= 0)
		progress_hide_now(app);
	else
		app->progress_hide = g_timeout_add((guint)(remaining / 1000) + 1, progress_hide_now, app);
}

static void refresh_clicked(GtkButton* button, gpointer data) {
	(void)button;
	(void)data;

	devices_refresh_interactive();
}

static GtkWidget* build_title(void) {
	return q_label("QUAVA", "q-wordmark");
}

static GtkWidget* build_sidebar(App* app) {
	GtkWidget* sidebar = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
	GtkToggleButton* first = NULL;

	gtk_widget_add_css_class(sidebar, "q-sidebar");
	gtk_widget_set_size_request(sidebar, 214, -1);

	for (guint i = 0; i < G_N_ELEMENTS(NAV); i++) {
		GtkWidget* button = nav_button(app, &NAV[i], first);

		if (first == NULL) {
			first = GTK_TOGGLE_BUTTON(button);
			gtk_toggle_button_set_active(first, TRUE);
		}
		gtk_box_append(GTK_BOX(sidebar), button);
	}

	GtkWidget* spacer = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
	gtk_widget_set_vexpand(spacer, TRUE);
	gtk_box_append(GTK_BOX(sidebar), spacer);

	GtkWidget* pill = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
	gtk_widget_add_css_class(pill, "q-daemon");
	app->daemon_dot = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
	gtk_widget_add_css_class(app->daemon_dot, "q-daemon-dot");
	gtk_widget_set_valign(app->daemon_dot, GTK_ALIGN_CENTER);
	app->daemon_label = q_label("Connecting…", "q-body");
	gtk_box_append(GTK_BOX(pill), app->daemon_dot);
	gtk_box_append(GTK_BOX(pill), app->daemon_label);
	gtk_box_append(GTK_BOX(sidebar), pill);

	return sidebar;
}

/* GTK has no max-width; keep the content column readable on wide windows. */
static gboolean clamp_tick(GtkWidget* scroll, GdkFrameClock* clock, gpointer data) {
	(void)clock;

	GtkWidget* column = data;
	gint available = gtk_widget_get_width(scroll) - 56;
	gint target = CLAMP(available, 420, 720);

	if (target != GPOINTER_TO_INT(g_object_get_data(G_OBJECT(column), "q-width"))) {
		g_object_set_data(G_OBJECT(column), "q-width", GINT_TO_POINTER(target));
		gtk_widget_set_size_request(column, target, -1);
	}

	return G_SOURCE_CONTINUE;
}

static GtkWidget* page_frame(GtkWidget* page) {
	GtkWidget* column = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
	GtkWidget* holder = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);

	gtk_widget_set_halign(column, GTK_ALIGN_CENTER);
	gtk_widget_set_hexpand(column, TRUE);
	gtk_widget_set_margin_top(column, 28);
	gtk_widget_set_margin_bottom(column, 28);
	gtk_widget_set_margin_start(column, 28);
	gtk_widget_set_margin_end(column, 28);
	gtk_box_append(GTK_BOX(column), page);
	gtk_box_append(GTK_BOX(holder), column);

	GtkWidget* scroll = q_scrolled(holder);
	gtk_widget_add_tick_callback(scroll, clamp_tick, column, NULL);

	return scroll;
}

void activate(GtkApplication* application, gpointer user_data) {
	(void)user_data;

	App* app = g_new0(App, 1);
	app->gtk = application;

	app->window = gtk_application_window_new(application);
	gtk_window_set_title(GTK_WINDOW(app->window), "Quava");
	gtk_window_set_default_size(GTK_WINDOW(app->window), 1080, 740);
	gtk_widget_set_size_request(app->window, 760, 520);
	theme_install(app->window);

	/* The icon is bundled in the binary; the launcher entry (make install)
	 * uses the same name so the dock and the window agree. */
	gtk_icon_theme_add_resource_path(gtk_icon_theme_get_for_display(gdk_display_get_default()), "/io/quava/Desktop/icons");
	gtk_window_set_icon_name(GTK_WINDOW(app->window), "io.quava.Desktop");

	/* Title bar */
	GtkWidget* header = gtk_header_bar_new();
	gtk_header_bar_set_title_widget(GTK_HEADER_BAR(header), build_title());
	GtkWidget* refresh = q_icon_button(Q_ICON_REFRESH, "Refresh devices", NULL);
	g_signal_connect(refresh, "clicked", G_CALLBACK(refresh_clicked), NULL);
	gtk_header_bar_pack_end(GTK_HEADER_BAR(header), refresh);
	gtk_window_set_titlebar(GTK_WINDOW(app->window), header);

	/* Pages */
	app->progress = gtk_drawing_area_new();
	gtk_widget_add_css_class(app->progress, "q-progress");
	gtk_widget_set_size_request(app->progress, -1, 3);
	gtk_widget_set_valign(app->progress, GTK_ALIGN_START);
	gtk_widget_set_can_target(app->progress, FALSE);
	gtk_widget_set_visible(app->progress, FALSE);
	gtk_drawing_area_set_draw_func(GTK_DRAWING_AREA(app->progress), progress_draw, app, NULL);

	app->stack = gtk_stack_new();
	gtk_stack_set_transition_type(GTK_STACK(app->stack), GTK_STACK_TRANSITION_TYPE_CROSSFADE);
	gtk_stack_set_transition_duration(GTK_STACK(app->stack), 140);
	gtk_widget_set_hexpand(app->stack, TRUE);

	gtk_stack_add_named(GTK_STACK(app->stack), page_frame(page_devices_new(app)), "devices");
	gtk_stack_add_named(GTK_STACK(app->stack), page_frame(page_media_new(app)), "media");
	gtk_stack_add_named(GTK_STACK(app->stack), page_frame(page_audio_new(app)), "audio");
	gtk_stack_add_named(GTK_STACK(app->stack), page_frame(page_settings_new(app)), "settings");

	/* Toast overlay */
	GtkWidget* overlay = gtk_overlay_new();
	gtk_overlay_set_child(GTK_OVERLAY(overlay), app->stack);
	gtk_overlay_add_overlay(GTK_OVERLAY(overlay), app->progress);

	app->toast_label = gtk_label_new(NULL);
	gtk_widget_add_css_class(app->toast_label, "q-toast");
	app->toast_revealer = gtk_revealer_new();
	gtk_revealer_set_transition_type(GTK_REVEALER(app->toast_revealer), GTK_REVEALER_TRANSITION_TYPE_SLIDE_UP);
	gtk_revealer_set_child(GTK_REVEALER(app->toast_revealer), app->toast_label);
	gtk_widget_set_halign(app->toast_revealer, GTK_ALIGN_CENTER);
	gtk_widget_set_valign(app->toast_revealer, GTK_ALIGN_END);
	gtk_widget_set_margin_bottom(app->toast_revealer, 24);
	gtk_overlay_add_overlay(GTK_OVERLAY(overlay), app->toast_revealer);

	GtkWidget* body = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
	gtk_box_append(GTK_BOX(body), build_sidebar(app));
	gtk_box_append(GTK_BOX(body), overlay);
	gtk_window_set_child(GTK_WINDOW(app->window), body);

	gtk_window_present(GTK_WINDOW(app->window));
}
