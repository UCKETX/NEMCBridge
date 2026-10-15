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
