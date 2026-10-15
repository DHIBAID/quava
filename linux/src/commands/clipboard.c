#include "../includes/commands.h"

void clipboard_toggled(GtkToggleButton* toggle, App* app) {
	const gchar* peer = selected_peer(app);
	if (peer == NULL) {
		g_signal_handlers_block_by_func(toggle, G_CALLBACK(clipboard_toggled), app);
		gtk_toggle_button_set_active(toggle, FALSE);
		g_signal_handlers_unblock_by_func(toggle, G_CALLBACK(clipboard_toggled), app);
		return;
	}

	Request* request = g_new0(Request, 1);
	request->kind = REQUEST_ACTION;
	request->command = g_strdup(gtk_toggle_button_get_active(toggle) ? "clipboard-service-start" : "clipboard-service-stop");
	request->peer = g_strdup(peer);
	request->toggle = toggle;
	request->toggle_value = gtk_toggle_button_get_active(toggle);

	GTask* task = g_task_new(NULL, NULL, request_done, app);
	g_task_set_task_data(task, request, (GDestroyNotify)request_free);
	g_task_run_in_thread(task, request_worker);
	g_object_unref(task);
}
