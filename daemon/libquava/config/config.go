// Package config owns the daemon's durable identity and paired-device store.
package config

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"libquava/crypto"
	"libquava/models"
)

const (
	fileName       = "config.yaml"
	currentVersion = 1
)

// Store is an opened configuration file. Call Save after making changes.
type Store struct {
	dir  string
	path string
	data file
}

type file struct {
	Version          int                                  `yaml:"version"`
	Identity         *models.IdentityFile                 `yaml:"identity,omitempty"`
	Devices          map[string]models.PairResult         `yaml:"devices,omitempty"`
	NotificationSync map[string]NotificationSyncDirection `yaml:"notification_sync,omitempty"`
	ClipboardSync    map[string]bool                      `yaml:"clipboard_sync,omitempty"`
}

// NotificationSyncDirection controls notification flow for one paired device.
// An absent setting is treated as ToHost so Android notifications are rendered
// on Linux without forwarding host notifications to Android.
type NotificationSyncDirection string

const (
	NotificationSyncBoth     NotificationSyncDirection = "both"
	NotificationSyncNone     NotificationSyncDirection = "none"
	NotificationSyncToClient NotificationSyncDirection = "toClient"
	NotificationSyncToHost   NotificationSyncDirection = "toHost"
)

// Path returns the YAML configuration path.
func (s *Store) Path() string { return s.path }

// Dir returns the directory containing the YAML configuration file.
func (s *Store) Dir() string { return s.dir }

// Open loads the store at storageDir. An empty directory uses the OS config
// location, usually ~/.config/quava.
func Open(storageDir string) (*Store, error) {
	dir, err := resolveDir(storageDir)

	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("config: create %s: %w", dir, err)
	}

	store := &Store{
		dir:  dir,
		path: filepath.Join(dir, fileName),
		data: file{Version: currentVersion, Devices: map[string]models.PairResult{}, NotificationSync: map[string]NotificationSyncDirection{}, ClipboardSync: map[string]bool{}},
	}

	data, err := os.ReadFile(store.path)
	if err == nil {
		if err := yaml.Unmarshal(data, &store.data); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", store.path, err)
		}

		if err := migrateConfig(&store.data); err != nil {
			return nil, err
		}
		if err := validateConfig(store.data); err != nil {
			return nil, fmt.Errorf("config: invalid data in %s: %w", store.path, err)
		}

		if store.data.Devices == nil {
			store.data.Devices = map[string]models.PairResult{}
		}
		if store.data.NotificationSync == nil {
			store.data.NotificationSync = map[string]NotificationSyncDirection{}
		}
		if store.data.ClipboardSync == nil {
			store.data.ClipboardSync = map[string]bool{}
		}

		return store, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("config: read %s: %w", store.path, err)
	}

	return store, nil
}

// Identity returns a copy of the daemon's long-term identity, if one exists.
func (s *Store) Identity() (models.IdentityFile, bool) {
	if s == nil || s.data.Identity == nil {
		return models.IdentityFile{}, false
	}

	return cloneIdentity(*s.data.Identity), true
}

// SetIdentity replaces the daemon's long-term identity.
func (s *Store) SetIdentity(identity models.IdentityFile) {
	s.data.Identity = ptrIdentity(cloneIdentity(identity))
}

// Devices returns all paired devices ordered by device ID.
func (s *Store) Devices() []models.PairResult {
	if s == nil {
		return nil
	}

	devices := make([]models.PairResult, 0, len(s.data.Devices))

	for _, device := range s.data.Devices {
		devices = append(devices, cloneDevice(device))
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].PeerDeviceID < devices[j].PeerDeviceID
	})

	return devices
}

// Device looks up a paired device by its canonical device ID.
func (s *Store) Device(deviceID string) (models.PairResult, bool) {
	if s == nil {
		return models.PairResult{}, false
	}

	device, ok := s.data.Devices[strings.ToLower(strings.TrimSpace(deviceID))]

	return cloneDevice(device), ok
}

