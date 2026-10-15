#ifndef QUAVA_ROOT_H
#define QUAVA_ROOT_H

#include <gtk/gtk.h>
#include <json-glib/json-glib.h>

typedef struct {
	gchar* id;
	gchar* name;
	gboolean connected;
} Device;

typedef enum {
	REQUEST_DEVICES,
	REQUEST_DISCOVER,
	REQUEST_ACTION
} RequestKind;

typedef struct {
	RequestKind kind;
	gchar* command;
	gchar* peer;
	gint64 timeout;
	GtkToggleButton* toggle;
	gboolean toggle_value;
} Request;

typedef struct {
	GtkWidget* window;
	GtkListBox* paired;
	GtkListBox* nearby;
	GtkLabel* selected;
	GtkLabel* status;
	GtkLabel* connection_summary;
	GtkToggleButton* clipboard_sync;
	GtkButton* connect_button;
	GtkDropDown* direction;
	gchar* peer;
} App;

typedef struct {
	App* app;
	gchar* peer;
	gchar* code;
	GMutex mutex;
	GCond condition;
	gboolean answered;
	gboolean accepted;
} Pairing;

/* Lifecycle and shared state. */
void device_free(Device* device);
void request_free(Request* request);
void pairing_free(Pairing* pairing);
void status(App* app, const gchar* text);

/* Device list and selection handling. */
void clear_list(GtkListBox* list);
void fill_list(App* app, GtkListBox* list, JsonObject* response, const gchar* member, GHashTable* connected);
void row_selected(GtkListBox* list, GtkListBoxRow* row, App* app);
void add_device(GtkListBox* list, JsonObject* object, GHashTable* connected);
const gchar* selected_peer(App* app);

/* Asynchronous daemon requests. */
void request_worker(GTask* task, gpointer source, gpointer task_data, GCancellable* cancellable);
void request_done(GObject* source, GAsyncResult* result, gpointer user_data);
void start_request(App* app, RequestKind kind, const gchar* command, const gchar* peer, gint64 timeout_ms);
void refresh_clicked(GtkButton* button, App* app);
void discover_clicked(GtkButton* button, App* app);
void action_clicked(GtkButton* button, App* app);
void notification_clicked(GtkButton* button, App* app);

/* UI helpers and resources. */
GtkWidget* button(const gchar* label, const gchar* command, GCallback callback, App* app);
GtkWidget* section_title(const gchar* title);
GtkWidget* section(const gchar* title, GtkWidget* content);
void install_css(void);
gchar* resource_path(const gchar* filename);
gchar* icon_path(void);

#endif
