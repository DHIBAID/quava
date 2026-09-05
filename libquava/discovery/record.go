package discovery

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
