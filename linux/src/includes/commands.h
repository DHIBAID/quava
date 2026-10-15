#ifndef QUAVA_COMMANDS_H
#define QUAVA_COMMANDS_H

#include <gtk/gtk.h>

#include "ipc.h"
#include "root.h"

void clipboard_toggled(GtkToggleButton* toggle, App* app);

void pairing_answer(GObject* source, GAsyncResult* result, gpointer user_data);
gboolean show_pairing_code(gpointer user_data);
void pairing_worker(GTask* task, gpointer source, gpointer task_data, GCancellable* cancellable);
void pairing_done(GObject* source, GAsyncResult* result, gpointer user_data);
void pair_clicked(GtkButton* button, App* app);

#endif
