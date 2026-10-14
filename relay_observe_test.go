package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"strings"
	"testing"
)

func TestEarlyLoginDiagnosticsExcludeJWT(t *testing.T) {
	privateJWT := "sensitive.fixture.jwt"
	identity := []byte(`{"chain":["` + privateJWT + `"]}`)
	connection := binary.LittleEndian.AppendUint32(nil, uint32(len(identity)))
	connection = append(connection, identity...)
	connection = binary.LittleEndian.AppendUint32(connection, uint32(len(privateJWT)))
	connection = append(connection, privateJWT...)
	login := []byte{1, 0, 0, 3, 0x5c}
	login = binary.AppendUvarint(login, uint64(len(connection)))
	login = append(login, connection...)
	batch := binary.AppendUvarint(nil, uint64(len(login)))
	batch = append(batch, login...)
	var compressed bytes.Buffer
	writer, _ := flate.NewWriter(&compressed, flate.DefaultCompression)
	writer.Write(batch)
	writer.Close()
	description := describeEarlyBatch(append([]byte{0}, compressed.Bytes()...), true)
	if !strings.Contains(description, "chain=true，Certificate=false") || strings.Contains(description, privateJWT) {
		t.Fatalf("unsafe or missing diagnosis: %s", description)
	}
}

func TestEarlyServerFailureDiagnosis(t *testing.T) {
	batch := []byte{5, 2, 0, 0, 0, 1}
	description := describeEarlyBatch(append([]byte{0xff}, batch...), true)
	if !strings.Contains(description, "PlayStatus=1") {
		t.Fatal(description)
	}
	for _, malformed := range [][]byte{{}, {0}, {0xff, 255}, {0xff, 100, 1}, {0, 0, 1}} {
		describeEarlyBatch(malformed, true)
	}
}
