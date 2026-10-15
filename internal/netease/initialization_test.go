package netease

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/netease/neteasetest"
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
	w.Write(gamebatch.Join([][]byte{neighbor, pk}))
	w.Close()
	result, err := (&ServerConverter{}).Convert(compressed.Bytes())
	if err != nil || result[0] != 0 {
		t.Fatal("initialization did not retain DEFLATE", err)
	}
	decoded, err := gamebatch.Decode(result)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := gamebatch.Split(decoded)
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
	if buffer.Len() != 0 || extra32 != 0 || extra64 != -1 {
		t.Fatal("NetEase ability fields incorrect")
	}
}

func TestInventoryOptionsNetEaseExtension(t *testing.T) {
	p := packet.SetPlayerInventoryOptions{LeftInventoryTab: 4, RightInventoryTab: 2, Filtering: true, InventoryLayout: 3, CraftingLayout: 2}
	var body bytes.Buffer
	p.Marshal(protocol.NewWriter(&body, 0))
	wire := binary.AppendUvarint(nil, uint64(p.ID()))
	wire = append(wire, body.Bytes()...)
	result, err := (&ServerConverter{}).Convert(append([]byte{0xff}, gamebatch.Join([][]byte{wire})...))
	if err != nil {
		t.Fatal(err)
	}
	packets, _ := gamebatch.Split(result[1:])
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
	reversed, err := ConvertClient(append([]byte{0xff}, gamebatch.Join([][]byte{{69, 16, 8}, clientPacket})...))
	if err != nil {
		t.Fatal(err)
	}
	reversedPackets, _ := gamebatch.Split(reversed[1:])
	if len(reversedPackets) != 2 || !bytes.Equal(reversedPackets[0], []byte{69, 16, 8}) || !bytes.Equal(reversedPackets[1], wire) {
		t.Fatal("NetEase inventory reverse conversion changed common fields")
	}
	for _, broken := range [][]byte{wire[:len(wire)-1], append(append([]byte(nil), wire...), 0x80), append(append([]byte(nil), wire...), 0, 1)} {
		if _, err := ConvertClient(append([]byte{0xff}, gamebatch.Join([][]byte{broken})...)); err == nil {
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
		if buffer.Len() != 12 || !bytes.Equal(buffer.Bytes(), []byte{0, 0, 0, 0, 255, 255, 255, 255, 255, 255, 255, 255}) {
			t.Fatal("ability count changed layer boundaries")
		}
	}
}

func nativePermissionFixture(t *testing.T, name string) []byte {
	t.Helper()
	text, err := os.ReadFile("testdata/" + name + ".hex")
	if err != nil {
		t.Fatal(err)
	}
	wire, err := hex.DecodeString(strings.TrimSpace(string(text)))
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func TestAbilityMatchesNativeMemberCapture(t *testing.T) {
	want := nativePermissionFixture(t, "native-860-member-abilities")
	// One base layer has the same one-byte count in both protocols. Remove
	// only the proprietary trailer from the independently captured packet.
	standard := want[:len(want)-12]
	got, err := patchAbilityPacket860(standard)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("native member packet mismatch: %x, error %v", got, err)
	}
}

// A visitor must remain restricted, while members and operators retain exactly
// the backend's capabilities, including custom overrides in a second layer.
func TestNetEasePreservesBackendPermissions(t *testing.T) {
	for _, permission := range []byte{0, 1, 2, 3} {
		t.Run(fmt.Sprintf("permission_%d", permission), func(t *testing.T) {
			values := uint32(protocol.AbilityBuild | protocol.AbilityMine | protocol.AbilityDoorsAndSwitches)
			if permission == 0 {
				values = 0
			}
			want := protocol.AbilityData{EntityUniqueID: -34359738367, PlayerPermissions: permission, CommandPermissions: permission / 2, Layers: []protocol.AbilityLayer{
				{Type: protocol.AbilityLayerTypeBase, Abilities: 0xfffff, Values: values, FlySpeed: .05, VerticalFlySpeed: 1, WalkSpeed: .1},
				{Type: protocol.AbilityLayerTypeCommands, Abilities: protocol.AbilityFlying, Values: protocol.AbilityFlying, FlySpeed: .2, VerticalFlySpeed: .5, WalkSpeed: .3},
			}}
			var body bytes.Buffer
			(&packet.UpdateAbilities{AbilityData: want}).Marshal(protocol.NewWriter(&body, 0))
			wire := append(binary.AppendUvarint(nil, uint64(packet.IDUpdateAbilities)), body.Bytes()...)
			converted, err := (&ServerConverter{}).Convert(append([]byte{0xff}, gamebatch.Join([][]byte{wire})...))
			if err != nil {
				t.Fatal(err)
			}
			packets, err := gamebatch.Split(converted[1:])
			if err != nil || len(packets) != 1 {
				t.Fatal("ability batch invalid", err)
			}
			_, n := binary.Uvarint(packets[0])
			buffer := bytes.NewBuffer(packets[0][n:])
			r := protocol.NewReader(buffer, 0, true)
			var got protocol.AbilityData
			r.Int64(&got.EntityUniqueID)
			r.Uint8(&got.PlayerPermissions)
			r.Uint8(&got.CommandPermissions)
			var count uint32
			r.Varuint32(&count)
			protocol.SliceOfLen(r, count, &got.Layers)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("backend permissions changed: got %+v want %+v", got, want)
			}
			if buffer.Len() != 12 {
				t.Fatal("NetEase extension moved into ability data")
			}
		})
	}
}

