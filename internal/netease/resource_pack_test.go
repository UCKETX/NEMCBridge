package netease

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
	stdprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	stdpacket "github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/gamebatch"
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
	batch := append([]byte{0xff}, gamebatch.Join([][]byte{encode(&info), encode(&stack)})...)
	converted, err := (&ServerConverter{}).Convert(batch)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := gamebatch.Split(converted[1:])
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

func TestExperimentalResourcePackMetadataAndContentPreserved(t *testing.T) {
	pack := uuid.New()
	info := &stdpacket.ResourcePacksInfo{TexturePackRequired: true, HasAddons: true, HasScripts: true, WorldTemplateVersion: "1.0.0", TexturePacks: []stdprotocol.TexturePackInfo{{UUID: pack, Version: "1.2.3", Size: 12345, ContentKey: "test-content-key", ContentIdentity: "identity", SubPackName: "subpack", DownloadURL: "https://example.invalid/test.mcpack"}}}
	stack := &stdpacket.ResourcePackStack{TexturePackRequired: true, TexturePacks: []stdprotocol.StackResourcePack{{UUID: pack.String(), Version: "1.2.3", SubPackName: "subpack"}}, BehaviourPacks: []stdprotocol.StackResourcePack{{UUID: uuid.NewString(), Version: "1.0.0"}}, BaseGameVersion: "*", Experiments: []stdprotocol.ExperimentData{{Name: "fixture", Enabled: true}}}
	data := &stdpacket.ResourcePackDataInfo{UUID: pack.String(), Size: 12345, DataChunkSize: 1024, ChunkCount: 13, Hash: bytes.Repeat([]byte{42}, 32), PackType: stdpacket.ResourcePackTypeResources}
	chunk := &stdpacket.ResourcePackChunkData{UUID: pack.String(), ChunkIndex: 12, DataOffset: 12288, Data: bytes.Repeat([]byte{0xa5}, 57)}
	var wire [][]byte
	for _, p := range []stdpacket.Packet{info, stack, data, chunk} {
		var body bytes.Buffer
		p.Marshal(stdprotocol.NewWriter(&body, 0))
		wire = append(wire, append(binary.AppendUvarint(nil, uint64(p.ID())|1<<10), body.Bytes()...))
	}
	for _, compression := range []byte{0xff, 0} {
		batch, err := gamebatch.Encode([]byte{compression}, gamebatch.Join(wire))
		if err != nil {
			t.Fatal(err)
		}
		converted, err := (&ServerConverter{ForwardResourcePacks: true}).Convert(batch)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := gamebatch.Decode(converted)
		if err != nil {
			t.Fatal(err)
		}
		packets, err := gamebatch.Split(plain)
		if err != nil || len(packets) != len(wire) {
			t.Fatal("pack batch changed", err)
		}
		for i := range wire {
			want := wire[i]
			if i == 1 {
				want = append(append([]byte(nil), want...), 0, 0)
			}
			if !bytes.Equal(packets[i], want) {
				t.Fatalf("pack packet %d mutated; metadata/chunk bytes must stay intact", i)
			}
		}
	}
}

func TestBackendTextureMetadataUsesNativeDefaults(t *testing.T) {
	pack := uuid.New()
	info := &stdpacket.ResourcePacksInfo{TexturePacks: []stdprotocol.TexturePackInfo{{UUID: pack, Version: "1.2.3", Size: 123}}}
	stack := &stdpacket.ResourcePackStack{TexturePacks: []stdprotocol.StackResourcePack{{UUID: pack.String(), Version: "1.2.3"}}, BaseGameVersion: "1.21.124"}
	for _, p := range []stdpacket.Packet{info, stack} {
		var body bytes.Buffer
		p.Marshal(stdprotocol.NewWriter(&body, 0))
		wire := append(binary.AppendUvarint(nil, uint64(p.ID())|2<<10), body.Bytes()...)
		var out []byte
		var err error
		if p.ID() == stdpacket.IDResourcePacksInfo {
			out, _, err = neteaseResourcePacksInfo(wire, true)
		} else {
			out, _, err = neteaseResourcePackStack(wire, true)
		}
		if err != nil {
			t.Fatal(err)
		}
		header, n := binary.Uvarint(out)
		if header != uint64(p.ID())|2<<10 {
			t.Fatal("subclient header changed")
		}
		buffer := bytes.NewBuffer(out[n:])
		if p.ID() == stdpacket.IDResourcePacksInfo {
			var got stdpacket.ResourcePacksInfo
			got.Marshal(stdprotocol.NewReader(buffer, 0, true))
			if got.WorldTemplateVersion != "0.0.0" || got.TexturePacks[0].ContentIdentity != pack.String() || got.TexturePacks[0].UUID != pack || got.TexturePacks[0].Version != "1.2.3" || got.TexturePacks[0].Size != 123 || got.TexturePacks[0].ContentKey != "" || buffer.Len() != 0 {
				t.Fatal("native defaults or genuine pack identity lost")
			}
		} else {
			var got stdpacket.ResourcePackStack
			got.Marshal(stdprotocol.NewReader(buffer, 0, true))
			if got.BaseGameVersion != "*" || got.TexturePacks[0].UUID != pack.String() || !bytes.Equal(buffer.Bytes(), []byte{0, 0}) {
				t.Fatal("native stack defaults invalid")
			}
		}
	}
}

func TestEmptyRequiredPackOfferCompatibility(t *testing.T) {
	info := &stdpacket.ResourcePacksInfo{TexturePackRequired: true}
	var body bytes.Buffer
	info.Marshal(stdprotocol.NewWriter(&body, 0))
	wire := append([]byte{byte(stdpacket.IDResourcePacksInfo)}, body.Bytes()...)
	converted, err := (&ServerConverter{}).Convert(append([]byte{0xff}, gamebatch.Join([][]byte{wire})...))
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := gamebatch.Decode(converted)
	packets, _ := gamebatch.Split(plain)
	var got stdpacket.ResourcePacksInfo
	got.Marshal(stdprotocol.NewReader(bytes.NewBuffer(packets[0][1:]), 0, true))
	if got.TexturePackRequired {
		t.Fatal("required empty pack offer was not rewritten")
	}
}
