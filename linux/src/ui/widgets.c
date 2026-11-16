#include "../includes/app.h"

GtkWidget* q_label(const gchar* text, const gchar* css_class) {
	GtkWidget* label = gtk_label_new(text);

	gtk_label_set_xalign(GTK_LABEL(label), 0);
	if (css_class != NULL)
		gtk_widget_add_css_class(label, css_class);

	return label;
}

GtkWidget* q_wrapped_label(const gchar* text, const gchar* css_class) {
	GtkWidget* label = q_label(text, css_class);

	gtk_label_set_wrap(GTK_LABEL(label), TRUE);
	gtk_label_set_wrap_mode(GTK_LABEL(label), PANGO_WRAP_WORD_CHAR);

	return label;
}

GtkWidget* q_icon_button(QuavaIconKind kind, const gchar* tooltip, const gchar* css_class) {
	GtkWidget* button = gtk_button_new();

	gtk_button_set_child(GTK_BUTTON(button), quava_icon_new(kind, 20));
	gtk_widget_add_css_class(button, "q-icon-button");
	if (css_class != NULL)
		gtk_widget_add_css_class(button, css_class);
	if (tooltip != NULL)
		gtk_widget_set_tooltip_text(button, tooltip);
	gtk_widget_set_valign(button, GTK_ALIGN_CENTER);

	return button;
}

GtkWidget* q_text_button(const gchar* label, const gchar* css_class) {
	GtkWidget* button = gtk_button_new_with_label(label);

	gtk_widget_add_css_class(button, "q-button");
	if (css_class != NULL)
		gtk_widget_add_css_class(button, css_class);
	gtk_widget_set_valign(button, GTK_ALIGN_CENTER);

	return button;
}

GtkWidget* q_page_header(const gchar* title, const gchar* subtitle, GtkWidget* action) {
	GtkWidget* row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
	GtkWidget* text = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);

	gtk_widget_set_hexpand(text, TRUE);
	gtk_box_append(GTK_BOX(text), q_label(title, "q-page-title"));
	if (subtitle != NULL)
		gtk_box_append(GTK_BOX(text), q_wrapped_label(subtitle, "q-page-subtitle"));
	gtk_box_append(GTK_BOX(row), text);

	if (action != NULL) {
		gtk_widget_set_valign(action, GTK_ALIGN_START);
		gtk_box_append(GTK_BOX(row), action);
	}

	return row;
}

GtkWidget* q_empty_state(QuavaIconKind kind, const gchar* title, const gchar* hint) {
	GtkWidget* box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 8);
	GtkWidget* label;

	gtk_widget_add_css_class(box, "q-empty");
	gtk_widget_set_halign(box, GTK_ALIGN_CENTER);
	gtk_widget_set_valign(box, GTK_ALIGN_CENTER);
	gtk_box_append(GTK_BOX(box), quava_icon_new(kind, 44));

	label = q_label(title, "q-title");
	gtk_label_set_xalign(GTK_LABEL(label), 0.5);
	gtk_box_append(GTK_BOX(box), label);

	if (hint != NULL) {
		label = q_wrapped_label(hint, "q-dim");
		gtk_label_set_xalign(GTK_LABEL(label), 0.5);
		gtk_label_set_justify(GTK_LABEL(label), GTK_JUSTIFY_CENTER);
		gtk_box_append(GTK_BOX(box), label);
	}

	return box;
}

GtkWidget* q_scrolled(GtkWidget* child) {
	GtkWidget* scroll = gtk_scrolled_window_new();

	gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(scroll), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
	gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(scroll), child);
	gtk_widget_set_vexpand(scroll, TRUE);

	return scroll;
}

void q_clear_box(GtkWidget* box) {
	GtkWidget* child;

	while ((child = gtk_widget_get_first_child(box)) != NULL)
		gtk_box_remove(GTK_BOX(box), child);
}

const gchar* q_json_string(JsonObject* object, const gchar* name, const gchar* fallback) {
	if (object != NULL && json_object_has_member(object, name)) {
		JsonNode* node = json_object_get_member(object, name);
		if (JSON_NODE_HOLDS_VALUE(node) && json_node_get_value_type(node) == G_TYPE_STRING) {
			const gchar* value = json_node_get_string(node);
			if (value != NULL && *value != '\0')
				return value;
		}
	}

	return fallback;
}

JsonArray* q_json_array(JsonObject* object, const gchar* name) {
	if (object == NULL || !json_object_has_member(object, name))
		return NULL;

	JsonNode* node = json_object_get_member(object, name);
	return JSON_NODE_HOLDS_ARRAY(node) ? json_node_get_array(node) : NULL;
}

JsonObject* q_json_object(JsonObject* object, const gchar* name) {
	if (object == NULL || !json_object_has_member(object, name))
		return NULL;

	JsonNode* node = json_object_get_member(object, name);
	return JSON_NODE_HOLDS_OBJECT(node) ? json_node_get_object(node) : NULL;
}

gint64 q_json_int(JsonObject* object, const gchar* name, gint64 fallback) {
	if (object == NULL || !json_object_has_member(object, name))
		return fallback;

	JsonNode* node = json_object_get_member(object, name);
	return JSON_NODE_HOLDS_VALUE(node) && json_node_get_value_type(node) != G_TYPE_STRING && json_node_get_value_type(node) != G_TYPE_BOOLEAN
			   ? json_node_get_int(node)
			   : fallback;
}

gdouble q_json_double(JsonObject* object, const gchar* name, gdouble fallback) {
	if (object == NULL || !json_object_has_member(object, name))
		return fallback;

	JsonNode* node = json_object_get_member(object, name);
	return JSON_NODE_HOLDS_VALUE(node) && json_node_get_value_type(node) != G_TYPE_STRING && json_node_get_value_type(node) != G_TYPE_BOOLEAN
			   ? json_node_get_double(node)
			   : fallback;
}

gboolean q_json_bool(JsonObject* object, const gchar* name) {
	if (object == NULL || !json_object_has_member(object, name))
		return FALSE;

	JsonNode* node = json_object_get_member(object, name);
	return JSON_NODE_HOLDS_VALUE(node) && json_node_get_value_type(node) == G_TYPE_BOOLEAN && json_node_get_boolean(node);
}

GdkTexture* q_json_texture(JsonObject* object, const gchar* name) {
	const gchar* encoded = q_json_string(object, name, NULL);
	if (encoded == NULL)
		return NULL;

	gsize length = 0;
	guchar* data = g_base64_decode(encoded, &length);
	if (data == NULL || length == 0) {
		g_free(data);
		return NULL;
	}

	GBytes* bytes = g_bytes_new_take(data, length);
	GdkTexture* texture = gdk_texture_new_from_bytes(bytes, NULL);
	g_bytes_unref(bytes);

	return texture;
}

gchar* q_format_time(gint64 microseconds) {
	gint64 seconds = microseconds > 0 ? microseconds / 1000000 : 0;

	return g_strdup_printf("%" G_GINT64_FORMAT ":%02" G_GINT64_FORMAT, seconds / 60, seconds % 60);
}
