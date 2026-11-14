#include <math.h>

#include "../includes/app.h"

/*
 * Quava draws its own icons instead of using the icon theme, so the app looks
 * the same on every desktop. Everything is authored on a 24x24 grid with a
 * consistent 1.8 px round-capped stroke, and takes its colour from the widget's
 * CSS `color`.
 */

#define STROKE 1.8

static void rounded_rect(cairo_t* cr, double x, double y, double w, double h, double r) {
	cairo_new_sub_path(cr);
	cairo_arc(cr, x + w - r, y + r, r, -M_PI / 2, 0);
	cairo_arc(cr, x + w - r, y + h - r, r, 0, M_PI / 2);
	cairo_arc(cr, x + r, y + h - r, r, M_PI / 2, M_PI);
	cairo_arc(cr, x + r, y + r, r, M_PI, 3 * M_PI / 2);
	cairo_close_path(cr);
}

/* Open arrowhead whose tip is (x, y) and which points along (dx, dy). */
static void arrowhead(cairo_t* cr, double x, double y, double dx, double dy) {
	const double size = 3.6, spread = 0.75;
	double length = hypot(dx, dy);
	dx /= length;
	dy /= length;

	for (int side = -1; side <= 1; side += 2) {
		double angle = side * spread;
		double bx = -(dx * cos(angle) - dy * sin(angle));
		double by = -(dx * sin(angle) + dy * cos(angle));
		cairo_move_to(cr, x, y);
		cairo_line_to(cr, x + bx * size, y + by * size);
	}
}

static void draw_speaker(cairo_t* cr, gboolean muted) {
	cairo_move_to(cr, 4, 9.5);
	cairo_line_to(cr, 8, 9.5);
	cairo_line_to(cr, 12.5, 5.5);
	cairo_line_to(cr, 12.5, 18.5);
	cairo_line_to(cr, 8, 14.5);
	cairo_line_to(cr, 4, 14.5);
	cairo_close_path(cr);
	cairo_stroke(cr);

	if (muted) {
		cairo_move_to(cr, 16, 9.5);
		cairo_line_to(cr, 21, 14.5);
		cairo_move_to(cr, 21, 9.5);
		cairo_line_to(cr, 16, 14.5);
		cairo_stroke(cr);
		return;
	}

	cairo_arc(cr, 12.5, 12, 4.6, -0.85, 0.85);
	cairo_stroke(cr);
	cairo_arc(cr, 12.5, 12, 8.4, -0.8, 0.8);
	cairo_stroke(cr);
}

