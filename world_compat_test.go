package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestWorldCompatibilityPreservesBlockHashMode(t *testing.T) {
	pk := append([]byte{11}, bytes.Repeat([]byte{77}, 100)...)
	tail := make([]byte, 28)
	binary.LittleEndian.PutUint64(tail, 123456789)
	copy(tail[8:24], bytes.Repeat([]byte{42}, 16))
	copy(tail[24:], []byte{1, 1, 0, 1})
	pk = append(pk, tail...)
	original := append([]byte{0xff}, joinGameBatch([][]byte{pk, {2, 0, 0, 0, 0}})...)
	patched, changed, err := patchWorldSettings860(original, nil)
	if err != nil || !changed {
		t.Fatal(err)
	}
	packets, err := splitGameBatch(patched[1:])
	if err != nil {
		t.Fatal(err)
	}
	result := packets[0]
	// This fixture was independently recovered from the original Eulogist
	// 4.8.0 NetEase Marshal using bounded offline instruction emulation.
	fixture, err := os.ReadFile("testdata/netease-startgame-extensions-zero.bin")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(pk)+len(fixture) || !bytes.Equal(result[len(pk):], fixture) {
		t.Fatal("NetEase world extension missing or malformed")
	}
	result = result[:len(pk)]
	if !bytes.Equal(result[:len(result)-28], pk[:len(pk)-28]) || !bytes.Equal(result[len(result)-20:len(result)-4], tail[8:24]) {
		t.Fatal("non-target world fields changed")
	}
	newTail := result[len(result)-28:]
	if binary.LittleEndian.Uint64(newTail) != 0 || !bytes.Equal(newTail[24:], []byte{0, 1, 0, 1}) {
		t.Fatal("compatibility flags incorrect")
	}
	if !bytes.Equal(packets[1], []byte{2, 0, 0, 0, 0}) {
		t.Fatal("adjacent packet changed")
	}
	tail[25] = 0
	bad := append(append([]byte{11}, bytes.Repeat([]byte{77}, 100)...), tail...)
	if _, _, err = patchWorldSettings860(append([]byte{0xff}, joinGameBatch([][]byte{bad})...), nil); err == nil {
		t.Fatal("sequential block IDs incorrectly accepted")
	}
}
