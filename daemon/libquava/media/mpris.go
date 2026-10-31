package media

import (
	"context"
	"fmt"
	"libquava/models"
	"reflect"
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

	callTimeout            = 3 * time.Second
	positionUpdateInterval = time.Second
)

// MediaEventType identifies a change in the MPRIS player cache.
type MediaEventType int

const (
	MediaEventStateChanged MediaEventType = iota
	MediaEventActivePlayerChanged
	MediaEventPlayersChanged
	MediaEventPositionChanged
)

// MediaEvent is a snapshot of the affected player. State is nil when a player
// was removed or when there is no active player.
type MediaEvent struct {
	Type     MediaEventType
	PlayerID string
	State    *models.MPRISMediaPlayer
}

// MPRISService discovers session-bus media players and controls playback.
// We love dbus :)
type MPRISService struct {
	mu                 sync.RWMutex
	conn               *dbus.Conn
	players            map[string]models.MPRISMediaPlayer
	activePlayer       string
	subscribers        map[uint64]chan MediaEvent
	nextSubscriber     uint64
	ctx                context.Context
	cancel             context.CancelFunc
	positionCancel     context.CancelFunc
	positionPlayer     string
	positionGeneration uint64
	wg                 sync.WaitGroup
	signals            chan *dbus.Signal
	started            bool
	stopped            bool
}

func NewMPRISService() *MPRISService {
	return &MPRISService{
		players:     make(map[string]models.MPRISMediaPlayer),
		subscribers: make(map[uint64]chan MediaEvent),
		signals:     make(chan *dbus.Signal, 128),
	}
}

// Subscribe registers an independent, non-blocking MPRIS event consumer. The
// returned unsubscribe function is safe to call more than once. Events may be
// dropped for a slow subscriber rather than delaying D-Bus signal processing.
func (s *MPRISService) Subscribe() (<-chan MediaEvent, func()) {
	s.mu.Lock()
	if s.stopped {
		ch := make(chan MediaEvent)
		close(ch)
		s.mu.Unlock()
		return ch, func() {}
	}
	id := s.nextSubscriber
	s.nextSubscriber++
	ch := make(chan MediaEvent, 16)
	s.subscribers[id] = ch
	s.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			s.mu.Lock()
			if subscriber, ok := s.subscribers[id]; ok {
				delete(s.subscribers, id)
				close(subscriber)
			}
			s.mu.Unlock()
		})
	}
}

