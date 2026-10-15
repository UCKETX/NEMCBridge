package netease

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	neprotocol "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol"
	nepacket "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol/packet"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/gamebatch"
)

func TestNetEaseCommandRequestToBDS(t *testing.T) {
	for _, origin := range []uint32{protocol.CommandOriginPlayer, protocol.CommandOriginDevConsole, protocol.CommandOriginTest} {
		for _, unlimited := range []bool{false, true} {
			request := &nepacket.CommandRequest{CommandLine: "/say 中文测试 🌍", CommandOrigin: neprotocol.CommandOrigin{Origin: origin, UUID: uuid.New(), RequestID: "request-测试", PlayerUniqueID: -12345}, Internal: true, Version: 42, UnLimited: unlimited}
			var body bytes.Buffer
			request.Marshal(neprotocol.NewWriter(&body, 0))
			header := binary.AppendUvarint(nil, uint64(packet.IDCommandRequest)|2<<10|1<<12)
			wire := append(append([]byte(nil), header...), body.Bytes()...)
			before := append([]byte(nil), wire...)
			converted, err := patchCommandRequest860(wire)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(converted, wire[:len(wire)-1]) || !bytes.Equal(before, wire) {
				t.Fatal("changed common bytes, header or original packet")
			}
			buffer := bytes.NewBuffer(converted[len(header):])
			var standard packet.CommandRequest
			standard.Marshal(protocol.NewReader(buffer, 0, true))
			if buffer.Len() != 0 || standard.CommandLine != request.CommandLine || standard.CommandOrigin.Origin != origin || standard.CommandOrigin.UUID != request.CommandOrigin.UUID || standard.CommandOrigin.RequestID != request.CommandOrigin.RequestID || standard.Internal != request.Internal || standard.Version != request.Version {
				t.Fatal("command fields changed or unread bytes remain")
			}
			if origin != protocol.CommandOriginPlayer && standard.CommandOrigin.PlayerUniqueID != request.CommandOrigin.PlayerUniqueID {
				t.Fatal("conditional command origin field changed")
			}
			if again, err := patchCommandRequest860(converted); err != nil || !bytes.Equal(again, converted) {
				t.Fatal("standard command was not preserved", err)
			}
		}
	}
}

func TestCommandBatchPreservesNeighborsAndCompression(t *testing.T) {
	request := &packet.CommandRequest{CommandLine: "/help", CommandOrigin: protocol.CommandOrigin{UUID: uuid.New()}}
	var body bytes.Buffer
	request.Marshal(protocol.NewWriter(&body, 0))
	standard := append([]byte{byte(packet.IDCommandRequest)}, body.Bytes()...)
	for _, bad := range [][]byte{nil, {byte(packet.IDCommandRequest)}, standard[:len(standard)-1], append(append([]byte(nil), standard...), 2), append(append([]byte(nil), standard...), 0, 0)} {
		if _, err := patchCommandRequest860(bad); err == nil {
			t.Fatalf("accepted malformed command %x", bad)
		}
	}
	for _, format := range []byte{0xff, 0} {
		packets := [][]byte{{69, 16, 8}, append(append([]byte(nil), standard...), 1), {70, 16}}
		compressed, err := gamebatch.Encode([]byte{format}, gamebatch.Join(packets))
		if err != nil {
			t.Fatal(err)
		}
		converted, err := ConvertClient(compressed)
		if err != nil || converted[0] != format {
			t.Fatal("command conversion changed compression", err)
		}
		plain, err := gamebatch.Decode(converted)
		if err != nil {
			t.Fatal(err)
		}
		got, err := gamebatch.Split(plain)
		want := [][]byte{packets[0], standard, packets[2]}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("command conversion changed neighboring packets", err)
		}
	}
}
