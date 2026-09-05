# Quava Pairing Wire Protocol

**Protocol:** Quava Pairing Protocol
**Version:** `1.0`
**Encoding:** CBOR
**Transport:** Reliable, ordered byte stream
**Cryptography:** Ed25519 + X25519 + HKDF-SHA-256 + ChaCha20-Poly1305

---

# 1. Protocol Stack

The pairing protocol operates above a reliable transport.

```text
┌─────────────────────────────────────┐
│          Quava Pairing               │
├─────────────────────────────────────┤
│          Quava Framing               │
├─────────────────────────────────────┤
│          Reliable Transport          │
├─────────────────────────────────────┤
│       TCP / Bluetooth / USB / ...    │
└─────────────────────────────────────┘
```

The transport is not trusted.

Transport-level encryption, if available, does not replace Quava authentication.

---

# 2. Cryptographic Primitives

All Quava 1.0 implementations MUST use the following primitives.

| Purpose                  | Primitive         |
| ------------------------ | ----------------- |
| Device identity          | Ed25519           |
| Identity fingerprint     | SHA-256           |
| Ephemeral key exchange   | X25519            |
| Key derivation           | HKDF-SHA-256      |
| Authenticated encryption | ChaCha20-Poly1305 |
| Randomness               | OS CSPRNG         |

Implementations MUST use well-tested platform/library implementations.

Quava MUST NOT implement these primitives itself.

---

# 3. Device Identity

Each Quava installation owns an Ed25519 key pair.

```text
Identity
├── Private Key: 32 bytes
└── Public Key: 32 bytes
```

The private key never leaves the device.

The public key is the device's cryptographic identity.

---

# 4. Device ID

The Device ID is derived from the Ed25519 public key.

```text
device_id = SHA256(public_key)
```

The wire representation is:

```text
device_id = first 16 bytes of SHA256(public_key)
```

represented as lowercase hexadecimal.

Example:

```text
8f3b7a2c1e9d...
```

The Device ID is therefore 32 hexadecimal characters.

The Device ID is an identifier, not a secret.

---

# 5. Pairing Roles

The protocol has two roles:

```text
Initiator
Responder
```

The Initiator is the device on which the user starts pairing.

The Responder is the device receiving the pairing request.

Roles exist only for the duration of the pairing transaction.

---

# 6. Protocol Constants

The following strings are protocol constants and MUST NOT be changed between compatible implementations.

```text
protocol_name = "quava-pairing"
protocol_version = 1
```

HKDF context:

```text
"quava-pairing-v1"
```

---

# 7. Message Encoding

All protocol messages are encoded using **CBOR**.

Each message is a CBOR map.

Integer keys are used instead of string keys to reduce message size and avoid ambiguity.

## 7.1 Common Fields

| Key | Name           | Type |
| --: | -------------- | ---- |
| `0` | version        | uint |
| `1` | type           | uint |
| `2` | transaction_id | bstr |
| `3` | payload        | map  |

Example:

```text
{
    0: 1,
    1: PAIR_REQUEST,
    2: <transaction_id>,
    3: { ... }
}
```

Unknown fields MUST be ignored unless explicitly marked critical.

Unknown message types MUST result in a protocol error.

---

# 8. Message Types

```text
0x01  PAIR_REQUEST
0x02  PAIR_CHALLENGE
0x03  PAIR_AUTHENTICATE
0x04  PAIR_CONFIRM
0x05  PAIR_COMPLETE
0x06  PAIR_REJECT
0x07  PAIR_CANCEL
```

Values are permanent protocol identifiers.

---

# 9. Transaction ID

Every pairing attempt has a unique transaction ID.

```text
transaction_id = 16 random bytes
```

The transaction ID MUST be generated using a cryptographically secure random number generator.

It MUST NOT be reused.

A transaction ID identifies the pairing attempt, not the device.

---

# 10. Nonces