func (s *MPRISService) notify(events ...MediaEvent) {
	if len(events) == 0 {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, event := range events {
		for _, subscriber := range s.subscribers {
			delivered := event
			delivered.State = clonePlayer(event.State)
			select {
			case subscriber <- delivered:
			default:
			}
		}
	}
}

func clonePlayer(player *models.MPRISMediaPlayer) *models.MPRISMediaPlayer {
	if player == nil {
		return nil
	}
	copy := *player
	copy.Metadata.Artists = append([]string(nil), player.Metadata.Artists...)
	copy.Metadata.AlbumArtist = append([]string(nil), player.Metadata.AlbumArtist...)
	return &copy
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
	seekedMatch := []dbus.MatchOption{
		dbus.WithMatchInterface(mprisPlayer),
		dbus.WithMatchMember("Seeked"),
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
	if err := conn.AddMatchSignal(seekedMatch...); err != nil {
		_ = conn.RemoveMatchSignal(nameMatch...)
		_ = conn.RemoveMatchSignal(propsMatch...)
		cancel()
		_ = conn.Close()
		s.mu.Lock()
		s.conn, s.started = nil, false
		s.mu.Unlock()
		return fmt.Errorf("MPRIS: subscribe to seek changes: %w", err)
	}

	conn.Signal(s.signals)
	if err := s.refreshPlayers(); err != nil {
		conn.RemoveSignal(s.signals)
		_ = conn.RemoveMatchSignal(nameMatch...)
		_ = conn.RemoveMatchSignal(propsMatch...)
		_ = conn.RemoveMatchSignal(seekedMatch...)
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
	cancel, positionCancel, conn := s.cancel, s.positionCancel, s.conn
	s.positionCancel = nil
	s.positionPlayer = ""
	s.positionGeneration++
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if positionCancel != nil {
		positionCancel()
	}
	if conn != nil {
		conn.RemoveSignal(s.signals)
		_ = conn.RemoveMatchSignal(
			dbus.WithMatchInterface(dbusIface),
			dbus.WithMatchMember("NameOwnerChanged"),
		)
		_ = conn.RemoveMatchSignal(
			dbus.WithMatchInterface(mprisPlayer),
			dbus.WithMatchMember("Seeked"),
			dbus.WithMatchObjectPath(mprisPath),
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
	for id, subscriber := range s.subscribers {
		delete(s.subscribers, id)
		close(subscriber)
	}
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
	if _, ok := s.players[busName]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("MPRIS player %q not found", busName)
	}
	previous := s.activePlayer
	s.activePlayer = busName
	for name, player := range s.players {
		player.IsActive = name == busName
		s.players[name] = player
	}
	s.syncPositionTickerLocked()
	player := s.players[busName]
	s.mu.Unlock()
	if previous != busName {
		s.notify(MediaEvent{Type: MediaEventActivePlayerChanged, PlayerID: busName, State: &player})
	}
	return nil
}

func (s *MPRISService) monitor() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return

		case signal := <-s.signals:
			if signal == nil {
				continue
			}

			switch signal.Name {
			case dbusProps + ".PropertiesChanged":
				s.notify(s.handlePropertiesChanged(signal)...)
			case mprisPlayer + ".Seeked":
				s.notify(s.handleSeeked(signal)...)

			case dbusIface + ".NameOwnerChanged":
				if len(signal.Body) < 3 {
					continue
				}

				name, ok := signal.Body[0].(string)
				if !ok || !strings.HasPrefix(name, mprisPrefix) {
					continue
				}

				_ = s.refreshPlayers()
			}
		}
	}
}

func (s *MPRISService) handleSeeked(signal *dbus.Signal) []MediaEvent {
	if len(signal.Body) == 0 {
		return nil
	}
	position := mprisNumber(signal.Body[0])
	s.mu.Lock()
	defer s.mu.Unlock()
	for busName, player := range s.players {
		if player.Owner != signal.Sender {
			continue
		}
		if player.Position == position {
			return nil
		}
		player.Position = position
		s.players[busName] = player
		return []MediaEvent{{Type: MediaEventPositionChanged, PlayerID: busName, State: &player}}
	}
	return nil
}

func (s *MPRISService) fetchInvalidatedProperties(
	busName string,
	iface string,
	properties []string,
) (map[string]dbus.Variant, error) {
	s.mu.RLock()
	conn, serviceCtx := s.conn, s.ctx
	s.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("MPRIS: D-Bus connection is not available")
	}

	obj := conn.Object(busName, mprisPath)
	result := make(map[string]dbus.Variant, len(properties))

	for _, property := range properties {
		if !isTrackedProperty(iface, property) {
			continue
		}
		var value dbus.Variant
		ctx := serviceCtx
		if ctx == nil {
			ctx = context.Background()
		}
		requestCtx, cancel := context.WithTimeout(ctx, callTimeout)
		err := obj.CallWithContext(
			requestCtx,
			dbusProps+".Get",
			0,
			iface,
			property,
		).Store(&value)
		cancel()

		if err != nil {
			continue
		}

		result[property] = value
	}

	return result, nil
}

func isTrackedProperty(iface, property string) bool {
	switch iface {
	case mprisPlayer:
		switch property {
		case "PlaybackStatus", "Position", "Rate", "CanControl", "CanPlay", "CanPause",
			"CanGoNext", "CanGoPrevious", "CanSeek", "LoopStatus", "Shuffle", "Volume", "Metadata":
			return true
		}
	case mprisRoot:
		switch property {
		case "Identity", "DesktopEntry", "CanQuit", "CanRaise":
			return true
		}
	}
	return false
}

