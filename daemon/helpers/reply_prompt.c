#include <gtk/gtk.h>
#include <stdio.h>

/* Minimal GTK4 reply dialog for Android notification quick-replies. Replaces
 * the zenity dependency: usage is `quava-reply-prompt <app-name> <action-title>`.
 * Prints the typed reply (and only the reply) to stdout and exits 0, or exits
 * 1 with no output if the prompt is cancelled or closed. */

static int result_code = 1;

static void respond(GtkWindow* window, GtkWidget* entry, gboolean send) {
	if (send && entry != NULL) {
		const char* text = gtk_editable_get_text(GTK_EDITABLE(entry));

		fputs(text, stdout);
		fputc('\n', stdout);
		fflush(stdout);
		result_code = 0;
	}

	gtk_window_destroy(window);
}

static void send_clicked(GtkButton* button, gpointer data) {
	respond(GTK_WINDOW(gtk_widget_get_root(GTK_WIDGET(button))), GTK_WIDGET(data), TRUE);
}

static void cancel_clicked(GtkButton* button, gpointer data) {
	(void)data;

	respond(GTK_WINDOW(gtk_widget_get_root(GTK_WIDGET(button))), NULL, FALSE);
}

static gboolean close_requested(GtkWindow* window, gpointer data) {
	(void)data;

	respond(window, NULL, FALSE);
	return TRUE;
}

static void entry_activated(GtkEntry* entry, gpointer data) {
	respond(GTK_WINDOW(data), GTK_WIDGET(entry), TRUE);
}

static void activate(GApplication* app, gpointer user_data) {
	const gchar** args = user_data;
	const gchar* app_name = args[0];
	const gchar* action_title = args[1];

	GtkWidget* window = gtk_application_window_new(GTK_APPLICATION(app));
	gtk_window_set_title(GTK_WINDOW(window), "Quava — Reply");
	gtk_window_set_default_size(GTK_WINDOW(window), 380, -1);
	gtk_window_set_resizable(GTK_WINDOW(window), FALSE);

	GtkWidget* box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
	gtk_widget_set_margin_start(box, 16);
	gtk_widget_set_margin_end(box, 16);
	gtk_widget_set_margin_top(box, 16);
	gtk_widget_set_margin_bottom(box, 16);

	gchar* prompt_text = g_strdup_printf("%s to %s", action_title, app_name);
	GtkWidget* label = gtk_label_new(prompt_text);
	g_free(prompt_text);
	gtk_label_set_xalign(GTK_LABEL(label), 0);
	gtk_box_append(GTK_BOX(box), label);

	GtkWidget* entry = gtk_entry_new();
	gtk_entry_set_placeholder_text(GTK_ENTRY(entry), "Type a reply…");
	gtk_box_append(GTK_BOX(box), entry);

	GtkWidget* buttons = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
	gtk_widget_set_halign(buttons, GTK_ALIGN_END);

	GtkWidget* cancel = gtk_button_new_with_label("Cancel");
	g_signal_connect(cancel, "clicked", G_CALLBACK(cancel_clicked), NULL);
	gtk_box_append(GTK_BOX(buttons), cancel);

	GtkWidget* send = gtk_button_new_with_label("Send");
	gtk_widget_add_css_class(send, "suggested-action");
	g_signal_connect(send, "clicked", G_CALLBACK(send_clicked), entry);
	gtk_box_append(GTK_BOX(buttons), send);

	gtk_box_append(GTK_BOX(box), buttons);
	gtk_window_set_child(GTK_WINDOW(window), box);
	gtk_window_set_default_widget(GTK_WINDOW(window), send);

	g_signal_connect(entry, "activate", G_CALLBACK(entry_activated), window);
	g_signal_connect(window, "close-request", G_CALLBACK(close_requested), NULL);

	gtk_window_present(GTK_WINDOW(window));
	gtk_widget_grab_focus(entry);
}

int main(int argc, char** argv) {
	if (argc < 3) {
		fprintf(stderr, "usage: %s <app-name> <action-title>\n", argv[0]);
		return 2;
	}

	const gchar* args[2] = {argv[1], argv[2]};
	/* Each invocation is its own reply prompt, not a singleton the desktop
	 * reactivates: several notifications may need a reply at once. */
	GtkApplication* app = gtk_application_new("io.quava.ReplyPrompt", G_APPLICATION_NON_UNIQUE);

	g_signal_connect(app, "activate", G_CALLBACK(activate), args);
	g_application_run(G_APPLICATION(app), 0, NULL);
	g_object_unref(app);

	return result_code;
}
