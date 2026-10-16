package netease

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestBuiltinClassicProfileHasNoKeysOrAccountData(t *testing.T) {
	p, err := BuiltinResourceProfile()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.ItemIDs) != 1 || p.ItemIDs[0] != ClassicPreloadComponentID {
		t.Fatal("unexpected preload component")
	}
	_, n := binary.Uvarint(p.Info)
	var info packet.ResourcePacksInfo
	info.Marshal(protocol.NewReader(bytes.NewBuffer(p.Info[n:]), 0, true))
	if len(info.TexturePacks) != 2 || info.TexturePacks[0].UUID.String() != "050ac25f-3124-4acb-ad16-64b779f8255f" || info.TexturePacks[0].Version != "1.0.0" {
		t.Fatal("classic/native pack identity lost")
	}
	for _, pack := range info.TexturePacks {
		if pack.ContentKey != "" || pack.DownloadURL != "" || pack.HasScripts {
			t.Fatal("embedded profile contains keys, remote URLs or script assets")
		}
	}
	p.ItemIDs[0] = 1
	p.Info[0] = 0
	next, err := BuiltinResourceProfile()
	if err != nil || next.ItemIDs[0] != ClassicPreloadComponentID || next.Info[0] != 6 {
		t.Fatal("caller mutated shared embedded profile", err)
	}
}
