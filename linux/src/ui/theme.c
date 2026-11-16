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
	if (dark)
		return TRUE;

	/* On GTK < 4.20 (e.g. the AppImage's bundled runtime), a "name:dark"
	 * GTK_THEME (as set by linuxdeploy-plugin-gtk's apprun hook) picks the
	 * dark variant of the named theme without touching gtk-theme-name or
	 * gtk-application-prefer-dark-theme at all, so neither check above can
	 * see it. Read the raw variant suffix directly as a last resort. */
	const gchar* env_theme = g_getenv("GTK_THEME");
	if (env_theme != NULL) {
		const gchar* variant = strchr(env_theme, ':');
		if (variant != NULL && strcmp(variant + 1, "dark") == 0)
			return TRUE;
	}

	return FALSE;
}

/* The stylesheets are plain literal colours (no CSS custom properties / `var()`),
 * since that syntax only works on GTK 4.15+ and the AppImage bundles whatever
 * GTK ships with the CI runner's distro, which can be older. Switching themes
 * means swapping which provider is attached, not toggling a CSS class. */
static GtkCssProvider* g_active_provider = NULL;

static void apply_scheme(GtkWidget* window) {
	(void)window;
	gboolean dark = prefers_dark(gtk_settings_get_default());

	GtkCssProvider* provider = gtk_css_provider_new();
	gtk_css_provider_load_from_resource(provider, dark ? "/io/quava/Desktop/app-dark.css" : "/io/quava/Desktop/app-light.css");

	GdkDisplay* display = gdk_display_get_default();
	if (g_active_provider != NULL) {
		gtk_style_context_remove_provider_for_display(display, GTK_STYLE_PROVIDER(g_active_provider));
		g_object_unref(g_active_provider);
	}
	gtk_style_context_add_provider_for_display(display, GTK_STYLE_PROVIDER(provider), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
	g_active_provider = provider;
}

static void scheme_changed(GObject* settings, GParamSpec* pspec, gpointer window) {
	(void)settings;
	(void)pspec;

	apply_scheme(GTK_WIDGET(window));
}

void theme_install(GtkWidget* window) {
	gtk_widget_add_css_class(window, "quava");
	apply_scheme(window);

	GtkSettings* settings = gtk_settings_get_default();
	g_signal_connect_object(settings, "notify::gtk-application-prefer-dark-theme", G_CALLBACK(scheme_changed), window, 0);
	g_signal_connect_object(settings, "notify::gtk-theme-name", G_CALLBACK(scheme_changed), window, 0);
	if (g_object_class_find_property(G_OBJECT_GET_CLASS(settings), "gtk-interface-color-scheme") != NULL)
		g_signal_connect_object(settings, "notify::gtk-interface-color-scheme", G_CALLBACK(scheme_changed), window, 0);
}
