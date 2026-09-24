package models

import "crypto/ed25519"

// Identity contains the long-term Ed25519 identity key pair.
type Identity struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

const (
	ProtocolName        = "quava-pairing"
	ProtocolVersionInfo = "quava-pairing-v1"
)
