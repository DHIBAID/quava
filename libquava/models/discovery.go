package models

const (
	TypeA     = 1
	TypePTR   = 12
	TypeSRV   = 33
	ClassINET = 1
)

// Header is the minimal DNS message header used by this prototype.
type Header struct {
	ID      uint16
	Flags   uint16
	QDCount uint16
	ANCount uint16
	NSCount uint16
	ARCount uint16
}

// Question is a minimal DNS question section entry.
type Question struct {
	Name  string
	Type  uint16
	Class uint16
}

// ResourceRecord is a minimal DNS resource record.
type ResourceRecord struct {
	Name  string
	Type  uint16
	Class uint16
	TTL   uint32
	Data  []byte
}

// Packet is a minimal DNS packet representation for the prototype.
type Packet struct {
	Header    Header
	Questions []Question
	Answers   []ResourceRecord
}

// PTRRecord stores the minimal payload of a DNS PTR record.
type PTRRecord struct {
	Name   string
	Target string
}

// SRVRecord stores the minimal payload of a DNS SRV record.
type SRVRecord struct {
	Name     string
	Target   string
	Port     int
	Priority uint16
	Weight   uint16
}

// ARecord stores the minimal payload of a DNS A record.
type ARecord struct {
	Name    string
	Address string
}