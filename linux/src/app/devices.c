#include "../includes/root.h"

void clear_list(GtkListBox* list) {
	GtkWidget* child;

	while ((child = gtk_widget_get_first_child(GTK_WIDGET(list))) != NULL)
		gtk_list_box_remove(list, child);
}

void row_selected(GtkListBox* list, GtkListBoxRow* row, App* app) {
	(void)list;

	if (row == NULL)
		return;

	Device* device = g_object_get_data(G_OBJECT(row), "device");
	if (device == NULL)
		return;

	g_free(app->peer);
	app->peer = g_strdup(device->id);

	gchar* markup = g_markup_printf_escaped(
		"Selected device\n%s\n<small>%s</small>",
		device->name,
		device->id);
	gtk_label_set_markup(app->selected, markup);
	g_free(markup);

	if (device->connected) {
		gtk_button_set_label(app->connect_button, "Disconnect");
		g_object_set_data(G_OBJECT(app->connect_button), "command", (gpointer) "disconnect");
		gtk_widget_remove_css_class(GTK_WIDGET(app->connect_button), "suggested-action");
		gtk_widget_add_css_class(GTK_WIDGET(app->connect_button), "destructive-action");
	} else {
		gtk_button_set_label(app->connect_button, "Connect");
		g_object_set_data(G_OBJECT(app->connect_button), "command", (gpointer) "connect");
		gtk_widget_remove_css_class(GTK_WIDGET(app->connect_button), "destructive-action");
		gtk_widget_add_css_class(GTK_WIDGET(app->connect_button), "suggested-action");
	}
}

void add_device(GtkListBox* list, JsonObject* object, GHashTable* connected) {
	const gchar* id = json_object_has_member(object, "peer_device_id")
						  ? json_object_get_string_member(object, "peer_device_id")
						  : NULL;

	if (id == NULL || *id == '\0')
		return;

	const gchar* name = NULL;
	if (json_object_has_member(object, "peer_device_name"))
		name = json_object_get_string_member(object, "peer_device_name");
	else if (json_object_has_member(object, "name"))
		name = json_object_get_string_member(object, "name");

	Device* device = g_new0(Device, 1);
	device->id = g_strdup(id);
	device->name = g_strdup(name != NULL && *name != '\0' ? name : "Unnamed device");
	device->connected = connected != NULL &&
						g_hash_table_contains(connected, device->id);

	GtkWidget* content = gtk_box_new(GTK_ORIENTATION_VERTICAL, 3);
	gchar* display_name = device->connected
							  ? g_strdup_printf("●  %s", device->name)
							  : g_strdup(device->name);
	GtkWidget* name_label = gtk_label_new(display_name);
	g_free(display_name);

	gtk_label_set_xalign(GTK_LABEL(name_label), 0);
	gtk_label_set_ellipsize(GTK_LABEL(name_label), PANGO_ELLIPSIZE_END);
	if (device->connected)
		gtk_widget_add_css_class(name_label, "connected-device");
	gtk_box_append(GTK_BOX(content), name_label);

	GtkWidget* id_label = gtk_label_new(device->id);
	gtk_label_set_xalign(GTK_LABEL(id_label), 0);
	gtk_label_set_ellipsize(GTK_LABEL(id_label), PANGO_ELLIPSIZE_MIDDLE);
	gtk_widget_add_css_class(id_label, "device-id");
	gtk_box_append(GTK_BOX(content), id_label);

	GtkWidget* row = gtk_list_box_row_new();
	gtk_list_box_row_set_child(GTK_LIST_BOX_ROW(row), content);
	g_object_set_data_full(G_OBJECT(row), "device", device, (GDestroyNotify)device_free);
	gtk_list_box_append(list, row);
}

void fill_list(App* app, GtkListBox* list, JsonObject* response, const gchar* member, GHashTable* connected) {
	(void)app;

	clear_list(list);
	if (!json_object_has_member(response, member))
		return;

	JsonArray* array = json_object_get_array_member(response, member);
	for (guint i = 0; i < json_array_get_length(array); i++) {
		JsonObject* item = json_array_get_object_element(array, i);
		if (item != NULL)
			add_device(list, item, connected);
	}

	if (connected == NULL)
		return;

	for (GtkWidget* child = gtk_widget_get_first_child(GTK_WIDGET(list));
		 child != NULL;
		 child = gtk_widget_get_next_sibling(child)) {
		Device* device = g_object_get_data(G_OBJECT(child), "device");
		if (device != NULL && device->connected) {
			gtk_list_box_select_row(list, GTK_LIST_BOX_ROW(child));
			break;
		}
	}
}

const gchar* selected_peer(App* app) {
	if (app->peer == NULL)
		status(app, "Select a paired or discovered device first");

	return app->peer;
}
