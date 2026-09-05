package models

import (
	"crypto/ed25519"
	"time"
)

// InitiatorOptions controls the pairing initiator flow.
type InitiatorOptions struct {
	StorageDir   string
	DeviceName   string
	Capabilities []uint64
}

// PairResult is persisted after a successful pairing.
type PairResult struct {
	PeerDeviceID      string    `json:"peer_device_id"`
	PeerPublicKey     []byte    `json:"peer_public_key"`
	PeerDeviceName    string    `json:"peer_device_name"`
	ProtocolVersion   uint64    `json:"protocol_version"`
	Permissions       []uint64  `json:"permissions"`
	PeerCredential    []byte    `json:"peer_credential"`
	PairedAt          time.Time `json:"paired_at"`
	VerificationCode  uint32    `json:"verification_code"`
	SelectedVersion   uint64    `json:"selected_version"`
	TransactionID     []byte    `json:"transaction_id"`
	InitiatorDeviceID string    `json:"initiator_device_id"`
}

// IdentityFile is stored on disk for the long-term device identity.
type IdentityFile struct {
	DeviceName string             `json:"device_name"`
	PrivateKey ed25519.PrivateKey `json:"private_key"`
	PublicKey  ed25519.PublicKey  `json:"public_key"`
}

// TrustStore persists authenticated peers.
type TrustStore struct {
	Peers []PairResult `json:"peers"`
}

// ConfirmFunc is called after the verification code is displayed.
type ConfirmFunc func(code uint32, remoteName string) (bool, error)
