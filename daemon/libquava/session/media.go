package session

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"libquava/media"
	"libquava/models"
	"libquava/protocol"

	"golang.org/x/image/draw"
)

// MediaCommand completely describes a host-media operation. An empty BusName
// targets the active player, whose selection is owned by this package.
type MediaCommand struct {
	Command    string
	BusName    string
	OffsetMS   int64
	Enabled    bool
	LoopStatus string
	Volume     float64
}

var hostMedia struct {
	sync.RWMutex
	service *media.MPRISService
}

// StartMedia starts the host MPRIS integration. Session-bus startup failure is
// returned to the caller so it can remain non-fatal to other daemon services.
func StartMedia(ctx context.Context) error {
	hostMedia.Lock()
	defer hostMedia.Unlock()
	if hostMedia.service != nil {
		return nil
	}

	service := media.NewMPRISService()
	if err := service.Start(ctx); err != nil {
		return err
	}

	hostMedia.service = service
	return nil
}

// StopMedia releases the MPRIS connection and player cache.
func StopMedia() error {
	hostMedia.Lock()
	service := hostMedia.service
	hostMedia.service = nil
	hostMedia.Unlock()
	if service == nil {
		return nil
	}

	return service.Stop()
}

func mediaService() (*media.MPRISService, error) {
	hostMedia.RLock()
	service := hostMedia.service
	hostMedia.RUnlock()
	if service == nil {
		return nil, fmt.Errorf("MPRIS service is unavailable")
	}

	return service, nil
}

func ListMediaPlayers() ([]models.MPRISMediaPlayer, error) {
	service, err := mediaService()
	if err != nil {
		return nil, err
	}

	return service.ListPlayers(), nil
}

func ActiveMediaPlayer() (models.MPRISMediaPlayer, error) {
	service, err := mediaService()
	if err != nil {
		return models.MPRISMediaPlayer{}, err
	}

	return service.ActivePlayer()
}

func SetActiveMediaPlayer(busName string) (models.MPRISMediaPlayer, error) {
	service, err := mediaService()
	if err != nil {
		return models.MPRISMediaPlayer{}, err
	}

	busName = strings.TrimSpace(busName)
	if busName == "" {
		return models.MPRISMediaPlayer{}, fmt.Errorf("bus_name is required")
	}

	if err := service.SetActivePlayer(busName); err != nil {
		return models.MPRISMediaPlayer{}, err
	}

	return service.GetPlayer(busName)
}

// ControlMedia validates, resolves the target player, and executes a host
// MPRIS action. Daemon callers only pass the command through.
func ControlMedia(command MediaCommand) error {
	service, err := mediaService()
	if err != nil {
		return err
	}

	busName := strings.TrimSpace(command.BusName)
	if busName == "" {
		player, err := service.ActivePlayer()
		if err != nil {
			return err
		}
		busName = player.BusName
	}

	var actionErr error
	switch command.Command {
	case "media-play-pause":
		actionErr = service.PlayPause(busName)

	case "media-next":
		actionErr = service.Next(busName)

	case "media-previous":
		actionErr = service.Previous(busName)

	case "media-play":
		actionErr = service.Play(busName)

	case "media-pause":
		actionErr = service.Pause(busName)

	case "media-stop":
		actionErr = service.StopPlayback(busName)

	case "media-seek":
		actionErr = service.Seek(busName, time.Duration(command.OffsetMS)*time.Millisecond)

	case "media-shuffle":
		actionErr = service.SetShuffle(busName, command.Enabled)

	case "media-loop":
		if command.LoopStatus != "None" && command.LoopStatus != "Track" && command.LoopStatus != "Playlist" {
			return fmt.Errorf("invalid loop status %q (use None, Track, or Playlist)", command.LoopStatus)
		}

		actionErr = service.SetLoopStatus(busName, command.LoopStatus)

	case "media-volume":
		if command.Volume < 0 || command.Volume > 1 {
			return fmt.Errorf("volume must be between 0.0 and 1.0")
		}

		actionErr = service.SetPlayerVolume(busName, command.Volume)

	default:
		return fmt.Errorf("unsupported media command %q", command.Command)
	}

	if actionErr != nil {
		return actionErr
	}

	return service.Refresh()
}