static void draw_icon(cairo_t* cr, QuavaIconKind kind, int level) {
	switch (kind) {
		case Q_ICON_PHONE:
			rounded_rect(cr, 6.5, 2.5, 11, 19, 2.8);
			cairo_stroke(cr);
			cairo_move_to(cr, 10.5, 18.2);
			cairo_line_to(cr, 13.5, 18.2);
			cairo_stroke(cr);
			break;

		case Q_ICON_MUSIC:
			cairo_arc(cr, 7.5, 17.5, 3, 0, 2 * M_PI);
			cairo_stroke(cr);
			cairo_arc(cr, 16.5, 15, 3, 0, 2 * M_PI);
			cairo_stroke(cr);
			cairo_move_to(cr, 10.5, 17.5);
			cairo_line_to(cr, 10.5, 6);
			cairo_line_to(cr, 19.5, 3.5);
			cairo_line_to(cr, 19.5, 15);
			cairo_stroke(cr);
			break;

		case Q_ICON_SPEAKER:
			draw_speaker(cr, FALSE);
			break;

		case Q_ICON_SPEAKER_MUTED:
			draw_speaker(cr, TRUE);
			break;

		case Q_ICON_BELL:
			cairo_arc(cr, 12, 10, 5.5, M_PI, 2 * M_PI);
			cairo_line_to(cr, 17.5, 15);
			cairo_line_to(cr, 19.5, 17);
			cairo_line_to(cr, 4.5, 17);
			cairo_line_to(cr, 6.5, 15);
			cairo_close_path(cr);
			cairo_stroke(cr);
			cairo_arc(cr, 12, 19.6, 1.8, 0, M_PI);
			cairo_stroke(cr);
			cairo_move_to(cr, 12, 3.2);
			cairo_line_to(cr, 12, 4.5);
			cairo_stroke(cr);
			break;

		case Q_ICON_PLAY:
			cairo_set_line_join(cr, CAIRO_LINE_JOIN_ROUND);
			cairo_move_to(cr, 8, 5);
			cairo_line_to(cr, 19, 12);
			cairo_line_to(cr, 8, 19);
			cairo_close_path(cr);
			cairo_fill_preserve(cr);
			cairo_stroke(cr);
			break;

		case Q_ICON_PAUSE:
			rounded_rect(cr, 6.5, 5, 4, 14, 1.4);
			cairo_fill(cr);
			rounded_rect(cr, 13.5, 5, 4, 14, 1.4);
			cairo_fill(cr);
			break;

		case Q_ICON_NEXT:
			cairo_set_line_join(cr, CAIRO_LINE_JOIN_ROUND);
			cairo_move_to(cr, 6, 5.5);
			cairo_line_to(cr, 15, 12);
			cairo_line_to(cr, 6, 18.5);
			cairo_close_path(cr);
			cairo_fill_preserve(cr);
			cairo_stroke(cr);
			rounded_rect(cr, 17, 5.5, 2.6, 13, 1.1);
			cairo_fill(cr);
			break;

		case Q_ICON_PREV:
			cairo_translate(cr, 24, 0);
			cairo_scale(cr, -1, 1);
			draw_icon(cr, Q_ICON_NEXT, level);
			break;

		case Q_ICON_SHUFFLE:
			cairo_move_to(cr, 3, 7);
			cairo_line_to(cr, 6.5, 7);
			cairo_curve_to(cr, 11, 7, 12, 17, 16.5, 17);
			cairo_line_to(cr, 20, 17);
			cairo_stroke(cr);
			cairo_move_to(cr, 3, 17);
			cairo_line_to(cr, 6.5, 17);
			cairo_curve_to(cr, 11, 17, 12, 7, 16.5, 7);
			cairo_line_to(cr, 20, 7);
			cairo_stroke(cr);
			arrowhead(cr, 21, 7, 1, 0);
			arrowhead(cr, 21, 17, 1, 0);
			cairo_stroke(cr);
			break;

		case Q_ICON_REPEAT:
		case Q_ICON_REPEAT_ONE:
			cairo_move_to(cr, 20, 11);
			cairo_line_to(cr, 20, 9);
			cairo_curve_to(cr, 20, 7, 19, 6, 17, 6);
			cairo_line_to(cr, 5, 6);
			cairo_stroke(cr);
			cairo_move_to(cr, 4, 13);
			cairo_line_to(cr, 4, 15);
			cairo_curve_to(cr, 4, 17, 5, 18, 7, 18);
			cairo_line_to(cr, 19, 18);
			cairo_stroke(cr);
			arrowhead(cr, 18.5, 6, 1, 0);
			arrowhead(cr, 5.5, 18, -1, 0);
			cairo_stroke(cr);
			if (kind == Q_ICON_REPEAT_ONE) {
				cairo_move_to(cr, 10.6, 10.8);
				cairo_line_to(cr, 12.4, 9.4);
				cairo_line_to(cr, 12.4, 14.6);
				cairo_stroke(cr);
			}
			break;

		case Q_ICON_REFRESH:
			cairo_arc(cr, 12, 12, 7.5, 0.7, 5.6);
			cairo_stroke(cr);
			arrowhead(cr, 12 + 7.5 * cos(5.6), 12 + 7.5 * sin(5.6), -sin(5.6), cos(5.6));
			cairo_stroke(cr);
			break;

		case Q_ICON_SEARCH:
			cairo_arc(cr, 10.5, 10.5, 6, 0, 2 * M_PI);
			cairo_stroke(cr);
			cairo_move_to(cr, 15, 15);
			cairo_line_to(cr, 20, 20);
			cairo_stroke(cr);
			break;

		case Q_ICON_CHECK:
			cairo_move_to(cr, 5, 12.5);
			cairo_line_to(cr, 10, 17.5);
			cairo_line_to(cr, 19, 7);
			cairo_stroke(cr);
			break;

		case Q_ICON_CHEVRON:
			cairo_move_to(cr, 6, 9);
			cairo_line_to(cr, 12, 15);
			cairo_line_to(cr, 18, 9);
			cairo_stroke(cr);
			break;

		case Q_ICON_CLIPBOARD:
			rounded_rect(cr, 5, 4.5, 14, 17, 2.6);
			cairo_stroke(cr);
			rounded_rect(cr, 8.8, 2.5, 6.4, 4, 1.6);
			cairo_stroke(cr);
			cairo_move_to(cr, 8.5, 12);
			cairo_line_to(cr, 15.5, 12);
			cairo_move_to(cr, 8.5, 16);
			cairo_line_to(cr, 13.5, 16);
			cairo_stroke(cr);
			break;

		case Q_ICON_PING:
			cairo_arc(cr, 12, 12, 2.2, 0, 2 * M_PI);
			cairo_fill(cr);
			for (int ring = 0; ring < 2; ring++) {
				double radius = 6.2 + ring * 4.2;
				cairo_arc(cr, 12, 12, radius, -0.75, 0.75);
				cairo_stroke(cr);
				cairo_arc(cr, 12, 12, radius, M_PI - 0.75, M_PI + 0.75);
				cairo_stroke(cr);
			}
			break;

		case Q_ICON_BATTERY:
			/* Body, terminal, and a fill that follows `level`. */
			rounded_rect(cr, 2.5, 7, 16, 10, 2.4);
			cairo_stroke(cr);
			rounded_rect(cr, 19.7, 10.2, 2, 3.6, 1);
			cairo_fill(cr);
			if (level >= 0) {
				double width = 11.4 * MIN(level, 100) / 100.0;
				if (width > 0.6) {
					rounded_rect(cr, 4.8, 9.3, width, 5.4, 1.1);
					cairo_fill(cr);
				}
			}
			break;

		case Q_ICON_BOLT:
			cairo_move_to(cr, 13.5, 2.5);
			cairo_line_to(cr, 5.8, 13.4);
			cairo_line_to(cr, 11.2, 13.4);
			cairo_line_to(cr, 10, 21.5);
			cairo_line_to(cr, 18.2, 10);
			cairo_line_to(cr, 12.6, 10);
			cairo_close_path(cr);
			cairo_fill_preserve(cr);
			cairo_set_line_width(cr, 1);
			cairo_stroke(cr);
			break;

		case Q_ICON_WIFI: {
			/* A dot and three arcs; the first `level` of the four are lit. */
			double red, green, blue, alpha;
			cairo_pattern_get_rgba(cairo_get_source(cr), &red, &green, &blue, &alpha);

			for (int part = 0; part < 4; part++) {
				double lit = (level >= 0 && level >= part + 1) ? 1.0 : 0.28;

				cairo_set_source_rgba(cr, red, green, blue, alpha * lit);
				if (part == 0) {
					cairo_arc(cr, 12, 19, 1.8, 0, 2 * M_PI);
					cairo_fill(cr);
				} else {
					cairo_arc(cr, 12, 19, part * 4.4 + 1.2, -M_PI * 0.80, -M_PI * 0.20);
					cairo_stroke(cr);
				}
			}
			break;
		}

		case Q_ICON_SIGNAL: {
			/* Four rising bars; `level` of them are lit. */
			double red, green, blue, alpha;
			cairo_pattern_get_rgba(cairo_get_source(cr), &red, &green, &blue, &alpha);

			for (int bar = 0; bar < 4; bar++) {
				double lit = (level >= 0 && level > bar) ? 1.0 : 0.28;
				double height = 5 + bar * 4.2;

				cairo_set_source_rgba(cr, red, green, blue, alpha * lit);
				rounded_rect(cr, 3.6 + bar * 4.6, 20 - height, 3, height, 1.2);
				cairo_fill(cr);
			}
			break;
		}

		case Q_ICON_BLUETOOTH:
			cairo_move_to(cr, 6.8, 7.6);
			cairo_line_to(cr, 17, 16.4);
			cairo_line_to(cr, 12, 21);
			cairo_line_to(cr, 12, 3);
			cairo_line_to(cr, 17, 7.6);
			cairo_line_to(cr, 6.8, 16.4);
			cairo_stroke(cr);
			break;

		case Q_ICON_GEAR:
			cairo_arc(cr, 12, 12, 3.2, 0, 2 * M_PI);
			cairo_stroke(cr);
			cairo_arc(cr, 12, 12, 6.6, 0, 2 * M_PI);
			cairo_stroke(cr);
			for (int tooth = 0; tooth < 8; tooth++) {
				double angle = tooth * M_PI / 4;
				cairo_move_to(cr, 12 + cos(angle) * 6.6, 12 + sin(angle) * 6.6);
				cairo_line_to(cr, 12 + cos(angle) * 9.4, 12 + sin(angle) * 9.4);
			}
			cairo_stroke(cr);
			break;
	}
}

