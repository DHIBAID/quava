#include <gtk/gtk.h>

#include "../includes/ui.h"

int main(int argc, char** argv) {
	GtkApplication* application = gtk_application_new("io.quava.Desktop", G_APPLICATION_DEFAULT_FLAGS);

	g_signal_connect(application, "activate", G_CALLBACK(activate), NULL);

	int status_code = g_application_run(G_APPLICATION(application), argc, argv);
	g_object_unref(application);

	return status_code;
}
