package media

import (
	"context"
	"fmt"
	"libquava/models"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	mprisPrefix = "org.mpris.MediaPlayer2."
	mprisRoot   = "org.mpris.MediaPlayer2"
	mprisPlayer = "org.mpris.MediaPlayer2.Player"
	dbusProps   = "org.freedesktop.DBus.Properties"
	dbusIface   = "org.freedesktop.DBus"
	mprisPath   = dbus.ObjectPath("/org/mpris/MediaPlayer2")

	refreshInterval = 1 * time.Second
	callTimeout     = 3 * time.Second
)

// MPRISService discovers session-bus media players and controls playback.
// We love dbus :)
type MPRISService struct {
	mu           sync.RWMutex
	conn         *dbus.Conn
	players      map[string]models.MPRISMediaPlayer
	activePlayer string

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	signals chan *dbus.Signal
	started bool
	stopped bool
}

func NewMPRISService() *MPRISService {
	return &MPRISService{
		players: make(map[string]models.MPRISMediaPlayer),
		signals: make(chan *dbus.Signal, 128),
	}
}

func (s *MPRISService) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("MPRIS: nil context")
	}

	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("MPRIS service cannot be started")
	}
	s.mu.Unlock()

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("MPRIS: connect to session bus: %w", err)
	}

	serviceCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.conn, s.ctx, s.cancel, s.started = conn, serviceCtx, cancel, true
	s.mu.Unlock()

	nameMatch := []dbus.MatchOption{
		dbus.WithMatchInterface(dbusIface),
		dbus.WithMatchMember("NameOwnerChanged"),
	}
	propsMatch := []dbus.MatchOption{
		dbus.WithMatchInterface(dbusProps),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath(mprisPath),
	}

	if err := conn.AddMatchSignal(nameMatch...); err != nil {
		cancel()
		_ = conn.Close()
		s.mu.Lock()
		s.conn, s.started = nil, false
		s.mu.Unlock()
		return fmt.Errorf("MPRIS: subscribe to name changes: %w", err)
	}
	if err := conn.AddMatchSignal(propsMatch...); err != nil {
		_ = conn.RemoveMatchSignal(nameMatch...)
		cancel()
		_ = conn.Close()
		s.mu.Lock()
		s.conn, s.started = nil, false
		s.mu.Unlock()
		return fmt.Errorf("MPRIS: subscribe to property changes: %w", err)
	}

	conn.Signal(s.signals)
	if err := s.refreshPlayers(); err != nil {
		conn.RemoveSignal(s.signals)
		_ = conn.RemoveMatchSignal(nameMatch...)
		_ = conn.RemoveMatchSignal(propsMatch...)
		cancel()
		_ = conn.Close()
		s.mu.Lock()
		s.conn, s.started = nil, false
		s.mu.Unlock()
		return fmt.Errorf("MPRIS: initial discovery: %w", err)
	}

	s.wg.Add(1)
	go s.monitor()
	return nil
}

func (s *MPRISService) Stop() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	cancel, conn := s.cancel, s.conn
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if conn != nil {
		conn.RemoveSignal(s.signals)
		_ = conn.RemoveMatchSignal(
			dbus.WithMatchInterface(dbusIface),
			dbus.WithMatchMember("NameOwnerChanged"),
		)
		_ = conn.RemoveMatchSignal(
			dbus.WithMatchInterface(dbusProps),
			dbus.WithMatchMember("PropertiesChanged"),
			dbus.WithMatchObjectPath(mprisPath),
		)
	}

	s.wg.Wait()

	var err error
	if conn != nil {
		err = conn.Close()
	}

	s.mu.Lock()
	s.conn = nil
	s.players = make(map[string]models.MPRISMediaPlayer)
	s.activePlayer = ""
	s.mu.Unlock()

	if err != nil {
		return fmt.Errorf("MPRIS: close session bus: %w", err)
	}
	return nil
}

func (s *MPRISService) ListPlayers() []models.MPRISMediaPlayer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	players := make([]models.MPRISMediaPlayer, 0, len(s.players))
	for _, player := range s.players {
		players = append(players, player)
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].BusName < players[j].BusName
	})
	return players
}

// Refresh updates the cached player state immediately after a control action.
func (s *MPRISService) Refresh() error { return s.refreshPlayers() }

func (s *MPRISService) GetPlayer(busName string) (models.MPRISMediaPlayer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	player, ok := s.players[busName]
	if !ok {
		return models.MPRISMediaPlayer{}, fmt.Errorf("MPRIS player %q not found", busName)
	}
	return player, nil
}

func (s *MPRISService) ActivePlayer() (models.MPRISMediaPlayer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	player, ok := s.players[s.activePlayer]
	if !ok {
		return models.MPRISMediaPlayer{}, fmt.Errorf("no active MPRIS player")
	}
	return player, nil
}

func (s *MPRISService) SetActivePlayer(busName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.players[busName]; !ok {
		return fmt.Errorf("MPRIS player %q not found", busName)
	}
	s.activePlayer = busName
	for name, player := range s.players {
		player.IsActive = name == busName
		s.players[name] = player
	}
	return nil
}

