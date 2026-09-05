package discovery

import (
	"encoding/binary"
	"fmt"
	"strings"
	
	"libquava/models"
)




// ParseHeader decodes a DNS header from data.
func ParseHeader(data []byte) (models.Header, error) {
	if len(data) < 12 {
		return models.Header{}, fmt.Errorf("dns: short header: %d", len(data))
	}
	return models.Header{
		ID:      binary.BigEndian.Uint16(data[0:2]),
		Flags:   binary.BigEndian.Uint16(data[2:4]),
		QDCount: binary.BigEndian.Uint16(data[4:6]),
		ANCount: binary.BigEndian.Uint16(data[6:8]),
		NSCount: binary.BigEndian.Uint16(data[8:10]),
		ARCount: binary.BigEndian.Uint16(data[10:12]),
	}, nil
}

// EncodeHeader encodes a DNS header.
func EncodeHeader(h models.Header) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], h.ID)
	binary.BigEndian.PutUint16(buf[2:4], h.Flags)
	binary.BigEndian.PutUint16(buf[4:6], h.QDCount)
	binary.BigEndian.PutUint16(buf[6:8], h.ANCount)
	binary.BigEndian.PutUint16(buf[8:10], h.NSCount)
	binary.BigEndian.PutUint16(buf[10:12], h.ARCount)
	return buf
}

// ParsePacket parses a minimal DNS packet.
func ParsePacket(data []byte) (*models.Packet, error) {
	header, err := ParseHeader(data)
	if err != nil {
		return nil, err
	}

	packet := &models.Packet{Header: header}
	offset := 12

	for i := 0; i < int(header.QDCount); i++ {
		question, next, err := parseQuestion(data, offset)
		if err != nil {
			return nil, err
		}
		packet.Questions = append(packet.Questions, question)
		offset = next
	}

	for i := 0; i < int(header.ANCount); i++ {
		rr, next, err := parseResourceRecord(data, offset)
		if err != nil {
			return nil, err
		}
		packet.Answers = append(packet.Answers, rr)
		offset = next
	}

	for i := 0; i < int(header.NSCount); i++ {
		_, next, err := parseResourceRecord(data, offset)
		if err != nil {
			return nil, err
		}
		offset = next
	}

	for i := 0; i < int(header.ARCount); i++ {
		_, next, err := parseResourceRecord(data, offset)
		if err != nil {
			return nil, err
		}
		offset = next
	}

	return packet, nil
}

// EncodeDNSName encodes a DNS name with standard label length encoding.
func EncodeDNSName(name string) []byte {
	trimmed := strings.TrimSuffix(name, ".")
	if trimmed == "" {
		return []byte{0}
	}

	labels := strings.Split(trimmed, ".")
	buf := make([]byte, 0, len(trimmed)+2)
	for _, label := range labels {
		if len(label) > 63 {
			panic("dns: label too long")
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, []byte(label)...)
	}
	buf = append(buf, 0)
	return buf
}

// DecodeDNSName decodes a DNS name and returns the next offset after it.
func DecodeDNSName(data []byte, offset int) (string, int, error) {
	if offset < 0 || offset >= len(data) {
		return "", 0, fmt.Errorf("dns: invalid name offset %d", offset)
	}

	labels := make([]string, 0, 4)
	seenPointer := false
	originalOffset := offset

	for {
		if offset >= len(data) {
			return "", 0, fmt.Errorf("dns: truncated name")
		}

		length := int(data[offset])
		if length == 0 {
			offset++
			break
		}

		if length&0xC0 == 0xC0 {
			if offset+1 >= len(data) {
				return "", 0, fmt.Errorf("dns: bad compression pointer")
			}
			pointer := int(binary.BigEndian.Uint16(data[offset:offset+2]) & 0x3FFF)
			if !seenPointer {
				originalOffset = offset + 2
			}
			seenPointer = true
			offset = pointer
			continue
		}

		offset++
		if offset+length > len(data) {
			return "", 0, fmt.Errorf("dns: name label exceeds buffer")
		}
		labels = append(labels, string(data[offset:offset+length]))
		offset += length
	}

	if len(labels) == 0 {
		if seenPointer {
			return ".", originalOffset, nil
		}
		return ".", offset, nil
	}

	name := strings.Join(labels, ".") + "."
	if seenPointer {
		return name, originalOffset, nil
	}
	return name, offset, nil
}

