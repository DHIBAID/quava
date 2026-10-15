#ifndef QUAVA_IPC_H
#define QUAVA_IPC_H

#include <gio/gio.h>
#include <json-glib/json-glib.h>

typedef struct {
	JsonObject* object;
	gchar* error;
} QuavaIpcReply;

gchar* quava_ipc_request_json(const gchar* command, const gchar* peer_id, gint64 timeout_ms, gboolean confirmation, gboolean include_confirmation);
QuavaIpcReply* quava_ipc_call(const gchar* command, const gchar* peer_id, gint64 timeout_ms);
QuavaIpcReply* quava_ipc_open_request(const gchar* json, GSocketConnection** connection_out);
QuavaIpcReply* quava_ipc_read_reply(GInputStream* input);
const gchar* quava_ipc_error(const QuavaIpcReply* reply);
void quava_ipc_reply_free(QuavaIpcReply* reply);

#endif
