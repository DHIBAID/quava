#include "../includes/root.h"

void device_free(Device* device) {
	if (device == NULL)
		return;

	g_free(device->id);
	g_free(device->name);
	g_free(device);
}

void request_free(Request* request) {
	if (request == NULL)
		return;

	g_free(request->command);
	g_free(request->peer);
	g_free(request);
}

void pairing_free(Pairing* pairing) {
	if (pairing == NULL)
		return;

	g_free(pairing->peer);
	g_free(pairing->code);
	g_mutex_clear(&pairing->mutex);
	g_cond_clear(&pairing->condition);
	g_free(pairing);
}

void status(App* app, const gchar* text) {
	if (app != NULL && app->status != NULL)
		gtk_label_set_text(app->status, text != NULL ? text : "");
}
