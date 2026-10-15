#include "../includes/commands.h"
#include "../includes/ui.h"

void activate(GtkApplication* application, gpointer user_data) {
	(void)user_data;

	App* app = g_new0(App, 1);
	install_css();

	app->window = gtk_application_window_new(application);
	gtk_window_set_title(GTK_WINDOW(app->window), "Quava");
	gtk_window_set_default_size(GTK_WINDOW(app->window), 1040, 720);
	gtk_widget_set_size_request(app->window, 720, 500);

	gchar* app_icon = icon_path();
	gchar* icon_dir = g_path_get_dirname(app_icon);
	gtk_icon_theme_add_search_path(gtk_icon_theme_get_for_display(gdk_display_get_default()), icon_dir);
	gtk_window_set_icon_name(GTK_WINDOW(app->window), "icon");
	g_free(icon_dir);

	GtkWidget* root = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
	gtk_widget_set_margin_top(root, 18);
	gtk_widget_set_margin_bottom(root, 18);
	gtk_widget_set_margin_start(root, 18);
	gtk_widget_set_margin_end(root, 18);

	GtkWidget* title_row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
	GtkWidget* logo = gtk_image_new_from_file(app_icon);
	gtk_image_set_pixel_size(GTK_IMAGE(logo), 38);
	gtk_box_append(GTK_BOX(title_row), logo);

	GtkWidget* title_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);
	GtkWidget* title = gtk_label_new(NULL);
	gtk_label_set_markup(GTK_LABEL(title), "<span size='xx-large' weight='bold'>Quava</span>");
	gtk_label_set_xalign(GTK_LABEL(title), 0);

	GtkWidget* subtitle = gtk_label_new("Manage your Linux ↔ Android bridge");
	gtk_label_set_xalign(GTK_LABEL(subtitle), 0);
	gtk_widget_add_css_class(subtitle, "subtitle");

	gtk_box_append(GTK_BOX(title_box), title);
	gtk_box_append(GTK_BOX(title_box), subtitle);
	gtk_box_append(GTK_BOX(title_row), title_box);
	gtk_box_append(GTK_BOX(root), title_row);
	g_free(app_icon);

	GtkWidget* pane = gtk_paned_new(GTK_ORIENTATION_HORIZONTAL);
	gtk_widget_set_vexpand(pane, TRUE);
	gtk_box_append(GTK_BOX(root), pane);

	/* Left pane: paired and discoverable devices. */
	GtkWidget* left = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
	gtk_widget_set_size_request(left, 270, -1);
	gtk_paned_set_start_child(GTK_PANED(pane), left);
	gtk_paned_set_position(GTK_PANED(pane), 320);

	GtkWidget* paired_header = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
	gtk_widget_set_hexpand(paired_header, TRUE);

	GtkWidget* paired_title = section_title("Paired devices");
	gtk_widget_set_hexpand(paired_title, TRUE);
	gtk_box_append(GTK_BOX(paired_header), paired_title);

	app->connection_summary = GTK_LABEL(gtk_label_new("0 connected"));
	gtk_widget_add_css_class(GTK_WIDGET(app->connection_summary), "subtitle");
	gtk_box_append(GTK_BOX(paired_header), GTK_WIDGET(app->connection_summary));
	gtk_box_append(GTK_BOX(paired_header), button("Refresh", NULL, G_CALLBACK(refresh_clicked), app));

	app->paired = GTK_LIST_BOX(gtk_list_box_new());
	gtk_list_box_set_selection_mode(app->paired, GTK_SELECTION_SINGLE);
	gtk_widget_set_vexpand(GTK_WIDGET(app->paired), TRUE);
	gtk_widget_add_css_class(GTK_WIDGET(app->paired), "device-list");
	g_signal_connect(app->paired, "row-selected", G_CALLBACK(row_selected), app);

	GtkWidget* paired_content = gtk_box_new(GTK_ORIENTATION_VERTICAL, 8);
	gtk_box_append(GTK_BOX(paired_content), paired_header);

	GtkWidget* paired_note = gtk_label_new("Trusted devices on this computer");
	gtk_label_set_xalign(GTK_LABEL(paired_note), 0);
	gtk_widget_add_css_class(paired_note, "subtitle");
	gtk_box_append(GTK_BOX(paired_content), paired_note);
	gtk_box_append(GTK_BOX(paired_content), GTK_WIDGET(app->paired));

	gtk_widget_add_css_class(paired_content, "device-panel");
	gtk_widget_set_vexpand(paired_content, TRUE);
	gtk_box_append(GTK_BOX(left), paired_content);

	GtkWidget* nearby_header = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
	GtkWidget* nearby_title = section_title("Nearby devices");
	gtk_widget_set_hexpand(nearby_title, TRUE);
	gtk_box_append(GTK_BOX(nearby_header), nearby_title);

	GtkWidget* discover = button("Discover", NULL, G_CALLBACK(discover_clicked), app);
	gtk_widget_add_css_class(discover, "suggested-action");
	gtk_box_append(GTK_BOX(nearby_header), discover);

	app->nearby = GTK_LIST_BOX(gtk_list_box_new());
	gtk_list_box_set_selection_mode(app->nearby, GTK_SELECTION_SINGLE);
	gtk_widget_set_size_request(GTK_WIDGET(app->nearby), -1, 100);
	gtk_widget_add_css_class(GTK_WIDGET(app->nearby), "device-list");
	g_signal_connect(app->nearby, "row-selected", G_CALLBACK(row_selected), app);

	GtkWidget* nearby_content = gtk_box_new(GTK_ORIENTATION_VERTICAL, 8);
	gtk_box_append(GTK_BOX(nearby_content), nearby_header);

	GtkWidget* nearby_note = gtk_label_new("Find a phone on this network");
	gtk_label_set_xalign(GTK_LABEL(nearby_note), 0);
	gtk_widget_add_css_class(nearby_note, "subtitle");
	gtk_box_append(GTK_BOX(nearby_content), nearby_note);
	gtk_box_append(GTK_BOX(nearby_content), GTK_WIDGET(app->nearby));
	gtk_widget_add_css_class(nearby_content, "device-panel");
	gtk_box_append(GTK_BOX(left), nearby_content);

	/* Right pane: actions for the currently selected device. */
	GtkWidget* right_scroll = gtk_scrolled_window_new();
	gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(right_scroll), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);

	GtkWidget* right = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
	gtk_widget_set_margin_start(right, 18);
	gtk_widget_set_margin_end(right, 4);
	gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(right_scroll), right);
	gtk_paned_set_end_child(GTK_PANED(pane), right_scroll);

	app->selected = GTK_LABEL(gtk_label_new("Select a device to manage it"));
	gtk_label_set_xalign(app->selected, 0);
	gtk_label_set_wrap(app->selected, TRUE);
	gtk_box_append(GTK_BOX(right), section("Selected device", GTK_WIDGET(app->selected)));

	GtkWidget* connection_grid = gtk_grid_new();
	gtk_grid_set_row_spacing(GTK_GRID(connection_grid), 8);
	gtk_grid_set_column_spacing(GTK_GRID(connection_grid), 8);

	GtkWidget* pair = button("Pair", NULL, G_CALLBACK(pair_clicked), app);
	GtkWidget* connect = button("Connect", "connect", G_CALLBACK(action_clicked), app);
	app->connect_button = GTK_BUTTON(connect);
	gtk_widget_add_css_class(connect, "suggested-action");

	gtk_grid_attach(GTK_GRID(connection_grid), pair, 0, 0, 1, 1);
	gtk_grid_attach(GTK_GRID(connection_grid), connect, 1, 0, 1, 1);
	gtk_grid_attach(GTK_GRID(connection_grid), button("Ping", "ping", G_CALLBACK(action_clicked), app), 0, 1, 1, 1);
	gtk_grid_attach(GTK_GRID(connection_grid), button("Ring", "ring", G_CALLBACK(action_clicked), app), 1, 1, 1, 1);
	gtk_box_append(GTK_BOX(right), section("Connection", connection_grid));

	GtkWidget* clipboard_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 8);
	gtk_box_append(GTK_BOX(clipboard_box), button("Push current clipboard", "clipboard-push", G_CALLBACK(action_clicked), app));

	app->clipboard_sync = GTK_TOGGLE_BUTTON(gtk_toggle_button_new_with_label("Clipboard sync enabled"));
	gtk_toggle_button_set_active(app->clipboard_sync, TRUE);
	g_signal_connect(app->clipboard_sync, "toggled", G_CALLBACK(clipboard_toggled), app);
	gtk_box_append(GTK_BOX(clipboard_box), GTK_WIDGET(app->clipboard_sync));
	gtk_box_append(GTK_BOX(right), section("Clipboard", clipboard_box));

	GtkWidget* notification_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
	gtk_widget_set_hexpand(notification_box, TRUE);

	GtkWidget* notification_hint = gtk_label_new("Sync direction");
	gtk_widget_set_hexpand(notification_hint, TRUE);
	gtk_label_set_xalign(GTK_LABEL(notification_hint), 0);
	gtk_box_append(GTK_BOX(notification_box), notification_hint);

	const gchar* directions[] = {
		"Android → Linux (default)",
		"Bidirectional",
		"Linux → Android",
		"Disabled",
		NULL};
	app->direction = GTK_DROP_DOWN(gtk_drop_down_new_from_strings(directions));
	gtk_widget_set_hexpand(GTK_WIDGET(app->direction), TRUE);
	gtk_box_append(GTK_BOX(notification_box), GTK_WIDGET(app->direction));

	GtkWidget* apply = button("Apply", NULL, G_CALLBACK(notification_clicked), app);
	gtk_widget_add_css_class(apply, "suggested-action");
	gtk_box_append(GTK_BOX(notification_box), apply);
	gtk_box_append(GTK_BOX(right), section("Notifications", notification_box));

	app->status = GTK_LABEL(gtk_label_new("Ready"));
	gtk_label_set_xalign(app->status, 0);
	gtk_label_set_wrap(app->status, TRUE);
	gtk_widget_add_css_class(GTK_WIDGET(app->status), "status");
	gtk_box_append(GTK_BOX(root), GTK_WIDGET(app->status));

	gtk_window_set_child(GTK_WINDOW(app->window), root);
	gtk_window_present(GTK_WINDOW(app->window));

	start_request(app, REQUEST_DEVICES, "devices", NULL, 0);
}
