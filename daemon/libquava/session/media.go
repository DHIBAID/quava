package session

import (
	"bytes"
	"container/list"
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

	qcrypto "libquava/crypto"
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
	OffsetMS   int64 // relative offset for media-seek
	PositionMS int64 // absolute target for media-seek-to
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

// startMultimediaSync sends the session's initial state and relays subsequent
// MPRIS events until the connection-specific context is cancelled.
func (s *Session) startMultimediaSync(ctx context.Context) {
	s.multimediaOnce.Do(func() {
		s.startMultimediaSyncOnce(ctx)
	})
}

func (s *Session) startMultimediaSyncOnce(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}

	service, err := mediaService()
	if err != nil {
		// Media support is optional; the initial state still included any system
		// audio information that was available.
		if err := s.sendMultimediaState(ctx); err != nil {
			log.Printf("session: send initial multimedia state: %v", err)
		}
		return
	}

	events, unsubscribe := service.Subscribe()
	if err := ctx.Err(); err != nil {
		unsubscribe()
		return
	}
	// Subscribe before the initial snapshot so changes in this initialization
	// window remain buffered and are sent only after the full state.
	if err := s.sendMultimediaState(ctx); err != nil {
		unsubscribe()
		log.Printf("session: send initial multimedia state: %v", err)
		return
	}

	s.multimediaWG.Add(1)
	go func() {
		defer s.multimediaWG.Done()
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				if err := s.sendMultimediaEvent(ctx, event); err != nil {
					log.Printf("session: send multimedia update: %v", err)
					return
				}
			}
		}
	}()
}

func (s *Session) waitMultimediaSync() {
	s.multimediaWG.Wait()
}

func (s *Session) sendMultimediaEvent(ctx context.Context, event media.MediaEvent) error {
	if event.Type == media.MediaEventPositionChanged {
		// Compare against the live active player: the event's snapshot may
		// predate an active-player change and carry a stale IsActive flag.
		active, err := ActiveMediaPlayer()
		if event.State == nil || err != nil || active.BusName != event.PlayerID {
			return nil
		}
		return s.sendMultimediaPosition(ctx, event.State.Position)
	}
	return s.sendMultimediaState(ctx)
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

	case "media-seek-to":
		actionErr = service.SeekTo(busName, time.Duration(command.PositionMS)*time.Millisecond)

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

	return nil
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
		if err := s.ackMultimedia(ctx, msg, nil); err != nil {
			return err
		}
		return s.sendMultimediaState(ctx)
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
				commandErr = media.SetOutputDeviceVolume(deviceName, value)
			}

		case "system-mute", "system-audio-mute":
			value, ok := msg.Payload[4].(bool)
			if !ok {
				commandErr = fmt.Errorf("missing or invalid mute value")
			} else {
				commandErr = media.SetOutputDeviceMuted(deviceName, value)
			}

		case "system-audio-default":
			commandErr = media.SetDefaultOutputDevice(deviceName)
		}

		return s.ackMultimedia(ctx, msg, commandErr)
	}
	request := MediaCommand{Command: "media-" + command}
	if value, ok := msg.Payload[2].(string); ok {
		request.BusName = value
	}

	if value, ok := msg.Payload[3].(int64); ok {
		request.OffsetMS, request.PositionMS = value, value
	} else if value, ok := msg.Payload[3].(uint64); ok {
		request.OffsetMS, request.PositionMS = int64(value), int64(value)
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

func (s *Session) sendMultimediaState(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload := map[uint64]any{
		protocol.MultimediaStateUpdateType: protocol.MultimediaStateFull,
	}

	// Media player state (optional)
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

		if artwork, err := mediaArtwork(ctx, player.Metadata.ArtworkURL); err == nil {
			payload[19] = artwork
		}
	}

	// System audio state (independent of media player)
	if audio, err := media.SystemAudioState(); err == nil {
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
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	transactionID, err := qcrypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	msg := protocol.NewMessage(protocol.MessageTypeMultimediaState, transactionID, payload)

	s.pingMutex.Lock()
	err = s.conn.WriteMessage(ctx, msg)
	s.pingMutex.Unlock()

	if err != nil {
		return fmt.Errorf("session: send multimedia state: %w", err)
	}

	return nil
}

// sendMultimediaPosition sends only the active player's current position. The
// explicit update kind prevents receivers from treating this as a partial full
// state update and clearing metadata or capabilities.
func (s *Session) sendMultimediaPosition(ctx context.Context, position int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	transactionID, err := qcrypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	msg := protocol.NewMessage(protocol.MessageTypeMultimediaState, transactionID, map[uint64]any{
		protocol.MultimediaStateUpdateType: protocol.MultimediaStatePosition,
		10:                                 position,
	})

	s.pingMutex.Lock()
	err = s.conn.WriteMessage(ctx, msg)
	s.pingMutex.Unlock()
	if err != nil {
		return fmt.Errorf("session: send multimedia position: %w", err)
	}

	return nil
}

func (s *Session) ackMultimedia(ctx context.Context, msg protocol.Message, commandErr error) error {
	payload := map[uint64]any{
		0: commandErr == nil,
	}

	if commandErr != nil {
		payload[1] = commandErr.Error()
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

func mediaArtwork(ctx context.Context, rawURL string) ([]byte, error) {
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
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		response, requestErr := client.Do(request)
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
	data, err = compressArtwork(rawURL, data)
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

const artworkCacheCapacity = 5

type artworkCacheEntry struct {
	key  string
	data []byte
}

type artworkLRU struct {
	mu    sync.Mutex
	items map[string]*list.Element
	order *list.List
}

func newArtworkLRU() *artworkLRU {
	return &artworkLRU{
		items: make(map[string]*list.Element, artworkCacheCapacity),
		order: list.New(),
	}
}

func (c *artworkLRU) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.items[key]
	if !ok {
		return nil, false
	}

	c.order.MoveToFront(element)
	return element.Value.(*artworkCacheEntry).data, true
}

func (c *artworkLRU) Add(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.items[key]; ok {
		c.order.MoveToFront(element)
		element.Value.(*artworkCacheEntry).data = data
		return
	}

	element := c.order.PushFront(&artworkCacheEntry{
		key:  key,
		data: data,
	})

	c.items[key] = element

	if c.order.Len() > artworkCacheCapacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*artworkCacheEntry).key)
	}
}

