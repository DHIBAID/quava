# Quava Runtime Sessions

Pairing and runtime sessions are separate layers.

```text
Discovery
   |
   v
TCP connection
   |
   +--> PAIR_REQUEST ... PAIR_COMPLETE  (first-time trust)
   |
   +--> SESSION_HELLO ... SESSION_READY  (existing trust)
                                      |
                                      v
                              long-lived session
                                      |
                                PING / PONG
```

The Android foreground service keeps the TCP listener alive. It does not keep a socket to
the PC alive when there is no active session. When a PC connects using an already-paired
identity, the Android service authenticates the peer and keeps that socket open.

The Go `session` package provides the corresponding initiator-side handshake and a
heartbeat loop. `quavad` owns that state and runs it independently of the CLI. Start
`quavad` (normally through systemd), discover the peer ID, then ask the CLI to connect:

```bash
cd daemon/quavad && go run .
cd ../cli && go run . discover
go run . connect <peer_device_id>
```

All CLI commands identify a peer by `peer_device_id`; display names are informational
only. `connect` returns once the daemon has scheduled the durable session, and `ping`
uses that daemon-owned session.

## Current limitation

The runtime handshake authenticates both endpoints using the persistent pairing
credential, but runtime payload encryption has not yet been wired into the message layer.
Do not send sensitive application payloads over the current runtime session until the
session encryption layer is implemented.
