package media

import (
	"errors"
	"fmt"
	"os"
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

func listWPCTLDevices() ([]models.AudioOutputDevice, error) {
	sinks, err := listAudioSinks()
	if err != nil {
		return nil, err
	}

	devices := make([]models.AudioOutputDevice, 0, len(sinks))
	for _, sink := range sinks {
		output, err := wpctl("get-volume", sink.ID)
		if err != nil {
			return nil, fmt.Errorf("audio: read volume for sink %q: %w", sink.Name, err)
		}

		volume, muted, err := parseWPCTLVolume(output)
		if err != nil {
			return nil, fmt.Errorf("audio: parse volume for sink %q: %w", sink.Name, err)
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
		return nil, errors.New("audio: no output sinks found in wpctl status")
	}
	return sinks, nil
}

func parseWPCTLVolume(output string) (float64, bool, error) {
	fields := strings.Fields(output)
	if len(fields) < 2 || fields[0] != "Volume:" {
		return 0, false, fmt.Errorf("audio: unexpected wpctl output %q", strings.TrimSpace(output))
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

func setWPCTLVolume(name string, volume float64) error {
	target, err := audioSinkID(name)
	if err != nil {
		return err
	}

	_, err = wpctl("set-volume", target, fmt.Sprintf("%.2f", volume))
	return err
}

func setWPCTLMuted(name string, muted bool) error {
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

func setWPCTLDefault(name string) error {
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
	return "", fmt.Errorf("audio: output device %q not found", name)
}

func wpctl(args ...string) (string, error) {
	output, err := exec.Command("wpctl", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("audio: wpctl %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// audioBackend identifies the sound server used for system audio control.
type audioBackend int

const (
	backendNone audioBackend = iota
	backendPipeWire
	backendPulse
)

// detectAudioBackend picks the backend per call so a sound-server restart or
// late startup is handled. WirePlumber's wpctl is preferred and must actually
// answer; otherwise pactl covers PulseAudio and pipewire-pulse.
func detectAudioBackend() audioBackend {
	if _, err := exec.LookPath("wpctl"); err == nil {
		if _, err := wpctl("status"); err == nil {
			return backendPipeWire
		}
	}
	if _, err := exec.LookPath("pactl"); err == nil {
		if _, err := pactl("info"); err == nil {
			return backendPulse
		}
	}
	return backendNone
}

var errNoAudioBackend = errors.New("audio: no supported audio server found (need wpctl or pactl)")

func ListAudioOutputDevices() ([]models.AudioOutputDevice, error) {
	switch detectAudioBackend() {
	case backendPipeWire:
		return listWPCTLDevices()
	case backendPulse:
		return listPactlDevices()
	}
	return nil, errNoAudioBackend
}

func SetOutputDeviceVolume(name string, volume float64) error {
	if volume < 0 || volume > 1 {
		return errors.New("audio: volume must be between 0.0 and 1.0")
	}
	switch detectAudioBackend() {
	case backendPipeWire:
		return setWPCTLVolume(name, volume)
	case backendPulse:
		return setPactlVolume(name, volume)
	}
	return errNoAudioBackend
}

func SetOutputDeviceMuted(name string, muted bool) error {
	switch detectAudioBackend() {
	case backendPipeWire:
		return setWPCTLMuted(name, muted)
	case backendPulse:
		return setPactlMuted(name, muted)
	}
	return errNoAudioBackend
}

func SetDefaultOutputDevice(name string) error {
	if name == "" {
		return errors.New("audio: output device name is required")
	}
	switch detectAudioBackend() {
	case backendPipeWire:
		return setWPCTLDefault(name)
	case backendPulse:
		return setPactlDefault(name)
	}
	return errNoAudioBackend
}

// pactl runs with the C locale so output labels are parseable.
func pactl(args ...string) (string, error) {
	cmd := exec.Command("pactl", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("audio: pactl %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

var pulsePercentRE = regexp.MustCompile(`(\d+)%`)

func listPactlDevices() ([]models.AudioOutputDevice, error) {
	info, err := pactl("info")
	if err != nil {
		return nil, err
	}
	defaultSink := ""
	for _, line := range strings.Split(info, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Default Sink:"); ok {
			defaultSink = strings.TrimSpace(v)
		}
	}

	detail, err := pactl("list", "sinks")
	if err != nil {
		return nil, err
	}

	var devices []models.AudioOutputDevice
	var current *models.AudioOutputDevice
	flush := func() {
		if current != nil && current.Name != "" {
			if current.Description == "" {
				current.Description = current.Name
			}
			current.IsDefault = current.Name == defaultSink
			devices = append(devices, *current)
		}
		current = nil
	}
	for _, line := range strings.Split(detail, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Sink #"):
			flush()
			current = &models.AudioOutputDevice{}
		case current == nil:
		case strings.HasPrefix(trimmed, "Name:"):
			current.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "Name:"))
		case strings.HasPrefix(trimmed, "Description:"):
			current.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "Description:"))
		case strings.HasPrefix(trimmed, "Mute:"):
			current.Muted = strings.TrimSpace(strings.TrimPrefix(trimmed, "Mute:")) == "yes"
		case strings.HasPrefix(trimmed, "Volume:") && current.Volume == 0:
			if m := pulsePercentRE.FindStringSubmatch(trimmed); len(m) == 2 {
				pct, _ := strconv.ParseFloat(m[1], 64)
				current.Volume = min(max(pct/100, 0), 1)
			}
		}
	}
	flush()

	if len(devices) == 0 {
		return nil, errors.New("audio: no output sinks found via pactl")
	}
	return devices, nil
}

// pulseSinkName resolves a requested name (or empty for the default sink)
// to an existing sink name so unknown devices fail with a clear error.
func pulseSinkName(name string) (string, error) {
	devices, err := listPactlDevices()
	if err != nil {
		return "", err
	}
	for _, d := range devices {
		if (name == "" && d.IsDefault) || (name != "" && d.Name == name) {
			return d.Name, nil
		}
	}
	return "", fmt.Errorf("audio: output device %q not found", name)
}

func setPactlVolume(name string, volume float64) error {
	target, err := pulseSinkName(name)
	if err != nil {
		return err
	}
	_, err = pactl("set-sink-volume", target, fmt.Sprintf("%d%%", int(volume*100+0.5)))
	return err
}

func setPactlMuted(name string, muted bool) error {
	target, err := pulseSinkName(name)
	if err != nil {
		return err
	}
	value := "0"
	if muted {
		value = "1"
	}
	_, err = pactl("set-sink-mute", target, value)
	return err
}

func setPactlDefault(name string) error {
	target, err := pulseSinkName(name)
	if err != nil {
		return err
	}
	_, err = pactl("set-default-sink", target)
	return err
}
