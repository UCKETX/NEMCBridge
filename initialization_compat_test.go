package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"github.com/google/uuid"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestCompressedInitializationPreservesCompleteBatch(t *testing.T) {
	// A large unmodified neighbor must survive recompression byte for byte.
	neighbor := append([]byte{76}, bytes.Repeat([]byte("commands"), 20000)...)
	var ability bytes.Buffer
	(&packet.UpdateAbilities{AbilityData: protocol.AbilityData{EntityUniqueID: 7, PlayerPermissions: 1, Layers: []protocol.AbilityLayer{{Type: 1, Abilities: 3, Values: 3, WalkSpeed: .1}}}}).Marshal(protocol.NewWriter(&ability, 0))
	pk := binary.AppendUvarint(nil, uint64(packet.IDUpdateAbilities))
	pk = append(pk, ability.Bytes()...)
	var compressed bytes.Buffer
	compressed.WriteByte(0)
	w, _ := flate.NewWriter(&compressed, flate.DefaultCompression)
	w.Write(joinGameBatch([][]byte{neighbor, pk}))
	w.Close()
	result, err := (&bedrockBridge{}).patchInitialization860(compressed.Bytes())
	if err != nil || result[0] != 0 {
		t.Fatal("initialization did not retain DEFLATE", err)
	}
	decoded, err := decodeGameCompression(result)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := splitGameBatch(decoded)
	if err != nil || len(packets) != 2 || !bytes.Equal(packets[0], neighbor) {
		t.Fatal("large adjacent packet damaged", err)
	}
	_, n := binary.Uvarint(packets[1])
	buffer := bytes.NewBuffer(packets[1][n:])
	var parsed packet.UpdateAbilities
	parsed.Marshal(protocol.NewReader(buffer, 0, true))
	if parsed.AbilityData.EntityUniqueID != 7 || len(parsed.AbilityData.Layers) != 1 || buffer.Len() != 12 {
		t.Fatal("ability data shifted")
	}
	r := protocol.NewReader(buffer, 0, true)
	var extra32 uint32
	var extra64 int64
	r.Uint32(&extra32)
	r.Int64(&extra64)
	if buffer.Len() != 0 || extra32 != 0 || extra64 != 0 {
		t.Fatal("NetEase ability fields incorrect")
	}
}

func TestInventoryOptionsNetEaseExtension(t *testing.T) {
	p := packet.SetPlayerInventoryOptions{LeftInventoryTab: 4, RightInventoryTab: 2, Filtering: true, InventoryLayout: 3, CraftingLayout: 2}
	var body bytes.Buffer
	p.Marshal(protocol.NewWriter(&body, 0))
	wire := binary.AppendUvarint(nil, uint64(p.ID()))
	wire = append(wire, body.Bytes()...)
	result, err := (&bedrockBridge{}).patchInitialization860(append([]byte{0xff}, joinGameBatch([][]byte{wire})...))
	if err != nil {
		t.Fatal(err)
	}
	packets, _ := splitGameBatch(result[1:])
	_, n := binary.Uvarint(packets[0])
	buffer := bytes.NewBuffer(packets[0][n:])
	r := protocol.NewReader(buffer, 0, true)
	var decoded packet.SetPlayerInventoryOptions
	decoded.Marshal(r)
	var extra int32
	r.Varint32(&extra)
	if buffer.Len() != 0 || extra != 0 || decoded != p {
		t.Fatal("inventory settings fields shifted")
	}
	// A multi-byte signed extension must be removed as a field, not one byte.
	clientPacket := append(append([]byte(nil), wire...), binary.AppendVarint(nil, -300)...)
	reversed, err := patchClientInitialization860(append([]byte{0xff}, joinGameBatch([][]byte{{69, 16, 8}, clientPacket})...))
	if err != nil {
		t.Fatal(err)
	}
	reversedPackets, _ := splitGameBatch(reversed[1:])
	if len(reversedPackets) != 2 || !bytes.Equal(reversedPackets[0], []byte{69, 16, 8}) || !bytes.Equal(reversedPackets[1], wire) {
		t.Fatal("NetEase inventory reverse conversion changed common fields")
	}
	for _, broken := range [][]byte{wire[:len(wire)-1], append(append([]byte(nil), wire...), 0x80), append(append([]byte(nil), wire...), 0, 1)} {
		if _, err := patchClientInitialization860(append([]byte{0xff}, joinGameBatch([][]byte{broken})...)); err == nil {
			t.Fatal("invalid inventory extension accepted")
		}
	}
}

func TestNetEaseAbilityLayerCountUsesVaruint(t *testing.T) {
	for _, count := range []int{1, 127, 128, 255} {
		p := packet.UpdateAbilities{AbilityData: protocol.AbilityData{EntityUniqueID: 8, Layers: make([]protocol.AbilityLayer, count)}}
		var body bytes.Buffer
		p.Marshal(protocol.NewWriter(&body, 0))
		pk := binary.AppendUvarint(nil, uint64(p.ID()))
		pk = append(pk, body.Bytes()...)
		converted, err := patchAbilityPacket860(pk)
		if err != nil {
			t.Fatal(err)
		}
		_, n := binary.Uvarint(converted)
		buffer := bytes.NewBuffer(converted[n:])
		r := protocol.NewReader(buffer, 0, true)
		var id int64
		var permission1, permission2 byte
		var layers uint32
		r.Int64(&id)
		r.Uint8(&permission1)
		r.Uint8(&permission2)
		r.Varuint32(&layers)
		if id != 8 || int(layers) != count {
			t.Fatal("NetEase ability header shifted")
		}
		var entries []protocol.AbilityLayer
		protocol.SliceOfLen(r, layers, &entries)
		if buffer.Len() != 12 || !bytes.Equal(buffer.Bytes(), make([]byte, 12)) {
			t.Fatal("ability count changed layer boundaries")
		}
	}
}

