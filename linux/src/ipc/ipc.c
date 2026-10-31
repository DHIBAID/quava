#include "../includes/ipc.h"

#include <gio/gunixsocketaddress.h>
#include <string.h>

void quava_ipc_reply_free(QuavaIpcReply* reply) {
	if (reply == NULL)
		return;

	if (reply->object != NULL)
		json_object_unref(reply->object);

	g_free(reply->error);
	g_free(reply);
}

gchar* quava_ipc_request_json(const gchar* command, const gchar* peer_id, gint64 timeout_ms, gboolean confirmation, gboolean include_confirmation) {
	JsonBuilder* builder = json_builder_new();

	json_builder_begin_object(builder);
	json_builder_set_member_name(builder, "command");
	json_builder_add_string_value(builder, command);

	if (peer_id != NULL) {
		json_builder_set_member_name(builder, "peer_device_id");
		json_builder_add_string_value(builder, peer_id);
	}

	if (timeout_ms > 0) {
		json_builder_set_member_name(builder, "timeout_ms");
		json_builder_add_int_value(builder, timeout_ms);
	}

	if (include_confirmation) {
		json_builder_set_member_name(builder, "confirm");
		json_builder_add_boolean_value(builder, confirmation);
	}

	json_builder_end_object(builder);

	JsonNode* root = json_builder_get_root(builder);
	JsonGenerator* generator = json_generator_new();
	json_generator_set_root(generator, root);

	gchar* json = json_generator_to_data(generator, NULL);

	json_node_free(root);
	g_object_unref(generator);
	g_object_unref(builder);

	return json;
}

QuavaIpcReply* quava_ipc_read_reply(GInputStream* input) {
	QuavaIpcReply* reply = g_new0(QuavaIpcReply, 1);
	GDataInputStream* data = g_data_input_stream_new(input);
	GError* error = NULL;
	gsize length = 0;

	gchar* line = g_data_input_stream_read_line_utf8(data, &length, NULL, &error);
	g_object_unref(data);

	if (line == NULL) {
		if (error != NULL) {
			reply->error = g_strdup_printf("cannot read quavad's reply: %s", error->message);
			g_clear_error(&error);
		} else {
			reply->error = g_strdup("quavad closed the connection without replying");
		}
		return reply;
	}

	JsonParser* parser = json_parser_new();
	if (!json_parser_load_from_data(parser, line, (gssize)length, &error)) {
		reply->error = g_strdup_printf("invalid reply from quavad: %s", error->message);
		g_clear_error(&error);
	} else {
		JsonNode* root = json_parser_get_root(parser);
		if (JSON_NODE_HOLDS_OBJECT(root))
			reply->object = json_object_ref(json_node_get_object(root));
		else
			reply->error = g_strdup("invalid reply from quavad: expected a JSON object");
	}

	g_object_unref(parser);
	g_free(line);

	return reply;
}

QuavaIpcReply* quava_ipc_open_request(const gchar* json,
									  GSocketConnection** connection_out) {
	QuavaIpcReply* reply = g_new0(QuavaIpcReply, 1);
	GError* error = NULL;

	if (connection_out != NULL)
		*connection_out = NULL;

	gchar* path = g_build_filename(g_get_user_config_dir(), "quava", "quavad.sock", NULL);
	GSocketClient* client = g_socket_client_new();
	GSocketAddress* address = g_unix_socket_address_new(path);
	GSocketConnection* connection = g_socket_client_connect(client, G_SOCKET_CONNECTABLE(address), NULL, &error);

	g_free(path);
	g_object_unref(address);
	g_object_unref(client);

	if (connection == NULL) {
		reply->error = g_strdup_printf("cannot reach quavad: %s",
									   error->message);
		g_clear_error(&error);
		return reply;
	}

	GOutputStream* output =
		g_io_stream_get_output_stream(G_IO_STREAM(connection));
	gchar* line = g_strconcat(json, "\n", NULL);
	gsize written = 0;

	if (!g_output_stream_write_all(output, line, strlen(line), &written, NULL, &error) ||
		!g_output_stream_flush(output, NULL, &error)) {
		reply->error = g_strdup_printf("cannot send the request to quavad: %s", error->message);
		g_clear_error(&error);
		g_free(line);
		g_object_unref(connection);
		return reply;
	}

	g_free(line);

	if (connection_out == NULL) {
		g_object_unref(connection);
		reply->error = g_strdup("internal error: no connection output was provided");
		return reply;
	}

	*connection_out = connection;
	return reply;
}

