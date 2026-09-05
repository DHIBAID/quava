package pairing

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	qcrypto "libquava/crypto"
	"libquava/protocol"
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

// Initiate runs the initiator side of the pairing protocol.
func Initiate(ctx context.Context, conn *protocol.Conn, options InitiatorOptions, confirm ConfirmFunc) (*PairResult, error) {
	if conn == nil {
		return nil, errors.New("pairing: nil connection")
	}
	if confirm == nil {
		return nil, errors.New("pairing: nil confirm callback")
	}

	identity, _, err := loadOrCreateIdentity(options.StorageDir, options.DeviceName)
	if err != nil {
		return nil, err
	}

	transactionID, err := qcrypto.GenerateTransactionID()
	if err != nil {
		return nil, err
	}
	initiatorNonce, err := qcrypto.GenerateNonce()
	if err != nil {
		return nil, err
	}
	initiatorEphemeralPrivate, initiatorEphemeralPublic, err := qcrypto.GenerateX25519KeyPair()
	if err != nil {
		return nil, err
	}

	selectedVersion := uint64(protocol.ProtocolVersion)
	initiatorDeviceID := qcrypto.DeviceIDBytes(identity.PublicKey)
	requestPayload := map[uint64]any{
		0: initiatorDeviceID,
		1: append([]byte(nil), identity.PublicKey...),
		2: initiatorNonce,
		3: []uint64{protocol.ProtocolVersion},
		4: identity.DeviceName,
		5: append([]uint64(nil), options.Capabilities...),
	}
	if err := conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePairRequest, transactionID, requestPayload)); err != nil {
		return nil, err
	}

	challengeMessage, err := conn.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateMessage(challengeMessage, protocol.MessageTypePairChallenge, transactionID); err != nil {
		return nil, err
	}

	responderDeviceID, err := protocol.PayloadBytes(challengeMessage.Payload, 0)
	if err != nil {
		return nil, err
	}
	responderPublicKey, err := protocol.PayloadBytes(challengeMessage.Payload, 1)
	if err != nil {
		return nil, err
	}
	responderNonce, err := protocol.PayloadBytes(challengeMessage.Payload, 2)
	if err != nil {
		return nil, err
	}
	selectedVersion, err = protocol.PayloadUint(challengeMessage.Payload, 3)
	if err != nil {
		return nil, err
	}
	if selectedVersion != protocol.ProtocolVersion {
		return nil, fmt.Errorf("pairing: unsupported version %d", selectedVersion)
	}
	responderEphemeralPublic, err := protocol.PayloadBytes(challengeMessage.Payload, 4)
	if err != nil {
		return nil, err
	}
	verificationCode, err := protocol.PayloadUint(challengeMessage.Payload, 5)
	if err != nil {
		return nil, err
	}
	computedVerificationCode := qcrypto.VerificationCode(identity.PublicKey, responderPublicKey, initiatorNonce, responderNonce, transactionID)
	if uint64(computedVerificationCode) != verificationCode {
		return nil, fmt.Errorf("pairing: verification code mismatch")
	}

	confirmCode, err := confirm(uint32(verificationCode), hex.EncodeToString(responderDeviceID))
	if err != nil {
		return nil, err
	}
	if !confirmCode {
		cancelPayload := map[uint64]any{0: protocol.ReasonUserRejected}
		_ = conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePairCancel, transactionID, cancelPayload))
		return nil, errors.New("pairing: user rejected pairing")
	}

	sharedSecret, err := qcrypto.X25519SharedSecret(initiatorEphemeralPrivate, responderEphemeralPublic)
	if err != nil {
		return nil, err
	}
	masterSecret, err := qcrypto.MasterSecret(initiatorNonce, responderNonce, sharedSecret)
	if err != nil {
		return nil, err
	}

	transcript := qcrypto.TranscriptBytes(selectedVersion, transactionID, initiatorDeviceID, responderDeviceID, identity.PublicKey, responderPublicKey, initiatorNonce, responderNonce, initiatorEphemeralPublic, responderEphemeralPublic)
	identitySignature := qcrypto.SignTranscript(identity.PrivateKey, transcript)

	authenticatePayload := map[uint64]any{
		0: append([]byte(nil), initiatorEphemeralPublic...),
		1: identitySignature,
	}
	if err := conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePairAuthenticate, transactionID, authenticatePayload)); err != nil {
		return nil, err
	}

	confirmPayload := map[uint64]any{1: true}
	if err := conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePairConfirm, transactionID, confirmPayload)); err != nil {
		return nil, err
	}

	confirmationMessage, err := conn.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateMessage(confirmationMessage, protocol.MessageTypePairComplete, transactionID); err != nil {
		return nil, err
	}
	completeDeviceID, err := protocol.PayloadBytes(confirmationMessage.Payload, 0)
	if err != nil {
		return nil, err
	}
	confirmationMAC, err := protocol.PayloadBytes(confirmationMessage.Payload, 1)
	if err != nil {
		return nil, err
	}
	responderSignature, err := protocol.PayloadBytes(confirmationMessage.Payload, 2)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(completeDeviceID, responderDeviceID) {
		return nil, errors.New("pairing: responder device id mismatch")
	}
	if !hmac.Equal(confirmationMAC, qcrypto.ConfirmationMAC(masterSecret, transactionID)) {
		return nil, errors.New("pairing: confirmation mac mismatch")
	}
	if err := qcrypto.VerifyTranscript(ed25519.PublicKey(responderPublicKey), transcript, responderSignature); err != nil {
		return nil, err
	}

	peerCredential, err := qcrypto.PeerCredential(transactionID, masterSecret)
	if err != nil {
		return nil, err
	}
	result := &PairResult{
		PeerDeviceID:      hex.EncodeToString(responderDeviceID),
		PeerPublicKey:     append([]byte(nil), responderPublicKey...),
		PeerDeviceName:    hex.EncodeToString(responderDeviceID),
		ProtocolVersion:   selectedVersion,
		Permissions:       append([]uint64(nil), options.Capabilities...),
		PeerCredential:    peerCredential,
		PairedAt:          time.Now().UTC(),
		VerificationCode:  uint32(verificationCode),
		SelectedVersion:   selectedVersion,
		TransactionID:     append([]byte(nil), transactionID...),
		InitiatorDeviceID: hex.EncodeToString(initiatorDeviceID),
	}
	if err := persistTrust(options.StorageDir, *result); err != nil {
		return nil, err
	}
	return result, nil
}