func (s *MPRISService) monitor() {
	defer s.wg.Done()
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case signal := <-s.signals:
			if signal != nil {
				_ = s.refreshPlayers()
			}
		case <-ticker.C:
			_ = s.refreshPlayers()
		}
	}
}

// refreshPlayers rebuilds the cache from names currently owned on the session bus.
func (s *MPRISService) refreshPlayers() error {
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("MPRIS service is not connected")
	}

	var names []string
	if err := conn.BusObject().Call(dbusIface+".ListNames", 0).Store(&names); err != nil {
		return fmt.Errorf("list D-Bus names: %w", err)
	}

	found := make(map[string]models.MPRISMediaPlayer)
	for _, name := range names {
		if !strings.HasPrefix(name, mprisPrefix) {
			continue
		}

		obj := conn.Object(name, mprisPath)
		var root, playback map[string]dbus.Variant
		if err := obj.Call(dbusProps+".GetAll", 0, mprisRoot).Store(&root); err != nil {
			continue // The player may have exited during discovery.
		}
		if err := obj.Call(dbusProps+".GetAll", 0, mprisPlayer).Store(&playback); err != nil {
			continue
		}

		var owner string
		_ = conn.BusObject().Call(dbusIface+".GetNameOwner", 0, name).Store(&owner)

		player := models.MPRISMediaPlayer{
			BusName: name, ObjectPath: string(mprisPath), Owner: owner,
		}

		if v, ok := root["Identity"]; ok {
			player.Identity, _ = v.Value().(string)
		}
		if v, ok := root["DesktopEntry"]; ok {
			player.DesktopEntry, _ = v.Value().(string)
		}
		if v, ok := root["CanQuit"]; ok {
			player.CanQuit, _ = v.Value().(bool)
		}
		if v, ok := root["CanRaise"]; ok {
			player.CanRaise, _ = v.Value().(bool)
		}

		if v, ok := playback["PlaybackStatus"]; ok {
			player.PlaybackStatus, _ = v.Value().(string)
		}
		if v, ok := playback["Position"]; ok {
			player.Position = mprisNumber(v.Value())
		}
		if v, ok := playback["Rate"]; ok {
			player.Rate, _ = v.Value().(float64)
		}
		if v, ok := playback["CanControl"]; ok {
			player.CanControl, _ = v.Value().(bool)
		}
		if v, ok := playback["CanPlay"]; ok {
			player.CanPlay, _ = v.Value().(bool)
		}
		if v, ok := playback["CanPause"]; ok {
			player.CanPause, _ = v.Value().(bool)
		}
		if v, ok := playback["CanGoNext"]; ok {
			player.CanGoNext, _ = v.Value().(bool)
		}
		if v, ok := playback["CanGoPrevious"]; ok {
			player.CanGoPrevious, _ = v.Value().(bool)
		}
		if v, ok := playback["CanSeek"]; ok {
			player.CanSeek, _ = v.Value().(bool)
		}
		if v, ok := playback["LoopStatus"]; ok {
			player.LoopStatus, _ = v.Value().(string)
		}
		if v, ok := playback["Shuffle"]; ok {
			player.Shuffle, _ = v.Value().(bool)
		}
		if v, ok := playback["Volume"]; ok {
			player.Volume, _ = v.Value().(float64)
		}

		if v, ok := playback["Metadata"]; ok {
			if metadata, ok := v.Value().(map[string]dbus.Variant); ok {
				if x, ok := metadata["mpris:trackid"]; ok {
					player.Metadata.TrackID, _ = x.Value().(string)
				}
				if x, ok := metadata["xesam:title"]; ok {
					player.Metadata.Title, _ = x.Value().(string)
				}
				if x, ok := metadata["xesam:artist"]; ok {
					player.Metadata.Artists, _ = x.Value().([]string)
				}
				if x, ok := metadata["xesam:album"]; ok {
					player.Metadata.Album, _ = x.Value().(string)
				}
				if x, ok := metadata["xesam:albumArtist"]; ok {
					player.Metadata.AlbumArtist, _ = x.Value().([]string)
				}
				if x, ok := metadata["mpris:artUrl"]; ok {
					player.Metadata.ArtworkURL, _ = x.Value().(string)
				}
				if x, ok := metadata["mpris:length"]; ok {
					player.Metadata.Duration = mprisNumber(x.Value())
				}
				if x, ok := metadata["xesam:trackNumber"]; ok {
					switch n := x.Value().(type) {
					case int32:
						player.Metadata.TrackNumber = n
					case int64:
						player.Metadata.TrackNumber = int32(n)
					}
				}
			}
		}

		found[name] = player
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.activePlayer
	s.players = found

	// Preserve an active player while it is still playing. Otherwise prefer
	// another controllable player that is playing, then the previous selection.
	if current, ok := found[s.activePlayer]; !ok || current.PlaybackStatus != "Playing" {
		s.activePlayer = ""
		if p, ok := found[previous]; ok && p.CanControl && p.PlaybackStatus == "Playing" {
			s.activePlayer = previous
		} else {
			names := make([]string, 0, len(found))
			for name := range found {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if found[name].CanControl && found[name].PlaybackStatus == "Playing" {
					s.activePlayer = name
					break
				}
			}
			if s.activePlayer == "" {
				if _, ok := found[previous]; ok {
					s.activePlayer = previous
				} else {
					for _, name := range names {
						if found[name].CanControl {
							s.activePlayer = name
							break
						}
					}
					if s.activePlayer == "" && len(names) > 0 {
						s.activePlayer = names[0]
					}
				}
			}
		}
	}

	for name, player := range s.players {
		player.IsActive = name == s.activePlayer
		s.players[name] = player
	}
	return nil
}

