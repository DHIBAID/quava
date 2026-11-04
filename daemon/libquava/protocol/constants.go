package protocol

const DefaultMaxFrameSize = 64 * 1024

// Payload limits. A whole frame is capped at DefaultMaxFrameSize, so large
// payloads leave room for the message wrapped around them.
const (
	MaxClipboardBytes = 48 * 1024
	MaxArtworkBytes   = 48 * 1024
)

const (
	ProtocolName        = "quava-pairing"
	SessionProtocolName = "quava-session"
	// Version 2 introduced encrypted session frames; version 1 peers are
	// rejected at the handshake instead of silently speaking plaintext.
	ProtocolVersion = 2

	MessageTypePairRequest      = 0x01
	MessageTypePairChallenge    = 0x02
	MessageTypePairAuthenticate = 0x03
	MessageTypePairConfirm      = 0x04
	MessageTypePairComplete     = 0x05
	MessageTypePairReject       = 0x06
	MessageTypePairCancel       = 0x07

	MessageTypeSessionHello        = 0x10
	MessageTypeSessionChallenge    = 0x11
	MessageTypeSessionAuthenticate = 0x12
	MessageTypeSessionReady        = 0x13

	MessageTypePing    = 0x20
	MessageTypePingAck = 0x21

	MessageTypeRing    = 0x22
	MessageTypeRingAck = 0x23

	MessageTypeSyncClipboard    = 0x24
	MessageTypeSyncClipboardAck = 0x25

	MessageTypeSyncNotification     = 0x26
	MessageTypeSyncNotificationAck  = 0x27
	MessageTypeClearNotification    = 0x28
	MessageTypeClearNotificationAck = 0x29

	// Android-originated notification controls. These use Android's stable
	// StatusBarNotification key rather than the D-Bus numeric ID.
	MessageTypeNotificationAction            = 0x2a
	MessageTypeNotificationActionAck         = 0x2b
	MessageTypeDismissAndroidNotification    = 0x2c
	MessageTypeDismissAndroidNotificationAck = 0x2d
	// Someone needs to find a better name for these constants.

	// InputEvent is sent asynchronously by Android after a session is ready.
	// InputEventAck mirrors its transaction ID after the host has accepted it.
	MessageTypeInputEvent    = 0x2e
	MessageTypeInputEventAck = 0x2f

	// Funny comment on top because everything else has one.
	MessageTypeMultimedia      = 0x30
	MessageTypeMultimediaAck   = 0x31
	MessageTypeMultimediaState = 0x32
	// The above is the only one without an Ack pair, wow.

	// Either side may send Unpair on an authenticated session to say "forget
	// me". The receiver removes the sender's trust record and acknowledges.
	MessageTypeUnpair    = 0x33
	MessageTypeUnpairAck = 0x34

	// DeviceStatus is sent by the phone whenever its battery, radios or
	// storage change. It needs no acknowledgement.
	MessageTypeDeviceStatus = 0x35

	// CallState is sent by the phone whenever an active cellular call starts
	// or ends, so the host can mute/unmute its own audio. Acknowledged so the
	// phone knows the mute actually took effect.
	MessageTypeCallState    = 0x36
	MessageTypeCallStateAck = 0x37

	// MultimediaStateUpdateType identifies whether a multimedia-state payload
	// contains the complete state or only the current playback position.
	MultimediaStateUpdateType uint64 = 20
	MultimediaStateFull       uint64 = 0
	MultimediaStatePosition   uint64 = 1
	// MultimediaStateIcon carries one player's 64x64 PNG icon:
	// key 2 = bus name, key 4 = PNG bytes. Sent separately from full state so
	// it can never push a frame past the size limit.
	MultimediaStateIcon uint64 = 2

	// PingPayloadSilent marks a heartbeat Ping that should not generate a
	// user-visible notification on the receiving device.
	PingPayloadSilent = 0

	ReasonUserRejected         = 0x01
	ReasonAuthenticationFailed = 0x02
	ReasonUnsupportedVersion   = 0x03
	ReasonInvalidMessage       = 0x04
	ReasonTimeout              = 0x05
	ReasonCancelled            = 0x06
	ReasonIdentityConflict     = 0x07
	ReasonInternalError        = 0x08
)
