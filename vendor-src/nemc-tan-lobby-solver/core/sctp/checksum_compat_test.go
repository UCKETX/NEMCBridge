package sctp

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestChecksumWireCompatibility(t *testing.T) {
	// Published Pion SCTP fixture uses RFC 4960 CRC32C in little endian.
	standard := []byte{0x13, 0x88, 0x13, 0x88, 0, 0, 0, 0, 0x06, 0xa9, 0, 0xe1}
	p := &packet{}
	if err := p.unmarshal(true, standard); err != nil {
		t.Fatal(err)
	}
	if p.checksumMode != 1 {
		t.Fatalf("expected standard checksum, got %d", p.checksumMode)
	}
	encoded, err := p.marshal(true)
	if err != nil || !bytes.Equal(encoded, standard) {
		t.Fatalf("standard fixture mismatch: %x %v", encoded, err)
	}
	legacy := bytes.Clone(standard)
	binary.BigEndian.PutUint32(legacy[8:], generatePacketChecksum(legacy))
	p = &packet{}
	if err := p.unmarshal(true, legacy); err != nil {
		t.Fatal(err)
	}
	if p.checksumMode != 2 {
		t.Fatal("legacy checksum not identified")
	}
	encoded, err = p.marshal(true)
	if err != nil || !bytes.Equal(encoded, legacy) {
		t.Fatal("legacy wire format changed")
	}
	corrupted := bytes.Clone(standard)
	corrupted[4] ^= 1
	if err := p.unmarshal(true, corrupted); err == nil {
		t.Fatal("corrupt packet accepted")
	}
}