func mprisNumber(value any) int64 {
	switch n := value.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case uint64:
		return int64(n)
	case uint32:
		return int64(n)
	}
	return 0
}

// callPlayer is shared by playback commands to keep connection, timeout,
// and D-Bus error handling in one place.
func (s *MPRISService) callPlayer(busName, method string, args ...any) error {
	if !strings.HasPrefix(busName, mprisPrefix) {
		return fmt.Errorf("invalid MPRIS bus name %q", busName)
	}

	s.mu.RLock()
	conn := s.conn
	_, exists := s.players[busName]
	s.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("MPRIS service is not started")
	}
	if !exists {
		return fmt.Errorf("MPRIS player %q not found", busName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	if err := conn.Object(busName, mprisPath).CallWithContext(
		ctx, mprisPlayer+"."+method, 0, args...,
	).Err; err != nil {
		return fmt.Errorf("MPRIS %s on %s: %w", method, busName, err)
	}
	return nil
}

func (s *MPRISService) PlayPause(busName string) error {
	player, err := s.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl || (!player.CanPlay && !player.CanPause) {
		return fmt.Errorf("MPRIS player %q does not support play/pause", busName)
	}
	return s.callPlayer(busName, "PlayPause")
}

func (s *MPRISService) Next(busName string) error {
	player, err := s.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl || !player.CanGoNext {
		return fmt.Errorf("MPRIS player %q cannot skip forward", busName)
	}
	return s.callPlayer(busName, "Next")
}

func (s *MPRISService) Previous(busName string) error {
	player, err := s.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl || !player.CanGoPrevious {
		return fmt.Errorf("MPRIS player %q cannot skip backward", busName)
	}
	return s.callPlayer(busName, "Previous")
}

func (s *MPRISService) Seek(busName string, offset time.Duration) error {
	player, err := s.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl || !player.CanSeek {
		return fmt.Errorf("MPRIS player %q does not support seeking", busName)
	}
	if offset == 0 {
		return nil
	}
	return s.callPlayer(busName, "Seek", offset.Microseconds())
}

func (s *MPRISService) Play(busName string) error {
	player, err := s.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl || !player.CanPlay {
		return fmt.Errorf("MPRIS player %q cannot play", busName)
	}
	return s.callPlayer(busName, "Play")
}

func (s *MPRISService) Pause(busName string) error {
	player, err := s.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl || !player.CanPause {
		return fmt.Errorf("MPRIS player %q cannot pause", busName)
	}
	return s.callPlayer(busName, "Pause")
}

func (s *MPRISService) StopPlayback(busName string) error {
	player, err := s.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl {
		return fmt.Errorf("MPRIS player %q cannot be controlled", busName)
	}
	return s.callPlayer(busName, "Stop")
}

func (s *MPRISService) SetShuffle(busName string, enabled bool) error {
	return s.setPlayerProperty(busName, "Shuffle", dbus.MakeVariant(enabled))
}

func (s *MPRISService) SetLoopStatus(busName, status string) error {
	switch status {
	case "None", "Track", "Playlist":
	default:
		return fmt.Errorf("invalid loop status %q", status)
	}
	return s.setPlayerProperty(busName, "LoopStatus", dbus.MakeVariant(status))
}

// SetPlayerVolume controls the application's MPRIS volume, not system volume.
func (s *MPRISService) SetPlayerVolume(busName string, volume float64) error {
	if volume < 0 {
		return fmt.Errorf("volume cannot be negative")
	}
	return s.setPlayerProperty(busName, "Volume", dbus.MakeVariant(volume))
}

func (s *MPRISService) setPlayerProperty(
	busName, property string,
	value dbus.Variant,
) error {
	if !strings.HasPrefix(busName, mprisPrefix) {
		return fmt.Errorf("invalid MPRIS bus name %q", busName)
	}

	s.mu.RLock()
	conn := s.conn
	_, exists := s.players[busName]
	s.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("MPRIS service is not started")
	}
	if !exists {
		return fmt.Errorf("MPRIS player %q not found", busName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	if err := conn.Object(busName, mprisPath).CallWithContext(
		ctx, dbusProps+".Set", 0, mprisPlayer, property, value,
	).Err; err != nil {
		return fmt.Errorf("MPRIS set %s on %s: %w", property, busName, err)
	}
	return nil
}
