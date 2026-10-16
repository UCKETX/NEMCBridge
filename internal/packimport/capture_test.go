package packimport

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	nepacket "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol/packet"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"nemcbridge/internal/gamebatch"
)

type captureReplay struct{ incoming, written [][]byte }

func (s *captureReplay) ReadPacket() ([]byte, error) {
	if len(s.incoming) == 0 {
		return nil, io.EOF
	}
	p := s.incoming[0]
	s.incoming = s.incoming[1:]
	return p, nil
}
func (s *captureReplay) Write(p []byte) (int, error) {
	s.written = append(s.written, append([]byte(nil), p...))
	return len(p), nil
}
func (*captureReplay) Close() error { return nil }

func TestCaptureNativeMetadataLeavesBeforeWorldAndPreservesPrivateKeys(t *testing.T) {
	encode := func(p packet.Packet) []byte {
		var b bytes.Buffer
		p.Marshal(protocol.NewWriter(&b, 0))
		return append(binary.AppendUvarint(nil, uint64(p.ID())), b.Bytes()...)
	}
	id := uuid.New()
	info := encode(&packet.ResourcePacksInfo{TexturePacks: []protocol.TexturePackInfo{{UUID: id, Version: "1.0.0", Size: 100, ContentKey: "fixture-key-only"}}})
	stack := append(encode(&packet.ResourcePackStack{TexturePacks: []protocol.StackResourcePack{{UUID: id.String(), Version: "1.0.0"}}, BaseGameVersion: "*"}), 0, 0)
	compress := nepacket.NeteaseCompression()
	compressed := func(packets ...[]byte) []byte {
		b, e := compress.Compress(gamebatch.Join(packets))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	s := &captureReplay{incoming: [][]byte{
		gamebatch.Join([][]byte{encode(&packet.NetworkSettings{CompressionThreshold: 1, CompressionAlgorithm: 2})}),
		compressed(encode(&packet.PlayStatus{Status: 0}), info),
		compressed(stack),
		[]byte("world initialization must remain unread"),
	}}
	p, err := captureStream(s, bunker.TanLobbyLoginResponse{UserUniqueID: 42, UserPlayerName: "ImportTest", RoomModDisplayName: []string{"4690087423136283144"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.incoming) != 1 || !bytes.Equal(p.Info, info) || !bytes.Equal(p.Stack, stack) || p.ItemIDs[0] != 4690087423136283144 {
		t.Fatal("resource metadata changed or collector entered world")
	}
	decompress := nepacket.NeteaseCompression()
	var ids []uint64
	for _, wire := range s.written[1:] {
		plain, e := decompress.Decompress(wire, 2<<20)
		if e != nil {
			t.Fatal(e)
		}
		packets, e := gamebatch.Split(plain)
		if e != nil {
			t.Fatal(e)
		}
		for _, pk := range packets {
			id, _ := binary.Uvarint(pk)
			ids = append(ids, id&1023)
			if id&1023 == 8 && !bytes.Equal(pk, []byte{8, 3, 0, 0}) {
				t.Fatal("collector must only acknowledge metadata availability")
			}
		}
	}
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 129 || ids[2] != 8 {
		t.Fatal("collector requested game content", ids)
	}
}
