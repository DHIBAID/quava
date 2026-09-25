package config

import (
	"bytes"
	"testing"
	"time"

	"libquava/models"
)

func TestStorePersistsAndLooksUpDevices(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	identity := models.IdentityFile{
		DeviceName: "desktop",
		PrivateKey: bytes.Repeat([]byte{1}, 64),
		PublicKey:  bytes.Repeat([]byte{2}, 32),
	}
	store.SetIdentity(identity)
	device := models.PairResult{
		PeerDeviceID:   "ABC123",
		PeerDeviceName: "Quava Android",
		PeerPublicKey:  []byte{1, 2, 3},
		PairedAt:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	if err := store.UpsertDevice(device); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotIdentity, ok := reopened.Identity()
	if !ok || gotIdentity.DeviceName != identity.DeviceName || !bytes.Equal(gotIdentity.PrivateKey, identity.PrivateKey) {
		t.Fatalf("identity was not persisted: %#v", gotIdentity)
	}
	gotDevice, ok := reopened.Device("abc123")
	if !ok || gotDevice.PeerDeviceName != device.PeerDeviceName || !bytes.Equal(gotDevice.PeerPublicKey, device.PeerPublicKey) {
		t.Fatalf("device ID lookup failed: %#v", gotDevice)
	}
	gotDevice, ok = reopened.Lookup("Quava Android")
	if !ok || gotDevice.PeerDeviceID != "abc123" {
		t.Fatalf("device name lookup failed: %#v", gotDevice)
	}
}