Each side generates a fresh random nonce.

```text
initiator_nonce = 32 random bytes
responder_nonce = 32 random bytes
```

Nonces MUST NOT be reused across pairing transactions.

---

# 11. Pairing Request

The Initiator begins pairing with:

```text
PAIR_REQUEST
```

## 11.1 Payload

```text
{
    0: initiator_device_id,
    1: initiator_public_key,
    2: initiator_nonce,
    3: supported_versions,
    4: device_name,
    5: capabilities
}
```

### Fields

| Key | Field              | Type        |
| --: | ------------------ | ----------- |
| `0` | device_id          | bstr(16)    |
| `1` | public_key         | bstr(32)    |
| `2` | nonce              | bstr(32)    |
| `3` | supported_versions | array<uint> |
| `4` | device_name        | text        |
| `5` | capabilities       | array<uint> |

`device_name` is informational and MUST NOT be trusted as an identity claim.

`capabilities` are informational until authorization has been established.

---

# 12. Pair Request Validation

The Responder MUST:

1. Validate the message structure.
2. Validate the protocol version.
3. Validate the public-key length.
4. Validate the nonce length.
5. Recalculate the Device ID from the public key.
6. Compare the calculated Device ID against the supplied Device ID.
7. Reject the request if they do not match.

The Device ID is therefore not independently trusted.

---

# 13. Pair Challenge

The Responder replies with:

```text
PAIR_CHALLENGE
```

## 13.1 Payload

```text
{
    0: responder_device_id,
    1: responder_public_key,
    2: responder_nonce,
    3: selected_version,
    4: ephemeral_public_key,
    5: verification_code
}
```

| Key | Field                | Type     |
| --: | -------------------- | -------- |
| `0` | device_id            | bstr(16) |
| `1` | public_key           | bstr(32) |
| `2` | nonce                | bstr(32) |
| `3` | selected_version     | uint     |
| `4` | ephemeral_public_key | bstr(32) |
| `5` | verification_code    | uint     |

---

# 14. Responder Ephemeral Key

The Responder generates a fresh X25519 key pair:

```text
responder_ephemeral_private
responder_ephemeral_public
```

The private key exists only for the pairing transaction.

The public key is sent in `PAIR_CHALLENGE`.

---

# 15. Verification Code

The verification code is a six-digit decimal value.

It is derived from the authenticated pairing transcript.

The exact calculation is:

```text
verification_input =
    "quava-pairing-code" ||
    initiator_public_key ||
    responder_public_key ||
    initiator_nonce ||
    responder_nonce ||
    transaction_id
```

Then:

```text
digest = SHA256(verification_input)

verification_code =
    Integer(digest[0:4]) mod 1_000_000
```

The displayed value is zero-padded:

```text
000000 - 999999
```

Example:

```text
483921
```

Both devices independently calculate the same value.

The code is NOT itself a cryptographic credential.

Its purpose is human verification of the authenticated transcript.

---

# 16. User Verification

Both devices display:

```text
Pairing code:

483 921
```

The user verifies that the codes match.

The UI MUST provide an explicit confirmation action.

Examples:

```text
[Cancel] [Confirm]
```

or:

```text
[Reject] [Pair]
```

Closing the dialog, timing out, or navigating away MUST NOT count as confirmation.

---

# 17. Pair Authenticate

The Initiator generates an ephemeral X25519 key pair:

```text
initiator_ephemeral_private
initiator_ephemeral_public
```

It then derives the shared secret:

```text
shared_secret =
    X25519(
        initiator_ephemeral_private,
        responder_ephemeral_public
    )
```

The Initiator sends:

```text
PAIR_AUTHENTICATE
```

## 17.1 Payload

```text
{
    0: initiator_ephemeral_public,
    1: identity_signature
}
```

---

# 18. Identity Signature

The Initiator signs the complete pairing transcript.

The signed data is:

