package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	bdsraknet "github.com/sandertv/go-raknet"
	stdprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	stdpacket "github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestNetEaseSkipsServerResourcePacks(t *testing.T) {
	pack := uuid.New()
	info := stdpacket.ResourcePacksInfo{TexturePackRequired: true, HasScripts: true, WorldTemplateVersion: "1.0.0", TexturePacks: []stdprotocol.TexturePackInfo{{UUID: pack, Version: "1.0.0", Size: 603}}}
	stack := stdpacket.ResourcePackStack{TexturePackRequired: true, TexturePacks: []stdprotocol.StackResourcePack{{UUID: pack.String(), Version: "1.0.0"}}, BehaviourPacks: []stdprotocol.StackResourcePack{{UUID: uuid.NewString(), Version: "1.0.0"}}, BaseGameVersion: "1.21.124", Experiments: []stdprotocol.ExperimentData{{Name: "test", Enabled: true}}}
	encode := func(p stdpacket.Packet) []byte {
		var body bytes.Buffer
		p.Marshal(stdprotocol.NewWriter(&body, 0))
		return append(binary.AppendUvarint(nil, uint64(p.ID())), body.Bytes()...)
	}
	batch := append([]byte{0xff}, joinGameBatch([][]byte{encode(&info), encode(&stack)})...)
	converted, err := (&bedrockBridge{}).patchInitialization860(batch)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := splitGameBatch(converted[1:])
	if err != nil || len(packets) != 2 {
		t.Fatal(err)
	}
	buffer := bytes.NewBuffer(packets[0][1:])
	var gotInfo stdpacket.ResourcePacksInfo
	gotInfo.Marshal(stdprotocol.NewReader(buffer, 0, true))
	if buffer.Len() != 0 || len(gotInfo.TexturePacks) != 0 || gotInfo.TexturePackRequired || gotInfo.HasScripts || gotInfo.WorldTemplateVersion != "1.0.0" {
		t.Fatal("NetEase pack offer not emptied")
	}
	buffer = bytes.NewBuffer(packets[1][1:])
	var gotStack stdpacket.ResourcePackStack
	gotStack.Marshal(stdprotocol.NewReader(buffer, 0, true))
	if !bytes.Equal(buffer.Bytes(), []byte{0, 0}) || len(gotStack.TexturePacks)+len(gotStack.BehaviourPacks) != 0 || gotStack.TexturePackRequired || gotStack.BaseGameVersion != "1.21.124" || len(gotStack.Experiments) != 1 || !gotStack.Experiments[0].Enabled {
		t.Fatal("NetEase stack not emptied or lost game version/experiments")
	}
}

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
	bridge, err := newBedrockBridge(server, func(s string) { t.Log(s) })
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.forgetSkin()
	var stream packetStream = bridge
	if webRTC {
		stream = localWebRTCFrontend(t, ctx, bridge)
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
		plain, err := decodeGameCompression(p[1:])
		if err != nil {
			t.Fatal(err)
		}
		packets, err := splitGameBatch(plain)
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
