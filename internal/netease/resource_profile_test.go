package netease

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"nemcbridge/internal/gamebatch"
)

func testProfile(t *testing.T) *ResourceProfile {
	t.Helper()
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	encode := func(p packet.Packet) []byte {
		var b bytes.Buffer
		p.Marshal(protocol.NewWriter(&b, 0))
		return append(binary.AppendUvarint(nil, uint64(p.ID())), b.Bytes()...)
	}
	info := encode(&packet.ResourcePacksInfo{TexturePacks: []protocol.TexturePackInfo{{UUID: id, Version: "1.0.0", Size: 42, ContentKey: "fixture-only-key", ContentIdentity: "original-native-identity"}}})
	stack := encode(&packet.ResourcePackStack{TexturePacks: []protocol.StackResourcePack{{UUID: id.String(), Version: "1.0.0"}}, BaseGameVersion: "*"})
	var extras bytes.Buffer
	a := []protocol.StackResourcePack{{UUID: uuid.NewString(), Version: "0.0.6"}}
	b := []protocol.StackResourcePack{{UUID: uuid.NewString(), Version: "0.0.2"}}
	w := protocol.NewWriter(&extras, 0)
	protocol.Slice(w, &a)
	protocol.Slice(w, &b)
	return &ResourceProfile{ProtocolID: 860, ItemIDs: []uint64{4690087423136283144}, Info: info, Stack: append(stack, extras.Bytes()...)}
}

func TestNativeProfilePreservesKeysListsAndSubclientHeaders(t *testing.T) {
	p := testProfile(t)
	path := filepath.Join(t.TempDir(), ResourceProfileFile)
	if err := SaveResourceProfile(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadResourceProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded.Info, p.Info) || !bytes.Equal(loaded.Stack, p.Stack) || loaded.ItemIDs[0] != p.ItemIDs[0] {
		t.Fatal("native profile changed in storage")
	}
	for _, compression := range []byte{0xff, 0} {
		var original [][]byte
		for _, id := range []uint64{6, 7} {
			original = append(original, binary.AppendUvarint(nil, id|1<<10))
		}
		wire, err := gamebatch.Encode([]byte{compression}, gamebatch.Join(original))
		if err != nil {
			t.Fatal(err)
		}
		wire, err = (&ServerConverter{ResourceProfile: loaded}).Convert(wire)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := gamebatch.Decode(wire)
		if err != nil {
			t.Fatal(err)
		}
		packets, err := gamebatch.Split(plain)
		if err != nil || len(packets) != 2 {
			t.Fatal("profile packets lost", err)
		}
		for i, captured := range [][]byte{p.Info, p.Stack} {
			header, n := binary.Uvarint(packets[i])
			_, m := binary.Uvarint(captured)
			if header != uint64(6+i)|1<<10 || !bytes.Equal(packets[i][n:], captured[m:]) {
				t.Fatal("native extensions, pack keys or subclient header changed")
			}
		}
	}
}

func TestNativeProfileRejectsMalformedOrMismatchedData(t *testing.T) {
	for _, mutate := range []func(*ResourceProfile){
		func(p *ResourceProfile) { p.ProtocolID = 999 },
		func(p *ResourceProfile) { p.ItemIDs = nil },
		func(p *ResourceProfile) { p.ItemIDs = append(p.ItemIDs, p.ItemIDs[0]) },
		func(p *ResourceProfile) { p.Info = p.Info[:5] },
		func(p *ResourceProfile) { p.Stack = append(p.Stack, 1) },
		func(p *ResourceProfile) { p.Stack[5] ^= 1 },
	} {
		p := testProfile(t)
		mutate(p)
		if p.Validate() == nil {
			t.Fatal("invalid profile accepted")
		}
	}
}
