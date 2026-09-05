package discovery

import (
	"encoding/binary"
	"libquava/models"
	"testing"
)

func TestDNSPacketRoundTrip(t *testing.T) {
	queryName := "_quava._udp.local."
	ptrTarget := "My-PC._quava._udp.local."
	srvTarget := "my-pc.local."

	header := models.Header{ID: 0x1234, Flags: 0x8180, QDCount: 1, ANCount: 2}
	question := models.Question{Name: queryName, Type: models.TypePTR, Class: models.ClassINET}
	ptrAnswerName := "_quava._udp.local."
	srvAnswerName := "My-PC._quava._udp.local."

	packet := EncodeHeader(header)
	packet = append(packet, EncodeQuestion(question)...)

	ptrData := EncodeDNSName(ptrTarget)
	ptrAnswer := append(EncodeDNSName(ptrAnswerName), make([]byte, 10)...)
	ptrNameLen := len(EncodeDNSName(ptrAnswerName))
	binary.BigEndian.PutUint16(ptrAnswer[ptrNameLen:ptrNameLen+2], models.TypePTR)
	binary.BigEndian.PutUint16(ptrAnswer[ptrNameLen+2:ptrNameLen+4], models.ClassINET)
	binary.BigEndian.PutUint32(ptrAnswer[ptrNameLen+4:ptrNameLen+8], 120)
	binary.BigEndian.PutUint16(ptrAnswer[ptrNameLen+8:ptrNameLen+10], uint16(len(ptrData)))
	ptrAnswer = append(ptrAnswer, ptrData...)
	packet = append(packet, ptrAnswer...)

	srvTargetData := EncodeDNSName(srvTarget)
	srvData := make([]byte, 6+len(srvTargetData))
	binary.BigEndian.PutUint16(srvData[0:2], 0)
	binary.BigEndian.PutUint16(srvData[2:4], 0)
	binary.BigEndian.PutUint16(srvData[4:6], 48273)
	copy(srvData[6:], srvTargetData)

	srvAnswer := append(EncodeDNSName(srvAnswerName), make([]byte, 10)...)
	srvNameLen := len(EncodeDNSName(srvAnswerName))
	binary.BigEndian.PutUint16(srvAnswer[srvNameLen:srvNameLen+2], models.TypeSRV)
	binary.BigEndian.PutUint16(srvAnswer[srvNameLen+2:srvNameLen+4], models.ClassINET)
	binary.BigEndian.PutUint32(srvAnswer[srvNameLen+4:srvNameLen+8], 120)
	binary.BigEndian.PutUint16(srvAnswer[srvNameLen+8:srvNameLen+10], uint16(len(srvData)))
	srvAnswer = append(srvAnswer, srvData...)
	packet = append(packet, srvAnswer...)

	parsed, err := ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket returned error: %v", err)
	}
	if len(parsed.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(parsed.Questions))
	}
	if parsed.Questions[0].Name != queryName {
		t.Fatalf("expected query name %q, got %q", queryName, parsed.Questions[0].Name)
	}
	if len(parsed.Answers) != 2 {
		t.Fatalf("expected 2 answers, got %d", len(parsed.Answers))
	}

	ptr, err := ParsePTRRecord(parsed.Answers[0])
	if err != nil {
		t.Fatalf("ParsePTRRecord failed: %v", err)
	}
	if ptr.Target != ptrTarget {
		t.Fatalf("expected PTR target %q, got %q", ptrTarget, ptr.Target)
	}

	srv, err := ParseSRVRecord(parsed.Answers[1])
	if err != nil {
		t.Fatalf("ParseSRVRecord failed: %v", err)
	}
	if srv.Name != srvAnswerName {
		t.Fatalf("expected SRV name %q, got %q", srvAnswerName, srv.Name)
	}
	if srv.Port != 48273 {
		t.Fatalf("expected SRV port 48273, got %d", srv.Port)
	}
	if srv.Target != srvTarget {
		t.Fatalf("expected SRV target %q, got %q", srvTarget, srv.Target)
	}
}
