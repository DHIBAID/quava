package models

import "github.com/godbus/dbus/v5"

type NotificationContent struct {
	AppName       string
	ReplacesID    uint32
	AppIcon       string
	IconData      []byte
	Summary       string
	Body          string
	Actions       []string
	Hints         map[string]dbus.Variant
	ExpireTimeout int32
}

// AndroidNotificationContent is deliberately separate from the freedesktop
// notification shape: Key is Android's opaque, stable notification key and
// Actions carry a label plus whether the action accepts a text reply.
type AndroidNotificationContent struct {
	Key      string
	AppName  string
	Title    string
	Body     string
	IconData []byte
	Actions  []AndroidNotificationAction
	Category string // Android Notification.category, may be empty
	Silent   bool   // Android marked it low-importance / non-alerting
}

// NotificationFilters selects which Android notifications are rendered on the
// host. Matching is case-insensitive. Configured under notification_filters
// in config.yaml.
type NotificationFilters struct {
	// MutedApps never render, matched against the Android app label.
	MutedApps []string `yaml:"muted_apps,omitempty"`
	// OnlyApps, when non-empty, renders only these apps.
	OnlyApps []string `yaml:"only_apps,omitempty"`
	// MutedCategories suppress Android notification categories such as
	// "promo", "social", "recommendation", "progress" or "sys".
	MutedCategories []string `yaml:"muted_categories,omitempty"`
	// Silent controls notifications Android marks as silent (low importance)
	// that the phone chose to send: "preserve" (default) shows them without
	// sound, "drop" ignores them, "normal" shows them like any other.
	Silent string `yaml:"silent,omitempty"`
}

type AndroidNotificationAction struct {
	Title          string
	HasRemoteInput bool
}