```text
transcript =
    protocol_name ||
    selected_version ||
    transaction_id ||
    initiator_device_id ||
    responder_device_id ||
    initiator_public_key ||
    responder_public_key ||
    initiator_nonce ||
    responder_nonce ||
    initiator_ephemeral_public ||
    responder_ephemeral_public
```

The Initiator computes:

```text
signature =
    Ed25519.Sign(
        initiator_private_key,
        SHA256(transcript)
    )
```

The signature is 64 bytes.

---

# 19. Responder Authentication

The Responder verifies:

```text
Ed25519.Verify(
    initiator_public_key,
    signature,
    SHA256(transcript)
)
```

If verification fails:

```text
PAIR_REJECT
reason = AUTHENTICATION_FAILED
```

The transaction is terminated.

No trust is established.

---

# 20. Responder Signature

The Responder also signs the same transcript.

The signature is:

```text
responder_signature =
    Ed25519.Sign(
        responder_private_key,
        SHA256(transcript)
    )
```

This proves that the Responder controls the private key corresponding to the identity it advertised.

---

# 21. Pair Confirm

After successful cryptographic authentication and user confirmation, the Initiator sends:

```text
PAIR_CONFIRM
```

## 21.1 Payload

```text
{
    0: responder_signature,
    1: initiator_confirmation
}
```

The Responder MUST verify:

1. The Initiator signature.
2. The Responder's own transcript.
3. The transaction ID.
4. The selected protocol version.
5. The user confirmation state.

---

# 22. Key Derivation

Both sides now possess:

```text
shared_secret
```

The salt is:

```text
salt =
    SHA256(
        initiator_nonce ||
        responder_nonce
    )
```

The key derivation context is:

```text
info = "quava-pairing-v1"
```

The master secret is:

```text
master_secret =
    HKDF-SHA256(
        salt,
        shared_secret,
        info,
        32
    )
```

The master secret MUST NOT be logged or transmitted.

---

# 23. Trust Credential

The pairing protocol derives a peer credential from the master secret.

```text
peer_credential =
    HKDF-SHA256(
        salt = SHA256(transaction_id),
        ikm = master_secret,
        info = "quava-peer-credential-v1",
        length = 32
    )
```

The exact persistence policy for this credential is implementation-defined.

The identity public keys MUST always remain associated with the trust record.

---

# 24. Pair Complete

After the Responder has successfully persisted the trust relationship, it sends:

```text
PAIR_COMPLETE
```

## 24.1 Payload

```text
{
    0: responder_device_id,
    1: confirmation_mac
}
```

The confirmation MAC is:

```text
confirmation_mac =
    HMAC-SHA256(
        master_secret,
        "quava-pair-complete" ||
        transaction_id
    )
```

---

# 25. Completion Rules

A device MUST NOT consider pairing complete until:

1. The peer identity has been authenticated.
2. The human verification step has succeeded.
3. The cryptographic key exchange has succeeded.
4. The local trust record has been persisted.
5. `PAIR_COMPLETE` has been successfully processed.

After completion:

```text
state = TRUSTED
```

---

# 26. Pair Reject

Any party may reject the transaction with:

```text
PAIR_REJECT
```

Payload:

```text
{
    0: reason
}
```

Reason codes:

```text
0x01 USER_REJECTED
0x02 AUTHENTICATION_FAILED
0x03 UNSUPPORTED_VERSION
0x04 INVALID_MESSAGE
0x05 TIMEOUT
0x06 CANCELLED
0x07 IDENTITY_CONFLICT
0x08 INTERNAL_ERROR
```

Reason codes are informational and MUST NOT expose sensitive implementation details.

---

# 27. Pair Cancel

A user may cancel an active pairing operation.

```text
PAIR_CANCEL
```

Payload:

```text
{
    0: reason
}
```

The receiving device MUST terminate the transaction.

Any subsequent message belonging to the cancelled transaction MUST be rejected.

