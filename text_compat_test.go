package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	neprotocol "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol"
	nepacket "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol/packet"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func textTestWire(p *packet.Text) []byte {
	var body bytes.Buffer
	p.Marshal(protocol.NewWriter(&body, 0))
	return append(binary.AppendUvarint(nil, uint64(packet.IDText)|2<<10|1<<12), body.Bytes()...)
}

func TestTextAllTypesBothEditions(t *testing.T) {
	for kind := byte(0); kind <= packet.TextTypeObjectAnnouncement; kind++ {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			original := textTestWire(&packet.Text{TextType: kind, NeedsTranslation: true, SourceName: "中文玩家", Message: "文字互通 🌍", Parameters: []string{"参数甲", "😀"}, XUID: "123456789", PlatformChatID: "platform", FilteredMessage: "过滤"})
			converted, err := patchText860(original, true)
			if err != nil {
				t.Fatal(err)
			}
			_, n := binary.Uvarint(converted)
			buffer := bytes.NewBuffer(converted[n:])
			var independent nepacket.Text
			independent.Marshal(neprotocol.NewReader(buffer, 0, true))
			if buffer.Len() != 0 || independent.Message != "文字互通 🌍" || independent.XUID != "123456789" || independent.FilteredMessage != "过滤" {
				t.Fatal("NetEase reader rejected converted text")
			}
			back, err := patchText860(converted, false)
			if err != nil || !bytes.Equal(back, original) {
				t.Fatal("common fields/header changed", err)
			}
		})
	}
}

func TestTextNonemptyNetEaseExtensions(t *testing.T) {
	for _, kind := range []byte{packet.TextTypeChat, packet.TextTypePopup} {
		common := textTestWire(&packet.Text{TextType: kind, SourceName: "玩家", Message: "测试🌍", Parameters: []string{"参数"}})
		var extension bytes.Buffer
		writer := protocol.NewWriter(&extension, 0)
		if kind == packet.TextTypeChat {
			count := uint32(128)
			writer.Varuint32(&count)
			for i := range count {
				value := fmt.Sprintf("扩展 %d 🌍", i)
				writer.String(&value)
			}
		} else {
			value := "非空弹窗扩展"
			writer.String(&value)
		}
		converted, err := patchText860(append(append([]byte(nil), common...), extension.Bytes()...), false)
		if err != nil || !bytes.Equal(converted, common) {
			t.Fatal("extension removal corrupted text", err)
		}
	}
}

func TestTextRejectMalformed(t *testing.T) {
	common := textTestWire(&packet.Text{TextType: packet.TextTypeChat, Message: "hello"})
	for _, bad := range [][]byte{nil, {9, 12, 0}, common[:len(common)-1], append(append([]byte(nil), common...), 1), append(append([]byte(nil), common...), 0, 0), append(append([]byte(nil), common...), 0x81, 0x08)} {
		if _, err := patchText860(bad, false); err == nil {
			t.Fatalf("accepted malformed text %x", bad)
		}
	}
	if _, err := patchText860(append(common, 0), true); err == nil {
		t.Fatal("backend unknown tail accepted")
	}
}

func TestTextCompressedBatchPreservesNeighbors(t *testing.T) {
	chat := textTestWire(&packet.Text{TextType: packet.TextTypeChat, SourceName: "玩家", Message: "连续聊天 🌍"})
	neighbors := [][]byte{{2, 0, 0, 0, 0}, chat, {70, 16}}
	compressed, err := encodeConvertedGameBatch([]byte{0}, joinGameBatch(neighbors))
	if err != nil {
		t.Fatal(err)
	}
	bridge := &bedrockBridge{}
	netease, err := bridge.patchInitialization860(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if netease[0] != 0 {
		t.Fatal("DEFLATE negotiation changed")
	}
	plain, err := decodeGameCompression(netease)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := splitGameBatch(plain)
	if err != nil || len(packets) != 3 || !bytes.Equal(packets[0], neighbors[0]) || !bytes.Equal(packets[2], neighbors[2]) || !bytes.Equal(packets[1], append(append([]byte(nil), chat...), 0)) {
		t.Fatal("batch neighbor changed", err)
	}
	standard, err := patchClientInitialization860(netease)
	if err != nil {
		t.Fatal(err)
	}
	plain, err = decodeGameCompression(standard)
	if err != nil || !bytes.Equal(plain, joinGameBatch(neighbors)) {
		t.Fatal("reverse batch conversion changed common packets", err)
	}
}
