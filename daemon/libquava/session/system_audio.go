package session

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"libquava/models"
)

var (
	sinkLineRE = regexp.MustCompile(`(\*)?\s*(\d+)\.\s+(\S+)`)
	propRE     = regexp.MustCompile(`^\s*\*?\s*([a-zA-Z0-9_.-]+)\s*=\s*"(.*)"\s*$`)
)

type audioSinkRef struct {
	ID          string
	Name        string
	IsDefault   bool
	Description string
}

func SystemAudioState() (models.SystemAudioState, error) {
	devices, err := ListAudioOutputDevices()
	if err != nil {
		return models.SystemAudioState{}, err
	}

	state := models.SystemAudioState{
		AvailableOutputDevices: devices,
	}
	for _, device := range devices {
		if device.IsDefault {
			state.OutputDevice = device.Name
			state.Volume = device.Volume
			state.Muted = device.Muted
			break
		}
	}
	return state, nil
}

func ListAudioOutputDevices() ([]models.AudioOutputDevice, error) {
	sinks, err := listAudioSinks()
	if err != nil {
		return nil, err
	}

	devices := make([]models.AudioOutputDevice, 0, len(sinks))
	for _, sink := range sinks {
		output, err := wpctl("get-volume", sink.ID)
		if err != nil {
			return nil, fmt.Errorf("read volume for sink %q: %w", sink.Name, err)
		}

		volume, muted, err := parseWPCTLVolume(output)
		if err != nil {
			return nil, fmt.Errorf("parse volume for sink %q: %w", sink.Name, err)
		}

		description := sink.Description
		if description == "" {
			description = sink.Name
		}

		devices = append(devices, models.AudioOutputDevice{
			Name:        sink.Name,
			Description: description,
			Volume:      volume,
			Muted:       muted,
			IsDefault:   sink.IsDefault,
		})
	}
	return devices, nil
}

func listAudioSinks() ([]audioSinkRef, error) {
	output, err := wpctl("status", "-n")
	if err != nil {
		return nil, err
	}

	var sinks []audioSinkRef
	inAudio := false
	inSinks := false

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)

		if trimmed == "Audio" {
			inAudio = true
			continue
		}
		if trimmed == "Video" || trimmed == "Settings" {
			inAudio = false
			inSinks = false
			continue
		}
		if !inAudio {
			continue
		}

		if strings.Contains(line, "Sinks:") {
			inSinks = true
			continue
		}
		if inSinks && (strings.Contains(line, "Sources:") ||
			strings.Contains(line, "Filters:") ||
			strings.Contains(line, "Streams:")) {
			inSinks = false
			continue
		}
		if !inSinks {
			continue
		}

		match := sinkLineRE.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}

		sink := audioSinkRef{
			ID:        match[2],
			Name:      match[3],
			IsDefault: match[1] == "*",
		}

		properties, err := wpctl("inspect", sink.ID)
		if err == nil {
			sink.Description = parseWPCTLProperty(properties, "node.description")
		}

		sinks = append(sinks, sink)
	}

	if len(sinks) == 0 {
		return nil, fmt.Errorf("no audio output sinks found in wpctl status")
	}
	return sinks, nil
}

func parseWPCTLVolume(output string) (float64, bool, error) {
	fields := strings.Fields(output)
	if len(fields) < 2 || fields[0] != "Volume:" {
		return 0, false, fmt.Errorf("unexpected wpctl output %q", strings.TrimSpace(output))
	}

	volume, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, false, err
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}

	return volume, strings.Contains(output, "[MUTED]"), nil
}

func parseWPCTLProperty(output, key string) string {
	for _, line := range strings.Split(output, "\n") {
		match := propRE.FindStringSubmatch(line)
		if len(match) == 3 && match[1] == key {
			return match[2]
		}
	}
	return ""
}

func SetOutputDeviceVolume(name string, volume float64) error {
	if volume < 0 || volume > 1 {
		return fmt.Errorf("system volume must be between 0.0 and 1.0")
	}

	target, err := audioSinkID(name)
	if err != nil {
		return err
	}

	_, err = wpctl("set-volume", target, fmt.Sprintf("%.2f", volume))
	return err
}

func SetOutputDeviceMuted(name string, muted bool) error {
	target, err := audioSinkID(name)
	if err != nil {
		return err
	}

	value := "0"
	if muted {
		value = "1"
	}
	_, err = wpctl("set-mute", target, value)
	return err
}

func SetDefaultOutputDevice(name string) error {
	if name == "" {
		return fmt.Errorf("output device name is required")
	}
	target, err := audioSinkID(name)
	if err != nil {
		return err
	}
	_, err = wpctl("set-default", target)
	return err
}

func audioSinkID(name string) (string, error) {
	sinks, err := listAudioSinks()
	if err != nil {
		return "", err
	}

	for _, sink := range sinks {
		if name == "" && sink.IsDefault {
			return sink.ID, nil
		}
		if name != "" && (sink.Name == name || sink.ID == name) {
			return sink.ID, nil
		}
	}
	return "", fmt.Errorf("audio output device %q not found", name)
}

func wpctl(args ...string) (string, error) {
	output, err := exec.Command("wpctl", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("wpctl %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