var compressedArtworkCache = newArtworkLRU()

func compressArtwork(cacheKey string, data []byte) ([]byte, error) {
	const (
		maxDimension = 512
		maxSize      = 48 * 1024
		minDimension = 64
	)

	// Return cached artwork without decoding or recompressing.
	if cached, ok := compressedArtworkCache.Get(cacheKey); ok {
		return cached, nil
	}

	// Inspect dimensions before decoding the full image.
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode artwork config: %w", err)
	}

	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("invalid artwork dimensions")
	}

	// Fast path: already-compliant JPEG.
	if format == "jpeg" && cfg.Width <= maxDimension && cfg.Height <= maxDimension && len(data) <= maxSize {
		compressedArtworkCache.Add(cacheKey, data)
		return data, nil
	}
	// Protect against excessively large decoded images.
	const maxPixels = 16_000_000

	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("artwork dimensions exceed pixel limit")
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode artwork: %w", err)
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	// Resize to maximum dimensions once, if needed.
	if width > maxDimension || height > maxDimension {
		scale := float64(maxDimension) /
			float64(max(width, height))

		width = max(1, int(float64(width)*scale))
		height = max(1, int(float64(height)*scale))

		resized := image.NewRGBA(image.Rect(0, 0, width, height))

		draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)

		img = resized
	}

	for {
		for _, quality := range []int{75, 60, 45, 35} {
			var buf bytes.Buffer

			if err := jpeg.Encode(&buf, img, &jpeg.Options{
				Quality: quality,
			}); err != nil {
				return nil, fmt.Errorf("encode artwork: %w", err)
			}

			if buf.Len() <= maxSize {
				result := buf.Bytes()
				compressedArtworkCache.Add(cacheKey, result)
				return result, nil
			}
		}

		bounds := img.Bounds()
		width, height := bounds.Dx(), bounds.Dy()

		if width <= minDimension && height <= minDimension {
			return nil, fmt.Errorf("artwork cannot fit transfer limit")
		}

		width = max(minDimension, width*3/4)
		height = max(minDimension, height*3/4)

		resized := image.NewRGBA(image.Rect(0, 0, width, height))

		draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)

		img = resized
	}
}