// handleMultimediaCommand accepts the compact Android control payload and
// delegates every policy and MPRIS decision to ControlMedia.
func (s *Session) handleMultimediaCommand(ctx context.Context, msg protocol.Message) error {
	command, err := protocol.PayloadString(msg.Payload, 1)
	if err != nil {
		log.Printf("session: malformed multimedia command: %v", err)
		return s.ackMultimedia(ctx, msg, err)
	}

	log.Printf("session: received multimedia command=%q", command)
	if command == "state" {
		return s.ackMultimedia(ctx, msg, nil)
	}

	if command == "system-state" || command == "system-audio-state" ||
		command == "system-volume" || command == "system-mute" ||
		command == "system-audio-volume" || command == "system-audio-mute" ||
		command == "system-audio-default" {

		deviceName, _ := msg.Payload[2].(string)
		var commandErr error

		switch command {
		case "system-volume", "system-audio-volume":
			value, ok := msg.Payload[6].(float64)
			if !ok {
				commandErr = fmt.Errorf("missing or invalid system volume")
			} else {
				commandErr = SetOutputDeviceVolume(deviceName, value)
			}

		case "system-mute", "system-audio-mute":
			value, ok := msg.Payload[4].(bool)
			if !ok {
				commandErr = fmt.Errorf("missing or invalid mute value")
			} else {
				commandErr = SetOutputDeviceMuted(deviceName, value)
			}

		case "system-audio-default":
			commandErr = SetDefaultOutputDevice(deviceName)
		}

		return s.ackMultimedia(ctx, msg, commandErr)
	}
	request := MediaCommand{Command: "media-" + command}
	if value, ok := msg.Payload[2].(string); ok {
		request.BusName = value
	}

	if value, ok := msg.Payload[3].(int64); ok {
		request.OffsetMS = value
	} else if value, ok := msg.Payload[3].(uint64); ok {
		request.OffsetMS = int64(value)
	}

	if value, ok := msg.Payload[4].(bool); ok {
		request.Enabled = value
	}

	if value, ok := msg.Payload[5].(string); ok {
		request.LoopStatus = value
	}

	if value, ok := msg.Payload[6].(float64); ok {
		request.Volume = value
	}

	commandErr := ControlMedia(request)
	if commandErr != nil {
		return s.ackMultimedia(ctx, msg, commandErr)
	}

	if err := s.ackMultimedia(ctx, msg, nil); err != nil {
		return err
	}
	return nil
}

func (s *Session) ackMultimedia(ctx context.Context, msg protocol.Message, commandErr error) error {
	payload := map[uint64]any{0: commandErr == nil}
	if commandErr != nil {
		payload[1] = commandErr.Error()
	}

	// Media-player state is optional. Its absence must not reject
	// an otherwise successful system-audio command.
	if player, err := ActiveMediaPlayer(); err == nil {
		payload[2] = player.BusName
		payload[3] = player.Identity
		payload[4] = player.PlaybackStatus
		payload[5] = player.Metadata.Title
		payload[6] = player.Metadata.Artists
		payload[7] = player.Metadata.Album
		payload[8] = player.Metadata.ArtworkURL
		payload[9] = player.Metadata.Duration
		payload[10] = player.Position
		payload[11] = player.Volume
		payload[12] = player.Shuffle
		payload[13] = player.LoopStatus
		payload[14] = player.CanGoPrevious
		payload[15] = player.CanGoNext
		payload[16] = player.CanSeek
		payload[17] = player.CanPlay
		payload[18] = player.CanPause

		if artwork, artworkErr := mediaArtwork(player.Metadata.ArtworkURL); artworkErr == nil {
			payload[19] = artwork
		} else if player.Metadata.ArtworkURL != "" {
			log.Printf("session: media artwork unavailable: %v", artworkErr)
		}
	}

	// Audio state is independent of whether a media player is active.
	if audio, err := SystemAudioState(); err == nil {
		payload[30] = audio.Volume
		payload[31] = audio.Muted
		payload[32] = audio.OutputDevice

		devices := make([]any, 0, len(audio.AvailableOutputDevices))
		for _, device := range audio.AvailableOutputDevices {
			devices = append(devices, map[uint64]any{
				0: device.Name,
				1: device.Description,
				2: device.Volume,
				3: device.Muted,
				4: device.IsDefault,
			})
		}
		payload[33] = devices
	} else {
		log.Printf("session: system audio state unavailable: %v", err)
	}

	ack := protocol.NewMessage(
		protocol.MessageTypeMultimediaAck,
		msg.TransactionID,
		payload,
	)

	s.pingMutex.Lock()
	err := s.conn.WriteMessage(ctx, ack)
	s.pingMutex.Unlock()
	if err != nil {
		return fmt.Errorf("session: send multimedia acknowledgement: %w", err)
	}

	return commandErr
}

