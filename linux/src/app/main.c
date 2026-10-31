#include <gtk/gtk.h>
#include <string.h>

#include "../includes/app.h"

#define APP_ID "io.quava.Desktop"
#define ICON_RESOURCE "/io/quava/Desktop/icons/hicolor/192x192/apps/" APP_ID ".png"

/* Write a file only when its content differs, so launches stay cheap. */
static void write_if_changed(const gchar* path, const gchar* data, gsize length) {
	gchar* existing = NULL;
	gsize existing_length = 0;

	if (g_file_get_contents(path, &existing, &existing_length, NULL) &&
		existing_length == length && memcmp(existing, data, length) == 0) {
		g_free(existing);
		return;
	}
	g_free(existing);

	gchar* directory = g_path_get_dirname(path);
	g_mkdir_with_parents(directory, 0755);
	g_free(directory);

	GError* error = NULL;
	if (!g_file_set_contents(path, data, (gssize)length, &error)) {
		g_warning("Could not write %s: %s", path, error->message);
		g_clear_error(&error);
	}
}

/*
 * On Wayland the desktop identifies an app by matching its app id to an
 * installed .desktop file; it ignores icons set on the window. So the app
 * installs its own launcher entry and icon (from the copy bundled in the
 * binary) for the current user. `make install` does the same thing.
 */
static void register_desktop_entry(void) {
	GError* error = NULL;
	gchar* executable = g_file_read_link("/proc/self/exe", &error);

	if (executable == NULL) {
		g_clear_error(&error);
		return;
	}

	GBytes* icon = g_resources_lookup_data(ICON_RESOURCE, G_RESOURCE_LOOKUP_FLAGS_NONE, NULL);
	if (icon != NULL) {
		gsize length = 0;
		const gchar* data = g_bytes_get_data(icon, &length);
		gchar* icon_path = g_build_filename(g_get_user_data_dir(), "icons", "hicolor", "192x192", "apps", APP_ID ".png", NULL);

		write_if_changed(icon_path, data, length);
		g_free(icon_path);
		g_bytes_unref(icon);
	}

	gchar* quoted = g_shell_quote(executable);
	gchar* entry = g_strdup_printf(
		"[Desktop Entry]\n"
		"Type=Application\n"
		"Name=Quava\n"
		"Comment=Connect your Android phone to this computer\n"
		"Exec=%s\n"
		"Icon=" APP_ID "\n"
		"Terminal=false\n"
		"Categories=Utility;Network;\n"
		"StartupWMClass=" APP_ID "\n",
		quoted);
	gchar* entry_path = g_build_filename(g_get_user_data_dir(), "applications", APP_ID ".desktop", NULL);

	write_if_changed(entry_path, entry, strlen(entry));

	g_free(entry_path);
	g_free(entry);
	g_free(quoted);
	g_free(executable);
}

int main(int argc, char** argv) {
	register_desktop_entry();

	GtkApplication* application = gtk_application_new(APP_ID, G_APPLICATION_DEFAULT_FLAGS);

	g_signal_connect(application, "activate", G_CALLBACK(activate), NULL);

	int status_code = g_application_run(G_APPLICATION(application), argc, argv);
	g_object_unref(application);

	return status_code;
}
