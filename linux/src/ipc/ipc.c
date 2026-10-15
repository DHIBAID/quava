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
			reply->error = g_strdup_printf("Cannot read quavad response: %s", error->message);
			g_clear_error(&error);
		} else {
			reply->error = g_strdup("quavad closed the connection without a response");
		}
		return reply;
	}

	JsonParser* parser = json_parser_new();
	if (!json_parser_load_from_data(parser, line, (gssize)length, &error)) {
		reply->error = g_strdup_printf("Invalid response from quavad: %s", error->message);
		g_clear_error(&error);
	} else {
		JsonNode* root = json_parser_get_root(parser);
		if (JSON_NODE_HOLDS_OBJECT(root))
			reply->object = json_object_ref(json_node_get_object(root));
		else
			reply->error = g_strdup("Invalid response from quavad: expected a JSON object");
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
		reply->error = g_strdup_printf("Cannot reach quavad: %s",
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
		reply->error = g_strdup_printf("Cannot write to quavad: %s", error->message);
		g_clear_error(&error);
		g_free(line);
		g_object_unref(connection);
		return reply;
	}

	g_free(line);

	if (connection_out == NULL) {
		g_object_unref(connection);
		reply->error = g_strdup("Internal error: no connection output was provided");
		return reply;
	}

	*connection_out = connection;
	return reply;
}

QuavaIpcReply* quava_ipc_call(const gchar* command, const gchar* peer_id, gint64 timeout_ms) {
	gchar* json = quava_ipc_request_json(command, peer_id, timeout_ms, FALSE, FALSE);
	GSocketConnection* connection = NULL;
	QuavaIpcReply* started = quava_ipc_open_request(json, &connection);
	g_free(json);

	if (started->error != NULL)
		return started;

	quava_ipc_reply_free(started);
	QuavaIpcReply* reply = quava_ipc_read_reply(
		g_io_stream_get_input_stream(G_IO_STREAM(connection)));
	g_object_unref(connection);

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
