#include "../includes/root.h"

GtkWidget* button(const gchar* label, const gchar* command, GCallback callback, App* app) {
	GtkWidget* widget = gtk_button_new_with_label(label);

	if (command != NULL)
		g_object_set_data(G_OBJECT(widget), "command", (gpointer)command);

	g_signal_connect(widget, "clicked", callback, app);
	return widget;
}

GtkWidget* section_title(const gchar* title) {
	GtkWidget* label = gtk_label_new(NULL);
	gchar* markup = g_markup_printf_escaped("<b>%s</b>", title);

	gtk_label_set_markup(GTK_LABEL(label), markup);
	gtk_label_set_xalign(GTK_LABEL(label), 0);
	gtk_widget_add_css_class(label, "section-title");
	g_free(markup);

	return label;
}

GtkWidget* section(const gchar* title, GtkWidget* content) {
	GtkWidget* box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 10);

	gtk_widget_add_css_class(box, "section");
	gtk_box_append(GTK_BOX(box), section_title(title));
	gtk_box_append(GTK_BOX(box), content);

	return box;
}

gchar* resource_path(const gchar* filename) {
	GError* error = NULL;
	gchar* executable = g_file_read_link("/proc/self/exe", &error);

	if (executable == NULL) {
		g_clear_error(&error);
		return g_build_filename("res", filename, NULL);
	}

	gchar* build_dir = g_path_get_dirname(executable);
	gchar* project_dir = g_path_get_dirname(build_dir);
	gchar* path = g_build_filename(project_dir, "res", filename, NULL);

	g_free(executable);
	g_free(build_dir);
	g_free(project_dir);

	return path;
}

gchar* icon_path(void) {
	return resource_path("icon.png");
}

void install_css(void) {
	gchar* path = resource_path("app.css");
	gchar* css = NULL;
	gsize length = 0;
	GError* error = NULL;

	if (!g_file_get_contents(path, &css, &length, &error)) {
		g_warning("Failed to load CSS from %s: %s", path, error->message);
		g_clear_error(&error);
		g_free(path);
		return;
	}

	GtkCssProvider* provider = gtk_css_provider_new();
	gtk_css_provider_load_from_data(provider, css, (gssize)length);

	gtk_style_context_add_provider_for_display(gdk_display_get_default(), GTK_STYLE_PROVIDER(provider), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);

	g_object_unref(provider);
	g_free(css);
	g_free(path);
}
