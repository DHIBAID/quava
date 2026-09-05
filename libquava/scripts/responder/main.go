package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	qcrypto "libquava/crypto"
	"libquava/models"
	"libquava/protocol"
)

func main() {
    addr := flag.String("addr", ":48273", "listen address")
    flag.Parse()

    ln, err := net.Listen("tcp4", *addr)
    if err != nil {
        log.Fatalf("listen: %v", err)
    }
    defer ln.Close()
    log.Printf("responder listening %s", *addr)

    identity, err := qcrypto.GenerateIdentityKeyPair()
    if err != nil {
        log.Fatalf("generate identity: %v", err)
    }

    for {
        conn, err := ln.Accept()
        if err != nil {
            log.Printf("accept: %v", err)
            continue
        }
        go handleConn(context.Background(), conn, identity)
    }
}

func handleConn(ctx context.Context, raw net.Conn, identity models.Identity) {
    defer raw.Close()
    c := protocol.NewConn(raw)

    // Read first message
    msg, err := c.ReadMessage(ctx)
    if err != nil {
        log.Printf("read message: %v", err)
        return
    }

    switch msg.Type {
    case protocol.MessageTypePing:
        // reply with pong
        if err := c.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePong, msg.TransactionID, nil)); err != nil {
            log.Printf("write pong: %v", err)
        }
        return
    case protocol.MessageTypePairRequest:
        if err := handlePairRequest(ctx, c, msg, identity); err != nil {
            log.Printf("pair flow error: %v", err)
        }
        return
    default:
        log.Printf("unexpected message type %d", msg.Type)
        return
    }
}

func handlePairRequest(ctx context.Context, c *protocol.Conn, req protocol.Message, identity models.Identity) error {
    // extract initiator fields
    initiatorDeviceID, err := protocol.PayloadBytes(req.Payload, 0)
    if err != nil {
        return err
    }
    initiatorPublicKey, err := protocol.PayloadBytes(req.Payload, 1)
    if err != nil {
        return err
    }
    initiatorNonce, err := protocol.PayloadBytes(req.Payload, 2)
    if err != nil {
        return err
    }

    // create responder nonce and ephemeral
    responderNonce, err := qcrypto.GenerateNonce()
    if err != nil {
        return err
    }
    respEphemeralPriv, respEphemeralPub, err := qcrypto.GenerateX25519KeyPair()
    if err != nil {
        return err
    }

    responderDeviceID := qcrypto.DeviceIDBytes(identity.PublicKey)

    // compute verification code
    verification := qcrypto.VerificationCode(initiatorPublicKey, identity.PublicKey, initiatorNonce, responderNonce, req.TransactionID)

    // send challenge
    payload := map[uint64]any{
        0: responderDeviceID,
        1: append([]byte(nil), identity.PublicKey...),
        2: responderNonce,
        3: uint64(protocol.ProtocolVersion),
        4: respEphemeralPub,
        5: uint64(verification),
    }
    if err := c.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePairChallenge, req.TransactionID, payload)); err != nil {
        return err
    }

    // read authenticate
    auth, err := c.ReadMessage(ctx)
    if err != nil {
        return err
    }
    if auth.Type != protocol.MessageTypePairAuthenticate {
        return fmt.Errorf("unexpected auth type %d", auth.Type)
    }
    initiatorEphemeral, err := protocol.PayloadBytes(auth.Payload, 0)
    if err != nil {
        return err
    }
    initiatorSignature, err := protocol.PayloadBytes(auth.Payload, 1)
    if err != nil {
        return err
    }

    // build transcript and verify initiator signature
    transcript := qcrypto.TranscriptBytes(protocol.ProtocolVersion, req.TransactionID, initiatorDeviceID, responderDeviceID, initiatorPublicKey, identity.PublicKey, initiatorNonce, responderNonce, initiatorEphemeral, respEphemeralPub)
    if err := qcrypto.VerifyTranscript(initiatorPublicKey, transcript, initiatorSignature); err != nil {
        return fmt.Errorf("verify initiator signature: %w", err)
    }

    // read confirm
    conf, err := c.ReadMessage(ctx)
    if err != nil {
        return err
    }
    if conf.Type != protocol.MessageTypePairConfirm {
        return fmt.Errorf("unexpected confirm type %d", conf.Type)
    }

    // compute shared secret, master secret, confirmation MAC and responder signature
    shared, err := qcrypto.X25519SharedSecret(respEphemeralPriv, initiatorEphemeral)
    if err != nil {
        return err
    }
    master, err := qcrypto.MasterSecret(initiatorNonce, responderNonce, shared)
    if err != nil {
        return err
    }
    confirmationMAC := qcrypto.ConfirmationMAC(master, req.TransactionID)
    responderSignature := qcrypto.SignTranscript(identity.PrivateKey, transcript)

    completePayload := map[uint64]any{
        0: responderDeviceID,
        1: confirmationMAC,
        2: responderSignature,
    }
    if err := c.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePairComplete, req.TransactionID, completePayload)); err != nil {
        return err
    }

    log.Printf("paired with initiator %x", initiatorDeviceID)
    return nil
}