func mediaArtwork(rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid artwork URL")
	}

	var data []byte
	switch u.Scheme {
	case "file":
		data, err = os.ReadFile(u.Path)

	case "http", "https":
		client := &http.Client{Timeout: 5 * time.Second}
		response, requestErr := client.Get(rawURL)
		if requestErr != nil {
			return nil, requestErr
		}

		defer func() {
			if err := response.Body.Close(); err != nil {
				log.Printf("failed to close artwork connection: %v", err)
			}
		}()

		if response.StatusCode < 200 || response.StatusCode > 299 {
			return nil, fmt.Errorf("artwork request returned %s", response.Status)
		}

		data, err = io.ReadAll(io.LimitReader(response.Body, 512*1024+1))

	default:
		return nil, fmt.Errorf("unsupported artwork URL scheme")
	}

	if err != nil {
		return nil, err
	}

	// Compress the image
	// TODO: There's no way recompressing this image every single poll is good. Look at project readme.
	data, err = compressArtwork(data)
	if err != nil {
		return nil, err
	}

	// fmt.Printf("artwork size: %d\n", len(data)/int(reflect.TypeOf(data).Size()))

	// The session protocol caps an entire frame at 64 KiB. Leave room for
	// metadata and CBOR overhead; Android can load an oversized HTTP(S) URL
	// itself, while local file artwork simply falls back to the placeholder.
	if len(data) > 48*1024 {
		return nil, fmt.Errorf("artwork exceeds transfer limit")
	}

	return data, nil
}

func compressArtwork(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode artwork: %w", err)
	}

	// Start at the original dimensions, capped at 512x512.
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid artwork dimensions")
	}

	const maxDimension = 512
	if width > maxDimension || height > maxDimension {
		scale := float64(maxDimension) / float64(max(width, height))
		width = max(1, int(float64(width)*scale))
		height = max(1, int(float64(height)*scale))

		resized := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(
			resized, resized.Bounds(),
			img, bounds, draw.Over, nil,
		)
		img = resized
	}

	// Try progressively lower JPEG qualities and resolutions.
	for {
		for _, quality := range []int{85, 75, 65, 55, 45, 35} {
			var buf bytes.Buffer

			if err := jpeg.Encode(&buf, img, &jpeg.Options{
				Quality: quality,
			}); err != nil {
				return nil, fmt.Errorf("encode artwork: %w", err)
			}

			if buf.Len() <= 48*1024 {
				return buf.Bytes(), nil
			}
		}

		bounds := img.Bounds()
		width, height := bounds.Dx(), bounds.Dy()

		if width <= 64 && height <= 64 {
			return nil, fmt.Errorf("artwork cannot fit transfer limit")
		}

		width = max(1, width*3/4)
		height = max(1, height*3/4)

		resized := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(
			resized, resized.Bounds(),
			img, bounds, draw.Over, nil,
		)
		img = resized
	}
}
