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
heartbeat loop. The CLI exposes this as:

```bash
go run . connect "Quava Android"
```

The command intentionally remains attached to the session. This is a prototype of the
future always-running Linux daemon's session manager: the eventual daemon can reuse the
same `session.Connect` API and retain the resulting `Session` in a manager instead of
terminating with the CLI.

## Current limitation

The runtime handshake authenticates both endpoints using the persistent pairing
credential, but runtime payload encryption has not yet been wired into the message layer.
Do not send sensitive application payloads over the current runtime session until the
session encryption layer is implemented.
