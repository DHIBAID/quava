#include <string.h>

#include "../includes/app.h"

typedef struct {
	App* app;
	gchar* peer;
	gchar* name;
	gchar* code;
	GMutex mutex;
	GCond condition;
	gboolean answered;
	gboolean accepted;
} Pairing;

static void pairing_free(Pairing* pairing) {
	if (pairing == NULL)
		return;

	g_free(pairing->peer);
	g_free(pairing->name);
	g_free(pairing->code);
	g_mutex_clear(&pairing->mutex);
	g_cond_clear(&pairing->condition);
	g_free(pairing);
}

static void pairing_answer(GObject* source, GAsyncResult* result, gpointer user_data) {
	Pairing* pairing = user_data;
	GError* error = NULL;
	int choice = gtk_alert_dialog_choose_finish(GTK_ALERT_DIALOG(source), result, &error);

	if (error != NULL) {
		g_clear_error(&error);
		choice = 0;
	}

	g_mutex_lock(&pairing->mutex);
	pairing->accepted = choice == 1;
	pairing->answered = TRUE;
	g_cond_signal(&pairing->condition);
	g_mutex_unlock(&pairing->mutex);
}

static gboolean show_pairing_code(gpointer user_data) {
	Pairing* pairing = user_data;
	GtkAlertDialog* dialog = gtk_alert_dialog_new("Pair with %s?", pairing->name);
	const gchar* buttons[] = {"Cancel", "Pair", NULL};

	gtk_alert_dialog_set_detail(dialog, pairing->code);
	gtk_alert_dialog_set_buttons(dialog, buttons);
	gtk_alert_dialog_set_cancel_button(dialog, 0);
	gtk_alert_dialog_set_default_button(dialog, 1);
	gtk_alert_dialog_choose(dialog, GTK_WINDOW(pairing->app->window), NULL, pairing_answer, pairing);
	g_object_unref(dialog);

	return G_SOURCE_REMOVE;
}

static void pairing_worker(GTask* task, gpointer source, gpointer task_data, GCancellable* cancellable) {
	(void)source;
	(void)cancellable;

	Pairing* pairing = task_data;
	gchar* json = quava_ipc_request_json("pair", pairing->peer, 120000, FALSE, FALSE);
	GSocketConnection* connection = NULL;
	QuavaIpcReply* reply = quava_ipc_open_request(json, &connection);
	g_free(json);

	if (reply->error != NULL) {
		g_task_return_pointer(task, reply, (GDestroyNotify)quava_ipc_reply_free);
		return;
	}
	quava_ipc_reply_free(reply);

	reply = quava_ipc_read_reply(g_io_stream_get_input_stream(G_IO_STREAM(connection)));
	const gchar* error = quava_ipc_error(reply);

	if (error != NULL ||
		!json_object_has_member(reply->object, "event") ||
		g_strcmp0(json_object_get_string_member(reply->object, "event"), "pairing_code") != 0) {
		g_object_unref(connection);
		g_task_return_pointer(task, reply, (GDestroyNotify)quava_ipc_reply_free);
		return;
	}

	guint64 code = (guint64)json_object_get_int_member(reply->object, "code");
	const gchar* peer_name = json_object_has_member(reply->object, "peer_name")
								 ? json_object_get_string_member(reply->object, "peer_name")
								 : pairing->name;

	g_free(pairing->name);
	pairing->name = g_strdup(peer_name);
	pairing->code = g_strdup_printf(
		"Check that this code matches the one on %s:\n\n%03" G_GUINT64_FORMAT " %03" G_GUINT64_FORMAT,
		peer_name,
		code / 1000,
		code % 1000);

	quava_ipc_reply_free(reply);

	/* GTK work always goes through the main loop, never from this worker. */
	g_idle_add(show_pairing_code, pairing);

	g_mutex_lock(&pairing->mutex);
	while (!pairing->answered)
		g_cond_wait(&pairing->condition, &pairing->mutex);
	gboolean accepted = pairing->accepted;
	g_mutex_unlock(&pairing->mutex);

	json = quava_ipc_request_json("confirm_pairing", NULL, 0, accepted, TRUE);
	GOutputStream* output = g_io_stream_get_output_stream(G_IO_STREAM(connection));
	gchar* line = g_strconcat(json, "\n", NULL);
	g_free(json);

	GError* write_error = NULL;
	gsize written = 0;

	if (!g_output_stream_write_all(output, line, strlen(line), &written, NULL, &write_error)) {
		QuavaIpcReply* failed = g_new0(QuavaIpcReply, 1);
		failed->error = g_strdup_printf("cannot confirm pairing: %s", write_error->message);
		g_clear_error(&write_error);
		g_free(line);
		g_object_unref(connection);
		g_task_return_pointer(task, failed, (GDestroyNotify)quava_ipc_reply_free);
		return;
	}

	g_free(line);
	reply = quava_ipc_read_reply(g_io_stream_get_input_stream(G_IO_STREAM(connection)));
	g_object_unref(connection);

	g_task_return_pointer(task, reply, (GDestroyNotify)quava_ipc_reply_free);
}

static void pairing_done(GObject* source, GAsyncResult* result, gpointer user_data) {
	(void)source;

	App* app = user_data;
	QuavaIpcReply* reply = g_task_propagate_pointer(G_TASK(result), NULL);
	const gchar* error = quava_ipc_error(reply);

	if (error != NULL) {
		app_toast(app, error);
	} else {
		app_toast(app, "Paired. You can connect now.");
		devices_refresh_now();
	}

	quava_ipc_reply_free(reply);
}

void quava_pair(App* app, const gchar* peer_id, const gchar* name) {
	Pairing* pairing = g_new0(Pairing, 1);

	pairing->app = app;
	pairing->peer = g_strdup(peer_id);
	pairing->name = g_strdup(name != NULL ? name : "this device");
	g_mutex_init(&pairing->mutex);
	g_cond_init(&pairing->condition);

	app_toast(app, "Waiting for the phone to show a pairing code…");

	GTask* task = g_task_new(NULL, NULL, pairing_done, app);
	g_task_set_task_data(task, pairing, (GDestroyNotify)pairing_free);
	g_task_run_in_thread(task, pairing_worker);
	g_object_unref(task);
}