const gchar* quava_ipc_error(const QuavaIpcReply* reply) {
	if (reply == NULL)
		return "No response from quavad";

	if (reply->error != NULL)
		return reply->error;

	if (reply->object == NULL)
		return "Invalid or empty response from quavad";

	if (reply->object != NULL &&
		json_object_has_member(reply->object, "error")) {
		return json_object_get_string_member(reply->object, "error");
	}

	return NULL;
}

/* ---- Generic requests ------------------------------------------------- */

JsonObject* quava_req_new(const gchar* command) {
	JsonObject* request = json_object_new();
	json_object_set_string_member(request, "command", command);
	return request;
}

void quava_req_string(JsonObject* request, const gchar* name, const gchar* value) {
	json_object_set_string_member(request, name, value);
}

void quava_req_bool(JsonObject* request, const gchar* name, gboolean value) {
	json_object_set_boolean_member(request, name, value);
}

void quava_req_int(JsonObject* request, const gchar* name, gint64 value) {
	json_object_set_int_member(request, name, value);
}

void quava_req_double(JsonObject* request, const gchar* name, gdouble value) {
	json_object_set_double_member(request, name, value);
}

typedef struct {
	JsonObject* request;
	QuavaReplyFn callback;
	gpointer user_data;
} SendContext;

static void send_context_free(SendContext* context) {
	json_object_unref(context->request);
	g_free(context);
}

static void send_worker(GTask* task, gpointer source, gpointer task_data, GCancellable* cancellable) {
	(void)source;
	(void)cancellable;

	SendContext* context = task_data;
	JsonNode* node = json_node_new(JSON_NODE_OBJECT);
	json_node_set_object(node, context->request);
	JsonGenerator* generator = json_generator_new();
	json_generator_set_root(generator, node);
	gchar* json = json_generator_to_data(generator, NULL);
	json_node_free(node);
	g_object_unref(generator);

	GSocketConnection* connection = NULL;
	QuavaIpcReply* reply = quava_ipc_open_request(json, &connection);
	g_free(json);

	if (reply->error == NULL) {
		quava_ipc_reply_free(reply);
		reply = quava_ipc_read_reply(g_io_stream_get_input_stream(G_IO_STREAM(connection)));
		g_object_unref(connection);
	}

	g_task_return_pointer(task, reply, (GDestroyNotify)quava_ipc_reply_free);
}

static void send_done(GObject* source, GAsyncResult* result, gpointer data) {
	(void)source;

	SendContext* context = data;
	QuavaIpcReply* reply = g_task_propagate_pointer(G_TASK(result), NULL);

	if (reply == NULL) {
		reply = g_new0(QuavaIpcReply, 1);
		reply->error = g_strdup("the request failed without a reply");
	}

	if (context->callback != NULL)
		context->callback(reply, context->user_data);

	quava_ipc_reply_free(reply);
	send_context_free(context);
}

void quava_ipc_send(JsonObject* request, QuavaReplyFn callback, gpointer user_data) {
	SendContext* context = g_new0(SendContext, 1);
	context->request = request;
	context->callback = callback;
	context->user_data = user_data;

	GTask* task = g_task_new(NULL, NULL, send_done, context);
	g_task_set_task_data(task, context, NULL);
	g_task_run_in_thread(task, send_worker);
	g_object_unref(task);
}