func TestNetEaseSpawnCompletesBackendLoading(t *testing.T) {
	for _, compression := range []byte{0xff, 0} {
		t.Run(fmt.Sprintf("compression_%d", compression), func(t *testing.T) {
			// Retain sub-client routing and insert completion immediately after
			// the client's spawn confirmation, before any subsequent action.
			header := uint64(packet.IDSetLocalPlayerAsInitialised) | 1<<10 | 2<<12
			spawn := binary.AppendUvarint(nil, header)
			spawn = binary.AppendUvarint(spawn, 42000)
			neighbor := []byte{69, 16, 8}
			original, err := gamebatch.Encode([]byte{compression}, gamebatch.Join([][]byte{neighbor, spawn, neighbor}))
			if err != nil {
				t.Fatal(err)
			}
			converted, err := ConvertClient(original)
			if err != nil || converted[0] != compression {
				t.Fatal("spawn batch compression changed", err)
			}
			plain, err := gamebatch.Decode(converted)
			if err != nil {
				t.Fatal(err)
			}
			packets, err := gamebatch.Split(plain)
			if err != nil || len(packets) != 4 || !bytes.Equal(packets[0], neighbor) || !bytes.Equal(packets[1], spawn) || !bytes.Equal(packets[3], neighbor) {
				t.Fatal("spawn confirmation or adjacent packets changed", err)
			}
			ackHeader, n := binary.Uvarint(packets[2])
			if ackHeader&0x3ff != packet.IDServerBoundLoadingScreen || ackHeader&^uint64(0x3ff) != header&^uint64(0x3ff) {
				t.Fatal("loading completion packet routing incorrect")
			}
			body := bytes.NewBuffer(packets[2][n:])
			var ack packet.ServerBoundLoadingScreen
			ack.Marshal(protocol.NewReader(body, 0, true))
			if ack.Type != packet.LoadingScreenTypeEnd || ack.LoadingScreenID != (protocol.Optional[uint32]{}) || body.Len() != 0 {
				t.Fatal("loading completion contains an invalid event or dimension ID")
			}
		})
	}
	for _, body := range [][]byte{nil, {0x80}, {1, 0}} {
		wire := append(binary.AppendUvarint(nil, uint64(packet.IDSetLocalPlayerAsInitialised)), body...)
		if _, err := ConvertClient(append([]byte{0xff}, gamebatch.Join([][]byte{wire})...)); err == nil {
			t.Fatal("invalid spawn confirmation accepted")
		}
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
	if len(converted) != len(wire)+16 || !bytes.Equal(converted[len(wire)-2:len(wire)], []byte{1, 1}) || !bytes.Equal(converted[len(wire):], make([]byte, 16)) {
		t.Fatal("extension group widths incorrect")
	}
	buffer := bytes.NewBuffer(converted[1:])
	var decoded packet.PlayerList
	decoded.Marshal(protocol.NewReader(buffer, 0, true))
	if buffer.Len() != 16 || !decoded.Entries[0].Host || !decoded.Entries[1].Teacher {
		t.Fatal("independent standard reader lost player metadata")
	}
	for i, entry := range decoded.Entries {
		want := list.Entries[i]
		want.Skin.Trusted, want.Skin.PremiumSkin = true, true
		var got, expected bytes.Buffer
		entry.Marshal(protocol.NewWriter(&got, 0))
		want.Marshal(protocol.NewWriter(&expected, 0))
		if !bytes.Equal(got.Bytes(), expected.Bytes()) || entry.Skin.Trusted != want.Skin.Trusted {
			t.Fatal("foreign player changed beyond NetEase skin flags")
		}
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
	neteasetest.VerifyBiomePayload(t, result[1:])
	count, n := binary.Uvarint(result[1:])
	if count != 2 || !bytes.Equal(result[1+n:1+n+29], wire[2:31]) {
		t.Fatal("biome fixed fields changed")
	}
	if _, err := patchBiomeDefinitions860(wire[:len(wire)-1]); err == nil {
		t.Fatal("truncated biome dictionary accepted")
	}
}

func TestResourcePackStackNetEaseEmptyArrays(t *testing.T) {
	converter := &ServerConverter{}
	p := []byte{7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	result, err := converter.Convert(append([]byte{0xff}, gamebatch.Join([][]byte{p})...))
	if err != nil {
		t.Fatal(err)
	}
	packets, _ := gamebatch.Split(result[1:])
	if !bytes.Equal(packets[0], append(p, 0, 0)) {
		t.Fatal("resource pack extension arrays missing")
	}
}

func TestAddPlayerGainsNetEaseExtension(t *testing.T) {
	spawn := packet.AddPlayer{UUID: uuid.New(), Username: "Keeper", EntityRuntimeID: 7, DeviceID: "device", BuildPlatform: 7, EntityMetadata: map[uint32]any{}}
	var payload bytes.Buffer
	spawn.Marshal(protocol.NewWriter(&payload, 0))
	wire := append([]byte{12}, payload.Bytes()...)
	neighbor := []byte{69, 16, 8}
	batch := append([]byte{0xff}, gamebatch.Join([][]byte{wire, neighbor})...)
	converted, err := (&ServerConverter{}).Convert(batch)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := gamebatch.Split(converted[1:])
	if err != nil || len(packets) != 2 || !bytes.Equal(packets[1], neighbor) {
		t.Fatal("adjacent packet damaged", err)
	}
	buffer := bytes.NewBuffer(packets[0][1:])
	var decoded packet.AddPlayer
	decoded.Marshal(protocol.NewReader(buffer, 0, true))
	if decoded.UUID != spawn.UUID || decoded.DeviceID != "device" || decoded.BuildPlatform != 7 || !bytes.Equal(buffer.Bytes(), nativePermissionFixture(t, "native-860-add-player-tail")) {
		t.Fatal("NetEase AddPlayer extension missing or common fields changed")
	}
}

func TestTerrainBatchAboveTwoMiBPreservesChatAndCompression(t *testing.T) {
	terrain := append([]byte{58}, bytes.Repeat([]byte{0x35, 0x4c, 0x99, 0x82}, 1<<20)...)
	chat := textTestWire(&packet.Text{TextType: packet.TextTypeChat, Message: "大批次中的聊天 🌍"})
	batch := gamebatch.Join([][]byte{terrain, chat})
	compressed, err := gamebatch.Encode([]byte{0}, batch)
	if err != nil {
		t.Fatal(err)
	}
	converted, err := (&ServerConverter{}).Convert(compressed)
	if err != nil || converted[0] != 0 {
		t.Fatal("large terrain batch rejected or uncompressed", err)
	}
	decoded, err := gamebatch.Decode(converted)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := gamebatch.Split(decoded)
	if err != nil || len(packets) != 2 || !bytes.Equal(packets[0], terrain) || !bytes.Equal(packets[1], append(append([]byte(nil), chat...), 0)) {
		t.Fatal("large batch boundaries or neighbors corrupted", err)
	}
}
