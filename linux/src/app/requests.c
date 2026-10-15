#include "../includes/commands.h"
#include "../includes/ipc.h"
#include "../includes/root.h"

void request_worker(GTask* task, gpointer source, gpointer task_data, GCancellable* cancellable) {
	(void)source;
	(void)cancellable;

	Request* request = task_data;
	QuavaIpcReply* reply = quava_ipc_call(request->command, request->peer, request->timeout);
	g_task_return_pointer(task, reply, (GDestroyNotify)quava_ipc_reply_free);
}

void request_done(GObject* source, GAsyncResult* result, gpointer user_data) {
	(void)source;

	App* app = user_data;
	GTask* task = G_TASK(result);
	Request* request = g_task_get_task_data(task);
	QuavaIpcReply* reply = g_task_propagate_pointer(task, NULL);

	if (reply == NULL) {
		status(app, "The request failed without a response");
		return;
	}

	const gchar* error = quava_ipc_error(reply);
	if (error != NULL) {
		if (request->toggle != NULL) {
			g_signal_handlers_block_by_func(request->toggle, G_CALLBACK(clipboard_toggled), app);
			gtk_toggle_button_set_active(request->toggle, !request->toggle_value);
			g_signal_handlers_unblock_by_func(request->toggle, G_CALLBACK(clipboard_toggled), app);
		}

		status(app, error);
	} else if (request->kind == REQUEST_DEVICES) {
		GHashTable* connected = g_hash_table_new(g_str_hash, g_str_equal);

		if (json_object_has_member(reply->object, "connected_device_ids")) {
			JsonArray* ids = json_object_get_array_member(reply->object, "connected_device_ids");
			for (guint i = 0; i < json_array_get_length(ids); i++) {
				const gchar* id = json_array_get_string_element(ids, i);
				if (id != NULL)
					g_hash_table_add(connected, (gpointer)id);
			}
		}

		fill_list(app, app->paired, reply->object, "devices", connected);

		gchar* summary = g_strdup_printf("%u connected", g_hash_table_size(connected));
		gtk_label_set_text(app->connection_summary, summary);
		g_free(summary);
		g_hash_table_unref(connected);
		status(app, "Paired devices refreshed");
	} else if (request->kind == REQUEST_DISCOVER) {
		fill_list(app, app->nearby, reply->object, "discovered", NULL);
		status(app, "Nearby devices refreshed");
	} else {
		gchar* message = g_strdup_printf("%s completed", request->command);
		status(app, message);
		g_free(message);
	}

	quava_ipc_reply_free(reply);
}

void start_request(App* app, RequestKind kind, const gchar* command, const gchar* peer, gint64 timeout_ms) {
	Request* request = g_new0(Request, 1);
	request->kind = kind;
	request->command = g_strdup(command);
	request->peer = g_strdup(peer);
	request->timeout = timeout_ms;

	GTask* task = g_task_new(NULL, NULL, request_done, app);
	g_task_set_task_data(task, request, (GDestroyNotify)request_free);
	g_task_run_in_thread(task, request_worker);
	g_object_unref(task);
}

void refresh_clicked(GtkButton* button_widget, App* app) {
	(void)button_widget;
	start_request(app, REQUEST_DEVICES, "devices", NULL, 0);
}

void discover_clicked(GtkButton* button_widget, App* app) {
	(void)button_widget;
	status(app, "Searching the local network…");
	start_request(app, REQUEST_DISCOVER, "discover", NULL, 10000);
}

void action_clicked(GtkButton* button_widget, App* app) {
	const gchar* peer = selected_peer(app);
	if (peer != NULL) {
		const gchar* command = g_object_get_data(G_OBJECT(button_widget), "command");
		start_request(app, REQUEST_ACTION, command, peer, 0);
	}
}

void notification_clicked(GtkButton* button_widget, App* app) {
	(void)button_widget;

	const gchar* peer = selected_peer(app);
	if (peer == NULL)
		return;

	static const gchar* commands[] = {
		"notif-sync-tohost",
		"notif-sync-both",
		"notif-sync-toclient",
		"notif-sync-stop"};

	guint selected = gtk_drop_down_get_selected(app->direction);
	if (selected >= G_N_ELEMENTS(commands))
		selected = 0;

	start_request(app, REQUEST_ACTION, commands[selected], peer, 0);
}
