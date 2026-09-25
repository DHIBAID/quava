package pairing

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"libquava/config"
	qcrypto "libquava/crypto"
	"libquava/models"
	"libquava/protocol"
)

// Initiate runs the initiator side of the pairing protocol.
func Initiate(ctx context.Context, conn *protocol.Conn, options models.InitiatorOptions, confirm models.ConfirmFunc) (*models.PairResult, error) {
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

	remoteName := options.RemoteDeviceName
	if strings.TrimSpace(remoteName) == "" {
		remoteName = hex.EncodeToString(responderDeviceID)
	}
	confirmCode, err := confirm(uint32(verificationCode), remoteName)
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
	result := &models.PairResult{
		PeerDeviceID:      hex.EncodeToString(responderDeviceID),
		PeerPublicKey:     append([]byte(nil), responderPublicKey...),
		PeerDeviceName:    remoteName,
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

func loadOrCreateIdentity(storageDir, deviceName string) (models.IdentityFile, string, error) {
	store, err := config.Open(storageDir)
	if err != nil {
		return models.IdentityFile{}, "", err
	}
	if file, ok := store.Identity(); ok {
		if len(file.PrivateKey) != ed25519.PrivateKeySize || len(file.PublicKey) != ed25519.PublicKeySize {
			return models.IdentityFile{}, "", errors.New("pairing: invalid identity file")
		}
		if strings.TrimSpace(file.DeviceName) == "" {
			file.DeviceName = fallbackDeviceName(deviceName)
			store.SetIdentity(file)
			if err := store.Save(); err != nil {
				return models.IdentityFile{}, "", err
			}
		}
		return file, store.Dir(), nil
	}

	identity, err := qcrypto.GenerateIdentityKeyPair()
	if err != nil {
		return models.IdentityFile{}, "", err
	}
	file := models.IdentityFile{
		DeviceName: fallbackDeviceName(deviceName),
		PrivateKey: identity.PrivateKey,
		PublicKey:  identity.PublicKey,
	}
	store.SetIdentity(file)
	if err := store.Save(); err != nil {
		return models.IdentityFile{}, "", err
	}
	return file, store.Dir(), nil
}

func persistTrust(storageDir string, result models.PairResult) error {
	store, err := config.Open(storageDir)
	if err != nil {
		return err
	}
	if err := store.UpsertDevice(result); err != nil {
		return err
	}
	return store.Save()
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