func (s *MPRISService) handlePropertiesChanged(signal *dbus.Signal) []MediaEvent {
	if len(signal.Body) < 3 {
		return nil
	}

	iface, ok := signal.Body[0].(string)
	if !ok {
		return nil
	}

	changed, ok := signal.Body[1].(map[string]dbus.Variant)
	if !ok {
		return nil
	}

	invalidated, _ := signal.Body[2].([]string)

	if iface != mprisPlayer && iface != mprisRoot {
		return nil
	}

	// D-Bus signals use the unique sender name (e.g. ":1.42"),
	// while our map is keyed by well-known names.
	var busName string
	s.mu.RLock()
	for name, player := range s.players {
		if player.Owner == signal.Sender {
			busName = name
			break
		}
	}
	s.mu.RUnlock()

	if busName == "" {
		// Player may have appeared before discovery completed.
		return nil
	}

	invalidatedValues, err := s.fetchInvalidatedProperties(busName, iface, invalidated)
	if err != nil {
		return nil
	}

	s.mu.Lock()
	player, ok := s.players[busName]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	before := player
	positionChanged := false
	for property, variant := range changed {
		positionChanged = applyMPRISUpdate(&player, iface, property, variant.Value()) || positionChanged
	}
	for property, variant := range invalidatedValues {
		positionChanged = applyMPRISUpdate(&player, iface, property, variant.Value()) || positionChanged
	}
	s.players[busName] = player
	activeChanged := s.updateActivePlayerLocked()
	s.syncPositionTickerLocked()
	activeName := s.activePlayer
	active, hasActive := s.players[activeName]
	s.mu.Unlock()

	events := make([]MediaEvent, 0, 3)
	if positionChanged && before.Position != player.Position {
		events = append(events, MediaEvent{Type: MediaEventPositionChanged, PlayerID: busName, State: &player})
	}
	stateBefore, stateAfter := before, player
	stateBefore.Position, stateAfter.Position = 0, 0
	if !reflect.DeepEqual(stateBefore, stateAfter) {
		events = append(events, MediaEvent{Type: MediaEventStateChanged, PlayerID: busName, State: &player})
	}
	if activeChanged {
		if hasActive {
			events = append(events, MediaEvent{Type: MediaEventActivePlayerChanged, PlayerID: activeName, State: &active})
		} else {
			events = append(events, MediaEvent{Type: MediaEventActivePlayerChanged})
		}
	}
	return events
}

