#include <string.h>

#include "../includes/app.h"

static gboolean prefers_dark(GtkSettings* settings) {
	/* GTK 4.20 exposes the desktop's light/dark choice directly. */
	if (g_object_class_find_property(G_OBJECT_GET_CLASS(settings), "gtk-interface-color-scheme") != NULL) {
		gint scheme = 0;
		g_object_get(settings, "gtk-interface-color-scheme", &scheme, NULL);
		if (scheme == 2)
			return TRUE;
		if (scheme == 3)
			return FALSE;
	}

	gboolean prefer_dark = FALSE;
	g_object_get(settings, "gtk-application-prefer-dark-theme", &prefer_dark, NULL);
	if (prefer_dark)
		return TRUE;

	gchar* theme = NULL;
	g_object_get(settings, "gtk-theme-name", &theme, NULL);
	gboolean dark = theme != NULL && strstr(theme, "dark") != NULL;
	g_free(theme);

	return dark;
}

static void apply_scheme(GtkWidget* window) {
	gboolean dark = prefers_dark(gtk_settings_get_default());

	gtk_widget_remove_css_class(window, dark ? "light" : "dark");
	gtk_widget_add_css_class(window, dark ? "dark" : "light");
}

static void scheme_changed(GObject* settings, GParamSpec* pspec, gpointer window) {
	(void)settings;
	(void)pspec;

	apply_scheme(GTK_WIDGET(window));
}

void theme_install(GtkWidget* window) {
	GtkCssProvider* provider = gtk_css_provider_new();

	gtk_css_provider_load_from_resource(provider, "/io/quava/Desktop/app.css");
	gtk_style_context_add_provider_for_display(gdk_display_get_default(), GTK_STYLE_PROVIDER(provider), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
	g_object_unref(provider);

	gtk_widget_add_css_class(window, "quava");
	apply_scheme(window);

	GtkSettings* settings = gtk_settings_get_default();
	g_signal_connect_object(settings, "notify::gtk-application-prefer-dark-theme", G_CALLBACK(scheme_changed), window, 0);
	g_signal_connect_object(settings, "notify::gtk-theme-name", G_CALLBACK(scheme_changed), window, 0);
	if (g_object_class_find_property(G_OBJECT_GET_CLASS(settings), "gtk-interface-color-scheme") != NULL)
		g_signal_connect_object(settings, "notify::gtk-interface-color-scheme", G_CALLBACK(scheme_changed), window, 0);
}
