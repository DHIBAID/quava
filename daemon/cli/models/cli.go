package models

import (
	"libquava/discovery"
	"libquava/models"
)

const ControlSocketName = "quavad.sock"

type Request struct {
	Command      string `json:"command"`
	PeerDeviceID string `json:"peer_device_id,omitempty"`
	TimeoutMS    int64  `json:"timeout_ms,omitempty"`
	Confirm      *bool  `json:"confirm,omitempty"`
}

type Response struct {
	Error      string              `json:"error,omitempty"`
	Event      string              `json:"event,omitempty"`
	Device     *models.PairResult  `json:"device,omitempty"`
	Devices    []models.PairResult `json:"devices,omitempty"`
	Discovered []discovery.Device  `json:"discovered,omitempty"`
	Code       uint32              `json:"code,omitempty"`
	PeerName   string              `json:"peer_name,omitempty"`
}