---

# 28. Message Sequence

A successful pairing follows:

```text
Initiator                         Responder
    │                                 │
    │        PAIR_REQUEST             │
    │────────────────────────────────>│
    │                                 │
    │        PAIR_CHALLENGE           │
    │<────────────────────────────────│
    │                                 │
    │       PAIR_AUTHENTICATE         │
    │────────────────────────────────>│
    │                                 │
    │      Human verification         │
    │<───────────────────────────────>│
    │                                 │
    │         PAIR_CONFIRM            │
    │<───────────────────────────────>│
    │                                 │
    │      Persist trust              │
    │<───────────────────────────────>│
    │                                 │
    │         PAIR_COMPLETE           │
    │<────────────────────────────────│
    │                                 │
    ▼                                 ▼
 TRUSTED                           TRUSTED
```

---

# 29. Exact State Machine

## 29.1 Initiator

```text
IDLE
  │
  │ startPairing()
  ▼
REQUEST_SENT
  │
  │ PAIR_CHALLENGE
  ▼
CHALLENGED
  │
  │ PAIR_AUTHENTICATE
  ▼
AUTHENTICATING
  │
  │ user confirms
  ▼
CONFIRMING
  │
  │ PAIR_COMPLETE
  ▼
TRUSTED
```

Any error transitions to:

```text
FAILED
  │
  ▼
IDLE
```

---

## 29.2 Responder

```text
IDLE
  │
  │ PAIR_REQUEST
  ▼
PENDING
  │
  │ PAIR_AUTHENTICATE
  ▼
AUTHENTICATING
  │
  │ user confirms
  ▼
CONFIRMING
  │
  │ persist trust
  ▼
TRUSTED
```

Any failure transitions to:

```text
FAILED
  │
  ▼
IDLE
```

---

# 30. Transcript Binding

The transcript is the central security object of the pairing protocol.

Every cryptographic authentication operation MUST be bound to:

```text
protocol version
transaction ID

Initiator identity
Responder identity

Initiator public key
Responder public key

Initiator nonce
Responder nonce

Initiator ephemeral key
Responder ephemeral key
```

This prevents an attacker from mixing components from separate pairing attempts.

---

# 31. Replay Protection

Replay protection is provided by:

```text
transaction_id
initiator_nonce
responder_nonce
ephemeral keys
```

Every pairing attempt therefore produces a new cryptographic transcript.

A previously captured:

```text
PAIR_REQUEST
PAIR_CHALLENGE
PAIR_AUTHENTICATE
PAIR_CONFIRM
```

sequence MUST NOT be valid in a new transaction.

---

# 32. Identity Substitution Protection

The Device ID and public key are cryptographically bound.

Given:

```text
device_id = SHA256(public_key)[0:16]
```

an attacker cannot substitute:

```text
Device ID A
Public Key B
```

without detection.

The complete transcript is additionally signed by both identities.

---

# 33. Man-in-the-Middle Protection

The X25519 exchange creates a shared secret:

```text
Initiator ephemeral key
          +
Responder ephemeral key
          ↓
     shared_secret
```

The long-term Ed25519 signatures authenticate the ephemeral keys.

The verification code allows the user to verify that the authenticated endpoints correspond to the devices they intended to pair.

An attacker positioned between the devices therefore cannot silently replace the cryptographic identities without causing authentication or verification failure.

---

# 34. Trust Record

After successful pairing, each device stores:

```text
Peer
├── device_id
├── public_key
├── device_name
├── protocol_version
├── permissions
├── peer_credential
└── paired_at
```

The peer public key is the authoritative identity.

The device name is mutable metadata.

---

# 35. Reconnection

Pairing is NOT repeated for every connection.

A subsequent connection performs a separate session-authentication protocol.

Conceptually:

```text
Existing Trust
      ↓
New Transport Connection
      ↓
Session Handshake
      ↓
Authenticate Peer
      ↓
Derive Session Keys
      ↓
Authorized Session
```

