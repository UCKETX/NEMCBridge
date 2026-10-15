package integration

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"

	bdsraknet "github.com/sandertv/go-raknet"
	stdprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	stdpacket "github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/bridge"
	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/relay"
)

func TestBDSResourcePackSkippedForNetEase(t *testing.T) {
	testBDSResourcePackSkipped(t, false)
}

func TestWebRTCResourcePackSkippedForNetEase(t *testing.T) {
	testBDSResourcePackSkipped(t, true)
}

// Requires a BDS whose world requires at least one resource pack, such as
// runtime/bedrock-rp-test (LOBBYKEEPER_BDS_RP_TEST=127.0.0.1:19142). Reproduces
// the NetEase client: it answers "have all packs" without downloading.
func testBDSResourcePackSkipped(t *testing.T, webRTC bool) {
	addr := os.Getenv("LOBBYKEEPER_BDS_RP_TEST")
	if addr == "" {
		t.Skip("requires local BDS with a required world resource pack")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	server, err := bdsraknet.DialContext(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	context.AfterFunc(ctx, func() { server.Close() })
	relayBridge, err := bridge.New(server, func(s string) { t.Log(s) })
	if err != nil {
		t.Fatal(err)
	}
	defer relayBridge.ForgetSkin()
	var stream relay.Stream = relayBridge
	if webRTC {
		stream = localWebRTCFrontend(t, ctx, relayBridge)
	}
	if _, err = stream.Write([]byte{0xfe, 6, 0xc1, 1, 0, 0, 3, 0x5c}); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.ReadPacket(); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Write(makeOfflineTestLogin(t)); err != nil {
		t.Fatal(err)
	}
	offered, stacked := false, false
	for i := 0; i < 256; i++ {
		p, err := stream.ReadPacket()
		if err != nil {
			t.Fatal(err)
		}
		plain, err := gamebatch.Decode(p[1:])
		if err != nil {
			t.Fatal(err)
		}
		packets, err := gamebatch.Split(plain)
		if err != nil {
			t.Fatal(err)
		}
		for _, pk := range packets {
			id, n := binary.Uvarint(pk)
			buffer := bytes.NewBuffer(pk[n:])
			reader := stdprotocol.NewReader(buffer, 0, true)
			switch id & 0x3ff {
			case stdpacket.IDResourcePacksInfo:
				var info stdpacket.ResourcePacksInfo
				info.Marshal(reader)
				if buffer.Len() != 0 || len(info.TexturePacks) != 0 || info.TexturePackRequired {
					t.Fatal("NetEase player was still offered server packs")
				}
				offered = true
				writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseAllPacksDownloaded})
			case stdpacket.IDResourcePackStack:
				var stack stdpacket.ResourcePackStack
				stack.Marshal(reader)
				if !bytes.Equal(buffer.Bytes(), []byte{0, 0}) || len(stack.TexturePacks)+len(stack.BehaviourPacks) != 0 {
					t.Fatal("NetEase stack still references server packs")
				}
				stacked = true
				writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseCompleted})
			case stdpacket.IDStartGame:
				if !offered || !stacked {
					t.Fatal("world started without the pack negotiation")
				}
				t.Log("BDS accepted the NetEase player without pack download and sent StartGame")
				return
			}
		}
	}
	t.Fatal("pack negotiation did not reach StartGame")
}
