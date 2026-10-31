package models

import "time"

// DeviceStatus is what a phone reports about itself while connected: battery,
// radios and storage. Unknown numbers are -1 rather than 0, so "no signal"
// and "not reported" stay different things.
type DeviceStatus struct {
	BatteryPercent int     `json:"battery_percent"`
	BatteryState   string  `json:"battery_state"`  // charging, discharging, full, unknown
	Plug           string  `json:"plug,omitempty"` // ac, usb, wireless
	BatteryTempC   float64 `json:"battery_temp_c,omitempty"`

	Network        string `json:"network"` // wifi, mobile, ethernet, none
	WifiLevel      int    `json:"wifi_level"`
	WifiRSSI       int    `json:"wifi_rssi,omitempty"`
	CellGeneration string `json:"cell_generation,omitempty"` // 2G, 3G, 4G, 5G
	CellLevel      int    `json:"cell_level"`

	Bluetooth bool `json:"bluetooth"`
	Airplane  bool `json:"airplane"`

	StorageFree  int64 `json:"storage_free"`
	StorageTotal int64 `json:"storage_total"`

	Model          string    `json:"model,omitempty"`
	AndroidVersion string    `json:"android_version,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}