The session protocol is outside this document.

---

# 36. Unpairing

Unpairing is not performed by pretending that a new pairing request occurred.

It is an explicit revocation operation.

Local unpairing MUST:

```text
1. Remove peer authorization.
2. Invalidate active sessions.
3. Remove/invalidate peer credentials.
4. Prevent future authentication.
```

After local unpairing:

```text
TRUSTED
   ↓
REVOKED
```

The peer MUST NOT automatically regain trust.

---

# 37. Identity Changes

A trusted peer is identified by its public key.

If a peer previously known as:

```text
Device ID = A
Public Key = KEY_A
```

later presents:

```text
Device ID = A
Public Key = KEY_B
```

the connection MUST be rejected.

The user must explicitly approve the new identity through a new pairing flow.

---

# 38. Pairing Timeout

A pairing transaction expires after:

```text
120 seconds
```

The timeout applies to the complete interactive pairing operation.

On timeout:

```text
PENDING
   ↓
TIMEOUT
   ↓
FAILED
```

No trust is established.

---

# 39. Maximum Message Size

Implementations MUST reject frames larger than:

```text
64 KiB
```

Pairing messages should normally be significantly smaller.

This prevents a malicious peer from using pairing as an uncontrolled memory-allocation mechanism.

---

# 40. Frame Format

The underlying byte stream uses length-prefixed frames.

```text
┌──────────────┬──────────────────────┐
│ Length       │ CBOR Message         │
│ 4 bytes      │ Length bytes         │
└──────────────┴──────────────────────┘
```

Length is:

```text
unsigned 32-bit
big-endian
```

The length specifies the number of bytes in the CBOR message.

Example:

```text
00 00 01 2A
<298 bytes of CBOR>
```

A length greater than `65536` MUST be rejected.

---

# 41. Protocol Errors

Protocol errors are connection-level failures.

An implementation MUST terminate the pairing transaction if it receives:

* malformed CBOR,
* invalid frame length,
* unknown mandatory message,
* invalid message ordering,
* invalid transaction ID,
* invalid field type,
* invalid field length,
* invalid cryptographic signature,
* invalid authentication data.

---

# 42. Unknown Fields

Unknown fields SHOULD be ignored.

This permits backwards-compatible extension.

Unknown fields MUST NOT alter the interpretation of existing fields.

Critical extensions may be introduced in a future protocol version.

---

# 43. Unknown Message Types

Unknown message types MUST NOT be silently ignored.

The receiver MUST terminate the pairing transaction with:

```text
UNSUPPORTED_VERSION
```

or an appropriate protocol error.

---

# 44. Simultaneous Pairing

Two devices may initiate pairing with one another simultaneously.

Example:

```text
Laptop ───── Pair ─────> Phone
Laptop <──── Pair ────── Phone
```

Implementations SHOULD detect that both transactions involve the same identities and collapse them into a single pairing interaction.

If this cannot be safely resolved, both transactions may be rejected and the user can retry.

The implementation MUST NOT create two independent trust records for the same peer identity.

---

# 45. Existing Trusted Peer

If a `PAIR_REQUEST` is received from an already trusted public key:

```text
Known Public Key
       ↓
PAIR_REQUEST
```

the implementation SHOULD NOT create a second peer entry.

It may instead:

```text
re-authenticate
refresh metadata
```

or reject the request as unnecessary.

If the presented public key differs from the stored identity:

```text
IDENTITY_CONFLICT
```

must be raised.

---

# 46. Security-Critical Invariants

The following MUST always hold.

### Identity

```text
Device ID ↔ Public Key
```

### Authentication

```text
Identity ↔ Signed Transcript
```

### Key Exchange

```text
Ephemeral Keys ↔ Signed Transcript
```

### Human Verification

```text
Verification Code ↔ Complete Transcript
```