// Independently walk the NetEase biome layout (fixed fields, extra fields,
// optional tags, no client terrain generation, dictionary) to catch shifted
// entry boundaries. This reader does not call the conversion implementation.
func verifyNetEaseBiomePayload(t *testing.T, payload []byte) {
	t.Helper()
	b := bytes.NewBuffer(payload)
	r := protocol.NewReader(b, 0, true)
	var count uint32
	r.Varuint32(&count)
	if count == 0 {
		t.Fatal("biomes unexpectedly empty")
	}
	for range count {
		if len(b.Next(29)) != 29 || !bytes.Equal(b.Next(5), make([]byte, 5)) {
			t.Fatal("NetEase biome extra fields not at expected wire offset")
		}
		var hasTags bool
		r.Bool(&hasTags)
		if hasTags {
			var tags uint32
			r.Varuint32(&tags)
			if len(b.Next(int(tags)*2)) != int(tags)*2 {
				t.Fatal("biome tag indices truncated")
			}
		}
		var generation bool
		r.Bool(&generation)
		if generation {
			t.Fatal("client generation still advertised")
		}
	}
	r.Varuint32(&count)
	for range count {
		var s string
		r.String(&s)
	}
	if b.Len() != 0 {
		t.Fatal("NetEase biome dictionary is misaligned")
	}
}

func TestPlayerListPreservesSkinsAndAppendsNetEaseGroups(t *testing.T) {
	list := packet.PlayerList{Entries: []protocol.PlayerListEntry{
		{UUID: uuid.New(), EntityUniqueID: 1, Username: "Keeper1", Host: true,
			Skin: protocol.Skin{SkinID: "skin1", SkinImageWidth: 64, SkinImageHeight: 64, SkinData: make([]byte, 64*64*4), Trusted: true}},
		{UUID: uuid.New(), EntityUniqueID: 2, Username: "Keeper2", Teacher: true,
			Skin: protocol.Skin{SkinID: "skin2", SkinImageWidth: 64, SkinImageHeight: 64, SkinData: make([]byte, 64*64*4)}},
	}}
	var payload bytes.Buffer
	list.Marshal(protocol.NewWriter(&payload, 0))
	wire := append([]byte{63}, payload.Bytes()...)
	converted, count, err := patchPlayerList860(wire)
	if err != nil || count != 2 {
		t.Fatal("player list conversion failed", err)
	}
	if !bytes.Equal(converted[:len(wire)-2], wire[:len(wire)-2]) || !bytes.Equal(converted[len(wire)-2:len(wire)], []byte{1, 1}) || !bytes.Equal(converted[len(wire):], make([]byte, 16)) {
		t.Fatal("player skin/identity bytes changed or extension group widths incorrect")
	}
	buffer := bytes.NewBuffer(converted[1:])
	var decoded packet.PlayerList
	decoded.Marshal(protocol.NewReader(buffer, 0, true))
	if buffer.Len() != 16 || !decoded.Entries[0].Host || !decoded.Entries[0].Skin.Trusted || !decoded.Entries[1].Skin.Trusted || !decoded.Entries[1].Teacher {
		t.Fatal("independent standard reader lost player metadata")
	}
	remove := []byte{63, 1, 0}
	unchanged, count, err := patchPlayerList860(remove)
	if err != nil || count != 0 || !bytes.Equal(unchanged, remove) {
		t.Fatal("removal acquired invalid extensions")
	}
}

func TestBiomeConversionWithGenerationAndDictionary(t *testing.T) {
	p := packet.BiomeDefinitionList{
		BiomeDefinitions: []protocol.BiomeDefinition{
			{NameIndex: 0, BiomeID: 1, Temperature: 0.8, Tags: protocol.Option([]uint16{2, 3}), ChunkGeneration: protocol.Option(protocol.BiomeChunkGeneration{Climate: protocol.Option(protocol.BiomeClimate{Temperature: 0.8})})},
			{NameIndex: 1, BiomeID: 2, Temperature: 2},
		},
		StringList: []string{"plains", "desert", "overworld", "animal"},
	}
	var encoded bytes.Buffer
	p.Marshal(protocol.NewWriter(&encoded, 0))
	wire := append([]byte{122}, encoded.Bytes()...)
	result, err := patchBiomeDefinitions860(wire)
	if err != nil {
		t.Fatal(err)
	}
	verifyNetEaseBiomePayload(t, result[1:])
	count, n := binary.Uvarint(result[1:])
	if count != 2 || !bytes.Equal(result[1+n:1+n+29], wire[2:31]) {
		t.Fatal("biome fixed fields changed")
	}
	if _, err := patchBiomeDefinitions860(wire[:len(wire)-1]); err == nil {
		t.Fatal("truncated biome dictionary accepted")
	}
}

func TestResourcePackStackNetEaseEmptyArrays(t *testing.T) {
	bridge := &bedrockBridge{}
	p := []byte{7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	result, err := bridge.patchInitialization860(append([]byte{0xff}, joinGameBatch([][]byte{p})...))
	if err != nil {
		t.Fatal(err)
	}
	packets, _ := splitGameBatch(result[1:])
	if !bytes.Equal(packets[0], append(p, 0, 0)) {
		t.Fatal("resource pack extension arrays missing")
	}
}
