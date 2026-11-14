#ifndef QUAVA_IPC_H
#define QUAVA_IPC_H

#include <gio/gio.h>
#include <json-glib/json-glib.h>

typedef struct {
	JsonObject* object;
	gchar* error;
} QuavaIpcReply;

gchar* quava_ipc_request_json(const gchar* command, const gchar* peer_id, gint64 timeout_ms, gboolean confirmation, gboolean include_confirmation);
QuavaIpcReply* quava_ipc_open_request(const gchar* json, GSocketConnection** connection_out);
QuavaIpcReply* quava_ipc_read_reply(GInputStream* input);
const gchar* quava_ipc_error(const QuavaIpcReply* reply);
void quava_ipc_reply_free(QuavaIpcReply* reply);

/* ---- Generic requests -------------------------------------------------
 * Build a request with quava_req_new() and the quava_req_* setters, then
 * send it with quava_ipc_send(). The callback runs on the GTK main thread
 * and the reply is freed after it returns. The request is consumed. */
typedef void (*QuavaReplyFn)(QuavaIpcReply* reply, gpointer user_data);

JsonObject* quava_req_new(const gchar* command);
void quava_req_string(JsonObject* request, const gchar* name, const gchar* value);
void quava_req_bool(JsonObject* request, const gchar* name, gboolean value);
void quava_req_int(JsonObject* request, const gchar* name, gint64 value);
void quava_req_double(JsonObject* request, const gchar* name, gdouble value);
void quava_ipc_send(JsonObject* request, QuavaReplyFn callback, gpointer user_data);

#endif
