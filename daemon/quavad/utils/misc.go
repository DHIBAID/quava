package utils

import (
	"os"
	"strings"
	"time"
)

func Duration(milliseconds int64, fallback time.Duration) time.Duration {
	if milliseconds <= 0 {
		return fallback
	}

	return time.Duration(milliseconds) * time.Millisecond
}

func LocalDeviceName() string {
	name, err := os.Hostname()

	if err != nil || strings.TrimSpace(name) == "" {
		return "Quava Linux"
	}

	return name
}
