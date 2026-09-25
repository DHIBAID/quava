// Package config owns the daemon's durable identity and paired-device store.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

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

// Path returns the YAML configuration path.
func (s *Store) Path() string { return s.path }

// Dir returns the directory containing the YAML configuration file.
func (s *Store) Dir() string { return s.dir }

type file struct {
	Version  int                          `yaml:"version"`
	Identity *models.IdentityFile         `yaml:"identity,omitempty"`
	Devices  map[string]models.PairResult `yaml:"devices,omitempty"`
}

// Open loads the store at storageDir. An empty directory uses the OS config
// location, usually ~/.config/quava. Existing identity.json and trust.json
// files are migrated to config.yaml without deleting those legacy files.
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
		data: file{Version: currentVersion, Devices: map[string]models.PairResult{}},
	}
	data, err := os.ReadFile(store.path)
	if err == nil {
		if err := yaml.Unmarshal(data, &store.data); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", store.path, err)
		}
		if store.data.Version != currentVersion {
			return nil, fmt.Errorf("config: unsupported version %d", store.data.Version)
		}
		if store.data.Devices == nil {
			store.data.Devices = map[string]models.PairResult{}
		}
		return store, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("config: read %s: %w", store.path, err)
	}

	migrated, err := store.migrateLegacyJSON()
	if err != nil {
		return nil, err
	}
	if migrated {
		if err := store.Save(); err != nil {
			return nil, err
		}
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
func (s *Store) Save() error {
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
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("config: protect temporary file: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()
		return fmt.Errorf("config: write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("config: sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("config: close temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("config: replace %s: %w", s.path, err)
	}
	return nil
}

func (s *Store) migrateLegacyJSON() (bool, error) {
	migrated := false
	identityPath := filepath.Join(s.dir, "identity.json")
	if data, err := os.ReadFile(identityPath); err == nil {
		var identity models.IdentityFile
		if err := json.Unmarshal(data, &identity); err != nil {
			return false, fmt.Errorf("config: migrate %s: %w", identityPath, err)
		}
		s.SetIdentity(identity)
		migrated = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("config: read %s: %w", identityPath, err)
	}

	trustPath := filepath.Join(s.dir, "trust.json")
	if data, err := os.ReadFile(trustPath); err == nil {
		var trust models.TrustStore
		if err := json.Unmarshal(data, &trust); err != nil {
			return false, fmt.Errorf("config: migrate %s: %w", trustPath, err)
		}
		for _, device := range trust.Peers {
			if err := s.UpsertDevice(device); err != nil {
				return false, err
			}
		}
		migrated = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("config: read %s: %w", trustPath, err)
	}
	return migrated, nil
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

func ptrIdentity(identity models.IdentityFile) *models.IdentityFile { return &identity }

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