func parseQuestion(data []byte, offset int) (models.Question, int, error) {
	name, next, err := DecodeDNSName(data, offset)
	if err != nil {
		return models.Question{}, 0, err
	}
	if next+4 > len(data) {
		return models.Question{}, 0, fmt.Errorf("dns: truncated question")
	}

	qtype := binary.BigEndian.Uint16(data[next : next+2])
	qclass := binary.BigEndian.Uint16(data[next+2 : next+4])
	return models.Question{Name: name, Type: qtype, Class: qclass}, next + 4, nil
}

func parseResourceRecord(data []byte, offset int) (models.ResourceRecord, int, error) {
	name, next, err := DecodeDNSName(data, offset)
	if err != nil {
		return models.ResourceRecord{}, 0, err
	}
	if next+10 > len(data) {
		return models.ResourceRecord{}, 0, fmt.Errorf("dns: truncated resource record")
	}

	rtype := binary.BigEndian.Uint16(data[next : next+2])
	clazz := binary.BigEndian.Uint16(data[next+2 : next+4])
	ttl := binary.BigEndian.Uint32(data[next+4 : next+8])
	rdlength := int(binary.BigEndian.Uint16(data[next+8 : next+10]))
	if next+10+rdlength > len(data) {
		return models.ResourceRecord{}, 0, fmt.Errorf("dns: resource record too large")
	}
	payload := append([]byte(nil), data[next+10:next+10+rdlength]...)
	return models.ResourceRecord{Name: name, Type: rtype, Class: clazz, TTL: ttl, Data: payload}, next + 10 + rdlength, nil
}

// EncodeQuestion encodes a single DNS question.
func EncodeQuestion(question models.Question) []byte {
	buf := EncodeDNSName(question.Name)
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:2], question.Type)
	binary.BigEndian.PutUint16(b[2:4], question.Class)
	return append(buf, b...)
}

// BuildPTRQuery creates a minimal PTR query.
func BuildPTRQuery(name string) ([]byte, error) {
	header := EncodeHeader(models.Header{ID: 0x1234, Flags: 0x0100, QDCount: 1})
	query := EncodeQuestion(models.Question{Name: name, Type: models.TypePTR, Class: models.ClassINET})
	return append(header, query...), nil
}

// ParsePTRRecord decodes a PTR record's payload.
func ParsePTRRecord(rr models.ResourceRecord) (PTRRecord, error) {
	if rr.Type != models.TypePTR {
		return PTRRecord{}, fmt.Errorf("dns: expected PTR record, got type %d", rr.Type)
	}
	name, _, err := DecodeDNSName(rr.Data, 0)
	if err != nil {
		return PTRRecord{}, err
	}
	return PTRRecord{Name: rr.Name, Target: name}, nil
}

// ParseSRVRecord decodes an SRV record's payload.
func ParseSRVRecord(rr models.ResourceRecord) (SRVRecord, error) {
	if rr.Type != models.TypeSRV {
		return SRVRecord{}, fmt.Errorf("dns: expected SRV record, got type %d", rr.Type)
	}
	if len(rr.Data) < 7 {
		return SRVRecord{}, fmt.Errorf("dns: SRV data too short")
	}

	priority := binary.BigEndian.Uint16(rr.Data[0:2])
	weight := binary.BigEndian.Uint16(rr.Data[2:4])
	port := int(binary.BigEndian.Uint16(rr.Data[4:6]))
	target, _, err := DecodeDNSName(rr.Data, 6)
	if err != nil {
		return SRVRecord{}, err
	}
	return SRVRecord{
		Name:     rr.Name,
		Target:   target,
		Port:     port,
		Priority: priority,
		Weight:   weight,
	}, nil
}

// ParseARecord decodes an A record's payload.
func ParseARecord(rr models.ResourceRecord) (ARecord, error) {
	if rr.Type != models.TypeA {
		return ARecord{}, fmt.Errorf("dns: expected A record, got type %d", rr.Type)
	}
	if len(rr.Data) != 4 {
		return ARecord{}, fmt.Errorf("dns: invalid A record length %d", len(rr.Data))
	}

	return ARecord{
		Name:    rr.Name,
		Address: fmt.Sprintf("%d.%d.%d.%d", rr.Data[0], rr.Data[1], rr.Data[2], rr.Data[3]),
	}, nil
}
