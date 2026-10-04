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
}

type AndroidNotificationAction struct {
	Title          string
	HasRemoteInput bool
}
