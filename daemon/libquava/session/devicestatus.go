package session

import (
	"log"
	"time"

	"libquava/models"
	"libquava/protocol"
)

// DeviceStatus returns the latest status the peer reported, if it has
// reported one on this connection.
func (s *Session) DeviceStatus() (models.DeviceStatus, bool) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()

	if s.deviceStatus == nil {
		return models.DeviceStatus{}, false
	}

	return *s.deviceStatus, true
}

func (s *Session) handleDeviceStatus(msg protocol.Message) {
	p := msg.Payload

	status := models.DeviceStatus{
		BatteryPercent: payloadInt(p, 0, -1),
		BatteryState:   payloadText(p, 1),
		Plug:           payloadText(p, 2),
		BatteryTempC:   payloadFloat(p, 3),
		Network:        payloadText(p, 4),
		WifiLevel:      payloadInt(p, 5, -1),
		WifiRSSI:       payloadInt(p, 6, 0),
		CellGeneration: payloadText(p, 7),
		CellLevel:      payloadInt(p, 8, -1),
		Bluetooth:      payloadFlag(p, 9),
		Airplane:       payloadFlag(p, 10),
		StorageFree:    int64(payloadInt(p, 11, -1)),
		StorageTotal:   int64(payloadInt(p, 12, -1)),
		Model:          payloadText(p, 13),
		AndroidVersion: payloadText(p, 14),
		UpdatedAt:      time.Now(),
	}

	s.statusMu.Lock()
	previous := s.deviceStatus
	s.deviceStatus = &status
	s.statusMu.Unlock()

	if previous == nil || statusChanged(*previous, status) {
		log.Printf("session: phone status: battery=%d%% (%s) network=%s wifi=%d mobile=%s/%d bluetooth=%t airplane=%t",
			status.BatteryPercent, status.BatteryState, status.Network, status.WifiLevel,
			status.CellGeneration, status.CellLevel, status.Bluetooth, status.Airplane)
	}
}

func statusChanged(a, b models.DeviceStatus) bool {
	a.UpdatedAt, b.UpdatedAt = time.Time{}, time.Time{}
	return a != b
}

func payloadInt(payload map[uint64]any, key uint64, fallback int) int {
	switch value := payload[key].(type) {
	case int64:
		return int(value)
	case uint64:
		return int(value)
	case int:
		return value
	case float64:
		return int(value)
	}

	return fallback
}

func payloadFloat(payload map[uint64]any, key uint64) float64 {
	switch value := payload[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int64:
		return float64(value)
	case uint64:
		return float64(value)
	}

	return 0
}

func payloadText(payload map[uint64]any, key uint64) string {
	value, _ := payload[key].(string)
	return value
}

func payloadFlag(payload map[uint64]any, key uint64) bool {
	value, _ := payload[key].(bool)
	return value
}
