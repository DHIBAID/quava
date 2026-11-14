package models

import (
	"context"
	"libquava/discovery"
	"libquava/models"
	"libquava/session"
)

const ControlSocketName = "quavad.sock"

type Request struct {
	Command      string  `json:"command"`
	PeerDeviceID string  `json:"peer_device_id,omitempty"`
	TimeoutMS    int64   `json:"timeout_ms,omitempty"`
	Confirm      *bool   `json:"confirm,omitempty"`
	BusName      string  `json:"bus_name,omitempty"`
	OffsetMS     int64   `json:"offset_ms,omitempty"`
	Enabled      bool    `json:"enabled"`
	LoopStatus   string  `json:"loop_status,omitempty"`
	Volume       float64 `json:"volume,omitempty"`
	PositionMS   int64   `json:"position_ms,omitempty"`
	// Device names a system audio output; empty means the default output.
	Device  string                      `json:"device,omitempty"`
	Filters *models.NotificationFilters `json:"filters,omitempty"`
}

type Response struct {
	Error              string                         `json:"error,omitempty"`
	Event              string                         `json:"event,omitempty"`
	Device             *models.PairResult             `json:"device,omitempty"`
	Devices            []models.PairResult            `json:"devices,omitempty"`
	ConnectedDeviceIDs []string                       `json:"connected_device_ids,omitempty"`
	Discovered         []discovery.Device             `json:"discovered,omitempty"`
	Code               uint32                         `json:"code,omitempty"`
	PeerName           string                         `json:"peer_name,omitempty"`
	Players            []models.MPRISMediaPlayer      `json:"players,omitempty"`
	Player             *models.MPRISMediaPlayer       `json:"player,omitempty"`
	AudioDevices       []models.AudioOutputDevice     `json:"audio_devices,omitempty"`
	Settings           map[string]DeviceSettings      `json:"device_settings,omitempty"`
	Status             map[string]models.DeviceStatus `json:"device_status,omitempty"`
	Filters            *models.NotificationFilters    `json:"filters,omitempty"`
}

// DeviceSettings is the per-device configuration reported with a device list.
type DeviceSettings struct {
	NotificationSync string `json:"notification_sync"` // both, none, toClient, toHost
	ClipboardSync    bool   `json:"clipboard_sync"`
	InputEnabled     bool   `json:"input_enabled"`
	InputAuto        bool   `json:"input_auto"`   // switch input on whenever the phone connects
	AutoConnect      bool   `json:"auto_connect"` // connect on startup and whenever the phone appears
	SSHAllowed       bool   `json:"ssh_allowed"`  // may open an SSH tunnel; defaults to false
}

type ManagedSession struct {
	Cancel context.CancelFunc
	Active *session.Session
}