func (s *MPRISService) updateActivePlayerLocked() bool {
	previous := s.activePlayer

	if current, ok := s.players[s.activePlayer]; !ok ||
		current.PlaybackStatus != "Playing" {

		s.activePlayer = ""

		if p, ok := s.players[previous]; ok &&
			p.CanControl && p.PlaybackStatus == "Playing" {
			s.activePlayer = previous
		} else {
			names := make([]string, 0, len(s.players))
			for name := range s.players {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				if s.players[name].CanControl &&
					s.players[name].PlaybackStatus == "Playing" {
					s.activePlayer = name
					break
				}
			}

			if s.activePlayer == "" {
				if _, ok := s.players[previous]; ok {
					s.activePlayer = previous
				} else {
					for _, name := range names {
						if s.players[name].CanControl {
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
	return previous != s.activePlayer
}

func applyMPRISUpdate(player *models.MPRISMediaPlayer, iface, property string, value any) bool {
	if iface == mprisRoot {
		switch property {
		case "Identity":
			if value, ok := value.(string); ok {
				player.Identity = value
			}
		case "DesktopEntry":
			if value, ok := value.(string); ok {
				player.DesktopEntry = value
			}
		case "CanQuit":
			if value, ok := value.(bool); ok {
				player.CanQuit = value
			}
		case "CanRaise":
			if value, ok := value.(bool); ok {
				player.CanRaise = value
			}
		}
		return false
	}
	switch property {
	case "PlaybackStatus":
		if value, ok := value.(string); ok {
			player.PlaybackStatus = value
		}
	case "Position":
		player.Position = mprisNumber(value)
		return true
	case "Rate":
		if value, ok := value.(float64); ok {
			player.Rate = value
		}
	case "CanControl":
		if value, ok := value.(bool); ok {
			player.CanControl = value
		}
	case "CanPlay":
		if value, ok := value.(bool); ok {
			player.CanPlay = value
		}
	case "CanPause":
		if value, ok := value.(bool); ok {
			player.CanPause = value
		}
	case "CanGoNext":
		if value, ok := value.(bool); ok {
			player.CanGoNext = value
		}
	case "CanGoPrevious":
		if value, ok := value.(bool); ok {
			player.CanGoPrevious = value
		}
	case "CanSeek":
		if value, ok := value.(bool); ok {
			player.CanSeek = value
		}
	case "LoopStatus":
		if value, ok := value.(string); ok {
			player.LoopStatus = value
		}
	case "Shuffle":
		if value, ok := value.(bool); ok {
			player.Shuffle = value
		}
	case "Volume":
		if value, ok := value.(float64); ok {
			player.Volume = value
		}
	case "Metadata":
		updateMPRISMetadata(player, value)
	}
	return false
}

// syncPositionTickerLocked keeps exactly one ticker for the current active,
// playing player. s.mu must be held by the caller.
func (s *MPRISService) syncPositionTickerLocked() {
	player, ok := s.players[s.activePlayer]
	shouldRun := ok && player.PlaybackStatus == "Playing" && s.ctx != nil && !s.stopped
	if shouldRun && s.positionCancel != nil && s.positionPlayer == s.activePlayer {
		return
	}
	if s.positionCancel != nil {
		s.positionCancel()
		s.positionCancel = nil
		s.positionPlayer = ""
		s.positionGeneration++
	}
	if !shouldRun {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.positionCancel = cancel
	s.positionPlayer = s.activePlayer
	s.positionGeneration++
	generation := s.positionGeneration
	s.wg.Add(1)
	go s.monitorPosition(ctx, s.activePlayer, generation)
}

func (s *MPRISService) monitorPosition(ctx context.Context, busName string, generation uint64) {
	defer s.wg.Done()
	ticker := time.NewTicker(positionUpdateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshPosition(ctx, busName, generation)
		}
	}
}

func (s *MPRISService) refreshPosition(ctx context.Context, busName string, generation uint64) {
	s.mu.RLock()
	conn := s.conn
	player, ok := s.players[busName]
	active := s.activePlayer == busName && s.positionPlayer == busName && s.positionGeneration == generation
	s.mu.RUnlock()
	if conn == nil || !ok || !active || player.PlaybackStatus != "Playing" {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var value dbus.Variant
	if err := conn.Object(busName, mprisPath).CallWithContext(requestCtx, dbusProps+".Get", 0, mprisPlayer, "Position").Store(&value); err != nil {
		return
	}
	position := mprisNumber(value.Value())
	s.mu.Lock()
	player, ok = s.players[busName]
	if !ok || s.activePlayer != busName || s.positionPlayer != busName || s.positionGeneration != generation || player.PlaybackStatus != "Playing" || player.Position == position {
		s.mu.Unlock()
		return
	}
	player.Position = position
	s.players[busName] = player
	s.mu.Unlock()
	s.notify(MediaEvent{Type: MediaEventPositionChanged, PlayerID: busName, State: &player})
}

func updateMPRISMetadata(
	player *models.MPRISMediaPlayer,
	value any,
) {
	metadata, ok := value.(map[string]dbus.Variant)
	if !ok {
		return
	}
	// Metadata is a snapshot. Reset it so absent keys do not retain stale data.
	player.Metadata = models.MediaMetadata{}

	if v, ok := metadata["mpris:trackid"]; ok {
		player.Metadata.TrackID = mprisTrackID(v.Value())
	}

	if v, ok := metadata["xesam:title"]; ok {
		player.Metadata.Title, _ = v.Value().(string)
	}

	if v, ok := metadata["xesam:artist"]; ok {
		player.Metadata.Artists, _ = v.Value().([]string)
	}

	if v, ok := metadata["xesam:album"]; ok {
		player.Metadata.Album, _ = v.Value().(string)
	}

	if v, ok := metadata["xesam:albumArtist"]; ok {
		player.Metadata.AlbumArtist, _ = v.Value().([]string)
	}

	if v, ok := metadata["mpris:artUrl"]; ok {
		player.Metadata.ArtworkURL, _ = v.Value().(string)
	}

	if v, ok := metadata["mpris:length"]; ok {
		player.Metadata.Duration = mprisNumber(v.Value())
	}

	if v, ok := metadata["xesam:trackNumber"]; ok {
		switch n := v.Value().(type) {
		case int32:
			player.Metadata.TrackNumber = n
		case int64:
			player.Metadata.TrackNumber = int32(n)
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
					player.Metadata.TrackID = mprisTrackID(x.Value())
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
	previous := s.players
	previousActive := s.activePlayer
	s.players = found
	activeChanged := s.updateActivePlayerLocked()
	s.syncPositionTickerLocked()
	activeName := s.activePlayer
	active, hasActive := s.players[activeName]
	events := make([]MediaEvent, 0, len(previous)+len(found)+1)
	for name, oldPlayer := range previous {
		newPlayer, exists := found[name]
		if !exists {
			events = append(events, MediaEvent{Type: MediaEventPlayersChanged, PlayerID: name})
			continue
		}
		if !reflect.DeepEqual(oldPlayer, newPlayer) {
			player := newPlayer
			events = append(events, MediaEvent{Type: MediaEventPlayersChanged, PlayerID: name, State: &player})
		}
	}
	for name, player := range found {
		if _, exists := previous[name]; !exists {
			copy := player
			events = append(events, MediaEvent{Type: MediaEventPlayersChanged, PlayerID: name, State: &copy})
		}
	}
	if activeChanged || previousActive != activeName {
		if hasActive {
			events = append(events, MediaEvent{Type: MediaEventActivePlayerChanged, PlayerID: activeName, State: &active})
		} else {
			events = append(events, MediaEvent{Type: MediaEventActivePlayerChanged})
		}
	}
	s.mu.Unlock()
	s.notify(events...)

	return nil
}

// mprisTrackID extracts mpris:trackid. The spec types it as a D-Bus object
// path, which godbus decodes as dbus.ObjectPath rather than string; a plain
// string assertion silently yields "" and makes SetPosition impossible.
func mprisTrackID(value any) string {
	switch id := value.(type) {
	case dbus.ObjectPath:
		return string(id)
	case string:
		return id
	}
	return ""
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

// SeekTo moves playback to an absolute position. Absolute targets avoid the
// race of computing a relative offset from a stale cached position.
func (s *MPRISService) SeekTo(busName string, position time.Duration) error {
	player, err := s.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl || !player.CanSeek {
		return fmt.Errorf("MPRIS player %q does not support seeking", busName)
	}
	target := position.Microseconds()
	if target < 0 {
		target = 0
	}
	if length := player.Metadata.Duration; length > 0 && target > length {
		target = length
	}

	trackID := player.Metadata.TrackID
	if trackID != "" && dbus.ObjectPath(trackID).IsValid() &&
		trackID != "/org/mpris/MediaPlayer2/TrackList/NoTrack" {
		return s.callPlayer(busName, "SetPosition", dbus.ObjectPath(trackID), target)
	}

	// No usable track ID: fall back to a relative seek from the live position.
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("MPRIS service is not started")
	}
	var value dbus.Variant
	if err := conn.Object(busName, mprisPath).CallWithContext(
		ctx, dbusProps+".Get", 0, mprisPlayer, "Position",
	).Store(&value); err != nil {
		return fmt.Errorf("MPRIS get Position on %s: %w", busName, err)
	}
	delta := target - mprisNumber(value.Value())
	if delta == 0 {
		return nil
	}
	return s.callPlayer(busName, "Seek", delta)
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