func loadOrCreateIdentity(storageDir, deviceName string) (IdentityFile, string, error) {
	if strings.TrimSpace(storageDir) == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return IdentityFile{}, "", err
		}
		storageDir = filepath.Join(configDir, "quava")
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return IdentityFile{}, "", err
	}

	identityPath := filepath.Join(storageDir, "identity.json")
	data, err := os.ReadFile(identityPath)
	if err == nil {
		var file IdentityFile
		if err := json.Unmarshal(data, &file); err != nil {
			return IdentityFile{}, "", err
		}
		if len(file.PrivateKey) != ed25519.PrivateKeySize || len(file.PublicKey) != ed25519.PublicKeySize {
			return IdentityFile{}, "", errors.New("pairing: invalid identity file")
		}
		if strings.TrimSpace(file.DeviceName) == "" {
			file.DeviceName = fallbackDeviceName(deviceName)
		}
		return file, storageDir, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return IdentityFile{}, "", err
	}

	identity, err := qcrypto.GenerateIdentityKeyPair()
	if err != nil {
		return IdentityFile{}, "", err
	}
	file := IdentityFile{
		DeviceName: fallbackDeviceName(deviceName),
		PrivateKey: identity.PrivateKey,
		PublicKey:  identity.PublicKey,
	}
	if err := saveJSON(identityPath, file); err != nil {
		return IdentityFile{}, "", err
	}
	return file, storageDir, nil
}

func persistTrust(storageDir string, result PairResult) error {
	if strings.TrimSpace(storageDir) == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		storageDir = filepath.Join(configDir, "quava")
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return err
	}

	trustPath := filepath.Join(storageDir, "trust.json")
	store := TrustStore{}
	data, err := os.ReadFile(trustPath)
	if err == nil {
		if err := json.Unmarshal(data, &store); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	store.Peers = append(store.Peers, result)
	sort.Slice(store.Peers, func(i, j int) bool {
		if store.Peers[i].PeerDeviceID == store.Peers[j].PeerDeviceID {
			return store.Peers[i].PairedAt.Before(store.Peers[j].PairedAt)
		}
		return store.Peers[i].PeerDeviceID < store.Peers[j].PeerDeviceID
	})
	return saveJSON(trustPath, store)
}

func saveJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o600)
}

func fallbackDeviceName(deviceName string) string {
	if strings.TrimSpace(deviceName) != "" {
		return strings.TrimSpace(deviceName)
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "Quava"
	}
	return hostname
}

func validateMessage(message protocol.Message, expectedType uint64, expectedTransactionID []byte) error {
	if message.Version != protocol.ProtocolVersion {
		return fmt.Errorf("pairing: unexpected version %d", message.Version)
	}
	if message.Type != expectedType {
		return fmt.Errorf("%w: got %d want %d", protocol.ErrUnexpectedType, message.Type, expectedType)
	}
	if !hmac.Equal(message.TransactionID, expectedTransactionID) {
		return fmt.Errorf("%w", protocol.ErrUnexpectedID)
	}
	return nil
}