static void icon_draw(GtkDrawingArea* area, cairo_t* cr, int width, int height, gpointer data) {
	(void)data;

	GdkRGBA color;
	QuavaIconKind kind = GPOINTER_TO_INT(g_object_get_data(G_OBJECT(area), "quava-icon-kind"));
	int level = GPOINTER_TO_INT(g_object_get_data(G_OBJECT(area), "quava-icon-level")) - 1;
	double scale = MIN(width, height) / 24.0;

	gtk_widget_get_color(GTK_WIDGET(area), &color);
	gdk_cairo_set_source_rgba(cr, &color);

	cairo_translate(cr, (width - 24 * scale) / 2, (height - 24 * scale) / 2);
	cairo_scale(cr, scale, scale);
	cairo_set_line_width(cr, STROKE);
	cairo_set_line_cap(cr, CAIRO_LINE_CAP_ROUND);
	cairo_set_line_join(cr, CAIRO_LINE_JOIN_ROUND);

	draw_icon(cr, kind, level);
}

GtkWidget* quava_icon_new(QuavaIconKind kind, gint size) {
	GtkWidget* area = gtk_drawing_area_new();

	gtk_widget_set_size_request(area, size, size);
	gtk_widget_set_halign(area, GTK_ALIGN_CENTER);
	gtk_widget_set_valign(area, GTK_ALIGN_CENTER);
	gtk_widget_add_css_class(area, "q-icon");
	g_object_set_data(G_OBJECT(area), "quava-icon-kind", GINT_TO_POINTER(kind));
	gtk_drawing_area_set_draw_func(GTK_DRAWING_AREA(area), icon_draw, NULL, NULL);

	return area;
}

void quava_icon_set_kind(GtkWidget* icon, QuavaIconKind kind) {
	g_object_set_data(G_OBJECT(icon), "quava-icon-kind", GINT_TO_POINTER(kind));
	gtk_widget_queue_draw(icon);
}

void quava_icon_set_level(GtkWidget* icon, gint level) {
	/* Stored +1 so an unset level reads as -1 (unknown), not 0. */
	g_object_set_data(G_OBJECT(icon), "quava-icon-level", GINT_TO_POINTER(level + 1));
	gtk_widget_queue_draw(icon);
}
