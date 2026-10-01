package protocol

const DefaultMaxFrameSize = 64 * 1024

const (
	ProtocolName        = "quava-pairing"
	SessionProtocolName = "quava-session"
	ProtocolVersion     = 1

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
