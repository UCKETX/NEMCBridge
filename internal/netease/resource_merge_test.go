package netease

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestNativeComponentsAndBackendTexturesKeepIndependentIdentities(t *testing.T) {
	profile := testProfile(t)
	backendID := uuid.New()
	info := &packet.ResourcePacksInfo{TexturePackRequired: true, TexturePacks: []protocol.TexturePackInfo{{UUID: backendID, Version: "2.0.1", Size: 123, ContentKey: "own-fixture-key"}}}
	stack := &packet.ResourcePackStack{TexturePackRequired: true, TexturePacks: []protocol.StackResourcePack{{UUID: backendID.String(), Version: "2.0.1"}}, BaseGameVersion: "1.21.124", Experiments: []protocol.ExperimentData{{Name: "fixture", Enabled: true}}}
	// BDS can repeat the same world texture through addon dependencies.
	info.TexturePacks = append(info.TexturePacks, info.TexturePacks[0])
	stack.TexturePacks = append(stack.TexturePacks, stack.TexturePacks[0])
	for _, p := range []packet.Packet{info, stack} {
		var body bytes.Buffer
		p.Marshal(protocol.NewWriter(&body, 0))
		header := uint64(p.ID()) | 1<<10
		wire := append(binary.AppendUvarint(nil, header), body.Bytes()...)
		var merged []byte
		var err error
		if p.ID() == packet.IDResourcePacksInfo {
			merged, err = mergeResourceInfo(wire, profile)
		} else {
			merged, err = mergeResourceStack(wire, profile)
		}
		if err != nil {
			t.Fatal(err)
		}
		got, n := binary.Uvarint(merged)
		if got != header {
			t.Fatal("subclient header changed")
		}
		buffer := bytes.NewBuffer(merged[n:])
		if p.ID() == packet.IDResourcePacksInfo {
			var result packet.ResourcePacksInfo
			result.Marshal(protocol.NewReader(buffer, 0, true))
			if len(result.TexturePacks) != 2 || result.TexturePacks[0].ContentKey != "fixture-only-key" || result.TexturePacks[0].ContentIdentity != "original-native-identity" || result.TexturePacks[1].UUID != backendID || result.TexturePacks[1].Version != "2.0.1" || result.TexturePacks[1].ContentKey != "own-fixture-key" || result.TexturePacks[1].ContentIdentity != backendID.String() || !result.TexturePackRequired || buffer.Len() != 0 {
				t.Fatal("component and backend identities/keys mixed or lost")
			}
		} else {
			var result, native packet.ResourcePackStack
			result.Marshal(protocol.NewReader(buffer, 0, true))
			_, m := binary.Uvarint(profile.Stack)
			original := bytes.NewBuffer(profile.Stack[m:])
			native.Marshal(protocol.NewReader(original, 0, true))
			if len(result.TexturePacks) != 2 || result.TexturePacks[1].UUID != backendID.String() || result.BaseGameVersion != "*" || len(result.Experiments) != 1 || !bytes.Equal(buffer.Bytes(), original.Bytes()) {
				t.Fatal("native extension/order or backend experiments lost")
			}
		}
	}
}

func TestMergedTexturesRejectComponentUUIDCollision(t *testing.T) {
	profile := testProfile(t)
	_, n := binary.Uvarint(profile.Info)
	var info packet.ResourcePacksInfo
	info.Marshal(protocol.NewReader(bytes.NewBuffer(profile.Info[n:]), 0, true))
	info.TexturePacks[0].Version = "9.9.9"
	var b bytes.Buffer
	info.Marshal(protocol.NewWriter(&b, 0))
	wire := append([]byte{byte(packet.IDResourcePacksInfo)}, b.Bytes()...)
	if _, err := mergeResourceInfo(wire, profile); err == nil {
		t.Fatal("backend must not replace a native component under its UUID")
	}
}
