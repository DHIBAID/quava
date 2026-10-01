package models

import "crypto/ed25519"

// Identity contains the long-term Ed25519 identity key pair.
type Identity struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

// Fixed 16 byte sequence used to identify the protocol version and message type.
type TransactionID [16]byte

const (
	ProtocolName        = "quava-pairing"
	ProtocolVersionInfo = "quava-pairing-v1"
)