### Trust

```text
Trust ↔ Authenticated Public Key
```

### Session

```text
Session ↔ Existing Trust
```

No layer may bypass the layer above it.

---

# 47. Complete Cryptographic Flow

```text
                 INITIATOR                  RESPONDER

Ed25519 Key A                              Ed25519 Key B
X25519 Key A'                              X25519 Key B'

      │                                         │
      │ public_key A                            │
      │ nonce A                                 │
      │────────────────────────────────────────>│
      │                                         │
      │                         public_key B     │
      │                         nonce B          │
      │                         X25519 key B'    │
      │<────────────────────────────────────────│
      │                                         │
      │ X25519(A', B')                          │
      │─────────────────────────────────────────│
      │                                         │
      │              shared_secret              │
      │<────────────────────────────────────────>│
      │                                         │
      │ Sign(transcript, Key A)                 │
      │────────────────────────────────────────>│
      │                                         │
      │                    Sign(transcript, B)   │
      │<────────────────────────────────────────│
      │                                         │
      │         Human verification              │
      │<────────────────────────────────────────>│
      │                                         │
      │             HKDF(shared_secret)         │
      │<────────────────────────────────────────>│
      │                                         │
      │              TRUST ESTABLISHED          │
      │<────────────────────────────────────────>│
```

---

# 48. What Pairing Guarantees

After successful pairing, Quava guarantees that:

1. Both devices possess the private keys corresponding to their advertised identities.
2. The authenticated identities were bound to the current pairing transaction.
3. The user explicitly approved the pairing.
4. Both devices derived the same pairing secret.
5. The trust relationship is associated with the peer's cryptographic identity.
6. Future sessions can authenticate against this trust relationship.

---

# 49. What Pairing Does Not Guarantee

Pairing does NOT guarantee that:

* the operating system is uncompromised,
* the Quava application is uncompromised,
* the user account is secure,
* the device remains trustworthy forever,
* the device is physically controlled by its owner,
* a malicious user cannot abuse permissions granted to a trusted device.

Trust can always be revoked through unpairing.

---

# 50. Implementation Checklist

A compliant implementation must implement:

* [ ] Ed25519 device identity
* [ ] Device ID derivation
* [ ] X25519 ephemeral exchange
* [ ] HKDF-SHA-256
* [ ] ChaCha20-Poly1305 where session encryption is required
* [ ] CBOR encoding
* [ ] Length-prefixed framing
* [ ] Transaction IDs
* [ ] Fresh nonces
* [ ] Signed pairing transcript
* [ ] Verification code
* [ ] Explicit user confirmation
* [ ] Pairing timeout
* [ ] State machine enforcement
* [ ] Identity conflict detection
* [ ] Persistent trust records
* [ ] Session invalidation on unpair
* [ ] Replay protection
* [ ] Malformed-message rejection

---

# 51. Protocol Summary

The entire pairing protocol can be represented as:

```text
DISCOVERY
    │
    │  untrusted
    ▼
PAIR_REQUEST
    │
    ▼
PAIR_CHALLENGE
    │
    ▼
EPHEMERAL KEY EXCHANGE
    │
    ▼
SIGNED TRANSCRIPT
    │
    ▼
IDENTITY AUTHENTICATION
    │
    ▼
HUMAN VERIFICATION
    │
    ▼
HKDF KEY DERIVATION
    │
    ▼
PERSIST TRUST
    │
    ▼
PAIR_COMPLETE
    │
    ▼
TRUSTED
```

The fundamental security relationship is:

```text
                Ed25519 Identity
                       │
                       ▼
              Signed Transcript
                       │
                       ▼
              X25519 Shared Secret
                       │
                       ▼
                 HKDF Secrets
                       │
                       ▼
                Persisted Trust
                       │
                       ▼
              Authenticated Sessions
                       │
                       ▼
                 Feature Access
```

No step should be skipped.
