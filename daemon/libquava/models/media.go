package models

type SystemAudioState struct {
	Volume                 float64             `json:"volume"`
	Muted                  bool                `json:"muted"`
	OutputDevice           string              `json:"output_device"`
	AvailableOutputDevices []AudioOutputDevice `json:"available_output_devices"`
}

type AudioOutputDevice struct {
	Name        string  `json:"name"`        // pactl sink name
	Description string  `json:"description"` // Human-readable name
	Volume      float64 `json:"volume"`      // Normalized 0.0–1.0
	Muted       bool    `json:"muted"`
	IsDefault   bool    `json:"is_default"`
}

// MPRISMediaPlayer represents a media player registered on the user's D-Bus session bus.
type MPRISMediaPlayer struct {
	BusName      string `json:"bus_name"`    // e.g. org.mpris.MediaPlayer2.spotify
	ObjectPath   string `json:"object_path"` // Usually /org/mpris/MediaPlayer2
	Identity     string `json:"identity"`    // e.g. Spotify
	DesktopEntry string `json:"desktop_entry,omitempty"`
	Owner        string `json:"owner,omitempty"` // Current unique D-Bus owner

	// Playback state
	PlaybackStatus string  `json:"playback_status"` // Playing, Paused, Stopped
	Position       int64   `json:"position"`        // Microseconds
	Rate           float64 `json:"rate,omitempty"`

	// Capabilities
	CanControl    bool `json:"can_control"`
	CanPlay       bool `json:"can_play"`
	CanPause      bool `json:"can_pause"`
	CanGoNext     bool `json:"can_go_next"`
	CanGoPrevious bool `json:"can_go_previous"`
	CanSeek       bool `json:"can_seek"`
	CanQuit       bool `json:"can_quit"`
	CanRaise      bool `json:"can_raise"`

	// Playback options
	LoopStatus string  `json:"loop_status,omitempty"` // None, Track, Playlist
	Shuffle    bool    `json:"shuffle"`
	Volume     float64 `json:"volume,omitempty"` // Player-specific volume, not system volume

	// Current track
	Metadata MediaMetadata `json:"metadata"`

	// Selection state managed by Quava
	IsActive bool `json:"is_active"`
}

type MediaMetadata struct {
	TrackID     string   `json:"track_id"`
	Title       string   `json:"title"`
	Artists     []string `json:"artists,omitempty"`
	Album       string   `json:"album,omitempty"`
	AlbumArtist []string `json:"album_artist,omitempty"`
	ArtworkURL  string   `json:"artwork_url,omitempty"`
	Duration    int64    `json:"duration,omitempty"` // MPRIS uses microseconds, nice.
	TrackNumber int32    `json:"track_number,omitempty"`
}