// Lookup finds a device by exact device ID or its stored display name.
func (s *Store) Lookup(query string) (models.PairResult, bool) {
	if device, ok := s.Device(query); ok {
		return device, true
	}

	query = strings.TrimSpace(query)

	for _, device := range s.Devices() {
		if device.PeerDeviceName == query {
			return device, true
		}
	}

	return models.PairResult{}, false
}

// NotificationSync returns the notification direction for deviceID. Paired
// devices without an explicit setting receive Android-to-Linux sync only.
func (s *Store) NotificationSync(deviceID string) NotificationSyncDirection {
	if s == nil {
		return NotificationSyncToHost
	}

	direction := s.data.NotificationSync[strings.ToLower(strings.TrimSpace(deviceID))]
	if validNotificationSyncDirection(direction) {
		return direction
	}
	return NotificationSyncToHost
}

// SetNotificationSync persists a notification direction for one paired device.
func (s *Store) SetNotificationSync(deviceID string, direction NotificationSyncDirection) error {
	deviceID = strings.ToLower(strings.TrimSpace(deviceID))
	if deviceID == "" {
		return errors.New("config: paired device has no device ID")
	}
	if !validNotificationSyncDirection(direction) {
		return fmt.Errorf("config: invalid notification sync direction %q", direction)
	}
	if s.data.NotificationSync == nil {
		s.data.NotificationSync = map[string]NotificationSyncDirection{}
	}
	s.data.NotificationSync[deviceID] = direction
	return nil
}

func validNotificationSyncDirection(direction NotificationSyncDirection) bool {
	switch direction {
	case NotificationSyncBoth, NotificationSyncNone, NotificationSyncToClient, NotificationSyncToHost:
		return true
	default:
		return false
	}
}

// ClipboardSync reports whether automatic clipboard broadcast is enabled for a peer.
// It defaults to false so a newly paired device does not receive clipboard data
// until the user explicitly opts in. Explicit CLI clipboard-push remains targeted.
func (s *Store) ClipboardSync(deviceID string) bool {
	if s == nil {
		return false
	}
	return s.data.ClipboardSync[strings.ToLower(strings.TrimSpace(deviceID))]
}

// SetClipboardSync enables or disables automatic host clipboard sync for a peer.
func (s *Store) SetClipboardSync(deviceID string, enabled bool) error {
	deviceID = strings.ToLower(strings.TrimSpace(deviceID))
	if deviceID == "" {
		return errors.New("config: paired device has no device ID")
	}
	if _, ok := s.data.Devices[deviceID]; !ok {
		return fmt.Errorf("config: device %q is not paired", deviceID)
	}
	if s.data.ClipboardSync == nil {
		s.data.ClipboardSync = map[string]bool{}
	}
	s.data.ClipboardSync[deviceID] = enabled
	return nil
}

// UpsertDevice adds or updates a trusted device by device ID.
func (s *Store) UpsertDevice(device models.PairResult) error {
	device.PeerDeviceID = strings.ToLower(strings.TrimSpace(device.PeerDeviceID))

	if device.PeerDeviceID == "" {
		return errors.New("config: paired device has no device ID")
	}

	if s.data.Devices == nil {
		s.data.Devices = map[string]models.PairResult{}
	}

	s.data.Devices[device.PeerDeviceID] = cloneDevice(device)

	return nil
}

