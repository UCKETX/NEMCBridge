package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestTerrainBatchAboveTwoMiBPreservesChatAndCompression(t *testing.T) {
	terrain := append([]byte{58}, bytes.Repeat([]byte{0x35, 0x4c, 0x99, 0x82}, 1<<20)...)
	chat := textTestWire(&packet.Text{TextType: packet.TextTypeChat, Message: "大批次中的聊天 🌍"})
	batch := joinGameBatch([][]byte{terrain, chat})
	compressed, err := encodeConvertedGameBatch([]byte{0}, batch)
	if err != nil {
		t.Fatal(err)
	}
	converted, err := (&bedrockBridge{}).patchInitialization860(compressed)
	if err != nil || converted[0] != 0 {
		t.Fatal("large terrain batch rejected or uncompressed", err)
	}
	decoded, err := decodeGameCompression(converted)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := splitGameBatch(decoded)
	if err != nil || len(packets) != 2 || !bytes.Equal(packets[0], terrain) || !bytes.Equal(packets[1], append(append([]byte(nil), chat...), 0)) {
		t.Fatal("large batch boundaries or neighbors corrupted", err)
	}
}

func TestDecodedBatchSizeLimit(t *testing.T) {
	for _, size := range []int{maxDecodedGameBatch, maxDecodedGameBatch + 1} {
		plain := bytes.Repeat([]byte{7}, size)
		compressed, err := encodeConvertedGameBatch([]byte{0}, plain)
		if err != nil {
			t.Fatal(err)
		}
		for _, wire := range [][]byte{compressed, append([]byte{0xff}, plain...)} {
			decoded, err := decodeGameCompression(wire)
			if size == maxDecodedGameBatch {
				if err != nil || !bytes.Equal(decoded, plain) {
					t.Fatal("exact limit rejected", err)
				}
			} else if err == nil {
				t.Fatal("over-limit batch accepted")
			}
		}
	}
}

func TestWebRTCMoreThan255Fragments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	backend := newPackets()
	stream := localWebRTCFrontend(t, ctx, backend)
	// The old uint8 fragment counter split this into two different messages.
	large := append([]byte{0xfe}, bytes.Repeat([]byte{0x83, 0x22, 0x71}, 900000)...)
	for _, size := range []int{len(large), 12} {
		want := large[:size]
		if _, err := stream.Write(want); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-backend.out:
			if !bytes.Equal(got, want) {
				t.Fatalf("upstream message boundary changed: got=%d want=%d", len(got), len(want))
			}
		case <-ctx.Done():
			t.Fatal("upstream large transfer timed out")
		}
		backend.in <- want
		got, err := stream.ReadPacket()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("downstream message boundary changed: got=%d want=%d err=%v", len(got), len(want), err)
		}
	}
}
