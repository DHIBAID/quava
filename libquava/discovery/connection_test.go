package discovery

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"net"
	"strconv"
	"testing"
	"time"

	qcrypto "libquava/crypto"
	"libquava/pairing"
	"libquava/protocol"
)

func TestDeviceConnectAndPing(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		framed := protocol.NewConn(conn)
		message, err := framed.ReadMessage(context.Background())
		if err != nil {
			serverDone <- err
			return
		}
		if message.Type != protocol.MessageTypePing {
			serverDone <- protocol.ErrUnexpectedType
			return
		}
		if err := framed.WriteMessage(context.Background(), protocol.NewMessage(protocol.MessageTypePong, message.TransactionID, nil)); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	_, portString, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}

	device := Device{Name: "demo", Host: "127.0.0.1", Port: mustPort(t, portString)}
	conn, err := device.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server timeout")
	}
}

func TestInitiatePairing(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	responderIdentity, err := qcrypto.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("responder identity: %v", err)
	}
	responderPublicKey := responderIdentity.PublicKey
	responderDeviceID := qcrypto.DeviceIDBytes(responderPublicKey)
	responderNonce := make([]byte, 32)
	for i := range responderNonce {
		responderNonce[i] = byte(i + 1)
	}
	responderEphemeralPrivate, responderEphemeralPublic, err := qcrypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("responder ephemeral: %v", err)
	}

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		framed := protocol.NewConn(conn)
		request, err := framed.ReadMessage(context.Background())
		if err != nil {
			serverDone <- err
			return
		}
		if request.Type != protocol.MessageTypePairRequest {
			serverDone <- protocol.ErrUnexpectedType
			return
		}

		initiatorDeviceID, err := protocol.PayloadBytes(request.Payload, 0)
		if err != nil {
			serverDone <- err
			return
		}
		initiatorPublicKey, err := protocol.PayloadBytes(request.Payload, 1)
		if err != nil {
			serverDone <- err
			return
		}
		initiatorNonce, err := protocol.PayloadBytes(request.Payload, 2)
		if err != nil {
			serverDone <- err
			return
		}
		if !bytes.Equal(qcrypto.DeviceIDBytes(ed25519.PublicKey(initiatorPublicKey)), initiatorDeviceID) {
			serverDone <- protocol.ErrUnexpectedID
			return
		}

		challengeTransactionID := append([]byte(nil), request.TransactionID...)
		challengePayload := map[uint64]any{
			0: responderDeviceID,
			1: responderPublicKey,
			2: responderNonce,
			3: uint64(protocol.ProtocolVersion),
			4: responderEphemeralPublic,
			5: qcrypto.VerificationCode(ed25519.PublicKey(initiatorPublicKey), responderPublicKey, initiatorNonce, responderNonce, challengeTransactionID),
		}
		if err := framed.WriteMessage(context.Background(), protocol.NewMessage(protocol.MessageTypePairChallenge, challengeTransactionID, challengePayload)); err != nil {
			serverDone <- err
			return
		}

		authenticate, err := framed.ReadMessage(context.Background())
		if err != nil {
			serverDone <- err
			return
		}
		if authenticate.Type != protocol.MessageTypePairAuthenticate {
			serverDone <- protocol.ErrUnexpectedType
			return
		}
		initiatorEphemeralPublic, err := protocol.PayloadBytes(authenticate.Payload, 0)
		if err != nil {
			serverDone <- err
			return
		}
		initiatorSignature, err := protocol.PayloadBytes(authenticate.Payload, 1)
		if err != nil {
			serverDone <- err
			return
		}

		sharedSecret, err := qcrypto.X25519SharedSecret(responderEphemeralPrivate, initiatorEphemeralPublic)
		if err != nil {
			serverDone <- err
			return
		}
		masterSecret, err := qcrypto.MasterSecret(initiatorNonce, responderNonce, sharedSecret)
		if err != nil {
			serverDone <- err
			return
		}
		transcript := qcrypto.TranscriptBytes(uint64(protocol.ProtocolVersion), challengeTransactionID, initiatorDeviceID, responderDeviceID, initiatorPublicKey, responderPublicKey, initiatorNonce, responderNonce, initiatorEphemeralPublic, responderEphemeralPublic)
		if err := qcrypto.VerifyTranscript(ed25519.PublicKey(initiatorPublicKey), transcript, initiatorSignature); err != nil {
			serverDone <- err
			return
		}

		confirm, err := framed.ReadMessage(context.Background())
		if err != nil {
			serverDone <- err
			return
		}
		if confirm.Type != protocol.MessageTypePairConfirm {
			serverDone <- protocol.ErrUnexpectedType
			return
		}
		confirmed, err := protocol.PayloadBool(confirm.Payload, 1)
		if err != nil || !confirmed {
			serverDone <- err
			return
		}

		responderSignature := qcrypto.SignTranscript(responderIdentity.PrivateKey, transcript)
		completePayload := map[uint64]any{
			0: responderDeviceID,
			1: qcrypto.ConfirmationMAC(masterSecret, challengeTransactionID),
			2: responderSignature,
		}
		if err := framed.WriteMessage(context.Background(), protocol.NewMessage(protocol.MessageTypePairComplete, challengeTransactionID, completePayload)); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	_, portString, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}

	device := Device{Name: "demo", Host: "127.0.0.1", Port: mustPort(t, portString)}
	conn, err := device.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := pairing.Initiate(ctx, conn.ProtocolConn(), pairing.InitiatorOptions{DeviceName: "desktop"}, func(code uint32, remoteName string) (bool, error) {
		if code == 0 || remoteName == "" {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if result == nil || result.PeerDeviceID == "" {
		t.Fatal("pair returned empty result")
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server timeout")
	}
}

func mustPort(t *testing.T, portString string) int {
	t.Helper()
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return port
}
