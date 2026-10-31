package models

import (
	"libquava/discovery"
	"libquava/models"
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
	Error      string                    `json:"error,omitempty"`
	Event      string                    `json:"event,omitempty"`
	Device     *models.PairResult        `json:"device,omitempty"`
	Devices    []models.PairResult       `json:"devices,omitempty"`
	Discovered []discovery.Device        `json:"discovered,omitempty"`
	Code       uint32                    `json:"code,omitempty"`
	PeerName   string                    `json:"peer_name,omitempty"`
	Players    []models.MPRISMediaPlayer `json:"players,omitempty"`
	Player     *models.MPRISMediaPlayer  `json:"player,omitempty"`

	ConnectedDeviceIDs []string                    `json:"connected_device_ids,omitempty"`
	AudioDevices       []models.AudioOutputDevice  `json:"audio_devices,omitempty"`
	Filters            *models.NotificationFilters `json:"filters,omitempty"`
}

// NotificationFilters is the host's Android-notification filter rules.
type NotificationFilters = models.NotificationFilters