// Save atomically writes config.yaml with owner-only permissions.
func (s *Store) Save() (resultErr error) {
	if s == nil {
		return errors.New("config: nil store")
	}

	s.data.Version = currentVersion
	encoded, err := yaml.Marshal(s.data)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}

	temporary, err := os.CreateTemp(s.dir, ".config.yaml-*")
	if err != nil {
		return fmt.Errorf("config: create temporary file: %w", err)
	}

	temporaryPath := temporary.Name()

	defer func() {
		if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) && resultErr == nil {
			resultErr = fmt.Errorf("config: remove temporary file: %w", err)
		}
	}()

	closeTemporary := func() error {
		if err := temporary.Close(); err != nil {
			return fmt.Errorf("config: close temporary file: %w", err)
		}

		return nil
	}

	if err := temporary.Chmod(0o600); err != nil {
		return errors.Join(fmt.Errorf("config: protect temporary file: %w", err), closeTemporary())
	}

	if _, err := temporary.Write(encoded); err != nil {
		return errors.Join(fmt.Errorf("config: write temporary file: %w", err), closeTemporary())
	}

	if err := temporary.Sync(); err != nil {
		return errors.Join(fmt.Errorf("config: sync temporary file: %w", err), closeTemporary())
	}

	if err := closeTemporary(); err != nil {
		return err
	}

	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("config: replace %s: %w", s.path, err)
	}

	return nil
}

// migrateConfig applies explicit, forward-only configuration migrations.
// Version zero represents the original unversioned/empty configuration shape.
func migrateConfig(data *file) error {
	if data == nil {
		return errors.New("config: nil configuration")
	}
	switch data.Version {
	case 0:
		data.Version = 1
	case currentVersion:
		// Current schema; no migration required.
	default:
		return fmt.Errorf("config: unsupported version %d", data.Version)
	}
	return nil
}

func validateConfig(data file) error {
	if data.Identity != nil {
		identity := data.Identity
		if len(identity.PrivateKey) != ed25519.PrivateKeySize || len(identity.PublicKey) != ed25519.PublicKeySize {
			return errors.New("identity has invalid Ed25519 key lengths")
		}
		derived, ok := identity.PrivateKey.Public().(ed25519.PublicKey)
		if !ok || !bytes.Equal(derived, identity.PublicKey) {
			return errors.New("identity public/private keys do not match")
		}
	}
	for id, device := range data.Devices {
		canonical := strings.ToLower(strings.TrimSpace(id))
		if canonical == "" || canonical != id || device.PeerDeviceID != id {
			return fmt.Errorf("device map key %q is not canonical", id)
		}
		if len(device.PeerPublicKey) != ed25519.PublicKeySize || len(device.PeerCredential) == 0 {
			return fmt.Errorf("device %q has incomplete trust material", id)
		}
		if crypto.DeviceIDFromPublicKey(ed25519.PublicKey(device.PeerPublicKey)) != id {
			return fmt.Errorf("device %q ID does not match its public key", id)
		}
	}
	for id, direction := range data.NotificationSync {
		if strings.ToLower(strings.TrimSpace(id)) != id || !validNotificationSyncDirection(direction) {
			return fmt.Errorf("invalid notification sync setting for device %q", id)
		}
	}
	for id := range data.ClipboardSync {
		if strings.ToLower(strings.TrimSpace(id)) != id {
			return fmt.Errorf("invalid clipboard sync setting for device %q", id)
		}
		if _, ok := data.Devices[id]; !ok {
			return fmt.Errorf("clipboard sync setting references unknown device %q", id)
		}
	}
	return nil
}

func resolveDir(storageDir string) (string, error) {
	if strings.TrimSpace(storageDir) != "" {
		return storageDir, nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve config directory: %w", err)
	}

	return filepath.Join(base, "quava"), nil
}

// surely there's a better way to do this
func ptrIdentity(identity models.IdentityFile) *models.IdentityFile {
	return &identity
}

func cloneIdentity(identity models.IdentityFile) models.IdentityFile {
	identity.PrivateKey = append(identity.PrivateKey[:0:0], identity.PrivateKey...)
	identity.PublicKey = append(identity.PublicKey[:0:0], identity.PublicKey...)
	return identity
}

func cloneDevice(device models.PairResult) models.PairResult {
	device.PeerPublicKey = append(device.PeerPublicKey[:0:0], device.PeerPublicKey...)
	device.Permissions = append(device.Permissions[:0:0], device.Permissions...)
	device.PeerCredential = append(device.PeerCredential[:0:0], device.PeerCredential...)
	device.TransactionID = append(device.TransactionID[:0:0], device.TransactionID...)
	return device
}
