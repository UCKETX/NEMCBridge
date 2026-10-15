package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func skinTestPixels(width int) []byte {
	pixels := make([]byte, width*width*4)
	for y := 0; y < width; y++ {
		for x := 0; x < width; x++ {
			i := (y*width + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = byte(x), byte(y), 90, 255
		}
	}
	return pixels
}

func TestForeignSkinUsesNetEaseFlags(t *testing.T) {
	for _, premium := range []bool{false, true} {
		geometry := []byte(`{"format_version":"1.12.0","minecraft:geometry":[{"description":{"identifier":"geometry.humanoid.custom","texture_width":64,"texture_height":64},"bones":[{"name":"head","pivot":[0,24,0]}]}]}`)
		skin := protocol.Skin{PremiumSkin: premium, SkinImageWidth: 64, SkinImageHeight: 64, SkinData: skinTestPixels(64), SkinGeometry: geometry, SkinResourcePatch: []byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)}
		level, err := neteaseRecipientSkin(uuid.New(), "foreign", &skin)
		if err != nil || level != 0 || !skin.PremiumSkin || skin.OverrideAppearance || !skin.Trusted || !strings.HasPrefix(skin.SkinID, "LobbyKeeper_") {
			t.Fatal("foreign skin not in NetEase shape", err)
		}
		before := skin
		neteaseRecipientSkin(uuid.New(), "foreign", &skin)
		if !reflect.DeepEqual(skin, before) {
			t.Fatal("NetEase skin changes on repeat conversion")
		}
	}
}

func netEaseLoginFixture(t *testing.T, id uuid.UUID) (map[string]any, map[string]json.RawMessage) {
	t.Helper()
	legacy := `{"format_version":"1.8.0","geometry.netease.fixture":{"texturewidth":256,"textureheight":256,"bones":[{"name":"head","pivot":[0,24,0]}]}}`
	values := map[string]any{
		"SkinId": "netease-item-skin", "PlayFabId": "", "SkinImageWidth": 256, "SkinImageHeight": 256,
		"SkinData":                      base64.StdEncoding.EncodeToString(skinTestPixels(256)),
		"SkinGeometryData":              base64.StdEncoding.EncodeToString([]byte(legacy)),
		"SkinResourcePatch":             base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.netease.fixture"}}`)),
		"SkinGeometryDataEngineVersion": base64.StdEncoding.EncodeToString([]byte("0.0.0")),
		"ArmSize":                       "wide", "PremiumSkin": true, "TrustedSkin": true, "OverrideSkin": false, "PersonaSkin": false, "GrowthLevel": 37, "CapeId": "",
	}
	claims := map[string]json.RawMessage{}
	for key, value := range values {
		claims[key], _ = json.Marshal(value)
	}
	return map[string]any{"identity": id.String(), "displayName": "NetEasePlayer", "XUID": "123456789", "netease_uid": float64(1261597877)}, claims
}

func TestNetEaseRecipientsGetOriginalLoginSkin(t *testing.T) {
	id := uuid.New()
	identity, claims := netEaseLoginFixture(t, id)
	parsed, record, ok := originalLoginSkin(identity, claims)
	if !ok || parsed != id || record.growthLevel != 37 {
		t.Fatal("original NetEase login skin not captured")
	}
	original := record.skin
	rememberNetEaseSkin(id, record)
	defer forgetNetEaseSkin(id)
	if _, err := normalizeLoginSkin(claims); err != nil {
		t.Fatal(err)
	}
	var backendID string
	json.Unmarshal(claims["SkinId"], &backendID)
	if !strings.HasPrefix(backendID, "LobbyKeeper_") {
		t.Fatal("backend login skin not normalized")
	}
	// What BDS relays: the downsampled LobbyKeeper skin with BDS flags.
	backend := protocol.Skin{SkinID: backendID, FullID: backendID + "cape", CapeID: "cape", SkinImageWidth: 128, SkinImageHeight: 128, SkinData: skinTestPixels(128), SkinGeometry: []byte(`{}`), SkinResourcePatch: []byte(`{}`), OverrideAppearance: true}
	list := packet.PlayerList{Entries: []protocol.PlayerListEntry{{UUID: id, Username: "NetEasePlayer", Skin: backend}}}
	var payload bytes.Buffer
	list.Marshal(protocol.NewWriter(&payload, 0))
	converted, count, err := patchPlayerList860(append([]byte{63}, payload.Bytes()...))
	if err != nil || count != 1 {
		t.Fatal(err)
	}
	if !bytes.Equal(converted[len(converted)-8:], []byte{0, 0, 0, 37, 0, 0, 0, 0}) {
		t.Fatal("growth level extension not filled")
	}
	buffer := bytes.NewBuffer(converted[1:])
	var decoded packet.PlayerList
	decoded.Marshal(protocol.NewReader(buffer, 0, true))
	skin := decoded.Entries[0].Skin
	if decoded.Entries[0].XUID != "123456789" {
		t.Fatal("NetEase recipient did not receive the player's XUID")
	}
	if lines := loginShapeSummary(identity, claims); len(lines) < 2 || strings.Contains(strings.Join(lines, ""), "123456789") {
		t.Fatal("login shape summary missing or leaked an identifier")
	}
	if skin.SkinID != original.SkinID || skin.FullID != original.SkinID+"cape" || skin.SkinImageWidth != 256 || !bytes.Equal(skin.SkinData, original.SkinData) || !bytes.Equal(skin.SkinGeometry, original.SkinGeometry) || !bytes.Equal(skin.SkinResourcePatch, original.SkinResourcePatch) || string(skin.GeometryDataEngineVersion) != "0.0.0" || !skin.PremiumSkin || !skin.Trusted || skin.OverrideAppearance || skin.CapeID != "cape" {
		t.Fatal("NetEase recipient did not receive the original skin")
	}
	// A NetEase in-game skin change is remembered before downsampling.
	changed := original
	changed.SkinID = "netease-item-skin-2"
	update := packet.PlayerSkin{UUID: id, Skin: changed}
	payload.Reset()
	update.Marshal(protocol.NewWriter(&payload, 0))
	upstream, err := patchPlayerSkin860(append([]byte{93}, payload.Bytes()...), false)
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := patchPlayerSkin860(upstream, true)
	if err != nil {
		t.Fatal(err)
	}
	buffer = bytes.NewBuffer(downstream[1:])
	var relayed packet.PlayerSkin
	relayed.Marshal(protocol.NewReader(buffer, 0, true))
	if relayed.Skin.SkinID != "netease-item-skin-2" || relayed.Skin.SkinImageWidth != 256 || !relayed.Skin.PremiumSkin {
		t.Fatal("NetEase skin change not restored for NetEase recipients")
	}
	forgetNetEaseSkin(id)
	if _, ok := lookupNetEaseSkin(id, "NetEasePlayer"); ok {
		t.Fatal("skin kept after the session ended")
	}
}

func TestPortableLegacySkinModelAndCacheIdentity(t *testing.T) {
	legacy := []byte(`{"format_version":"1.8.0","geometry.netease.fixture":{"texturewidth":64,"textureheight":64,"visible_bounds_width":2,"visible_bounds_height":3,"visible_bounds_offset":[0,1,0],"bones":[{"name":"head","pivot":[0,24,0],"rotation":[0,0,0],"cubes":[{"origin":[-4,24,-4],"size":[8,8,8],"uv":[32,0],"mirror":true}]}]}}`)
	skin := protocol.Skin{SkinID: "foreign-store", FullID: "foreign-full", PlayFabID: "foreign-playfab", PremiumSkin: true, SkinImageWidth: 128, SkinImageHeight: 128, SkinData: skinTestPixels(128), SkinGeometry: legacy, SkinResourcePatch: []byte(`{"geometry":{"default":"geometry.netease.fixture"},"other":"keep"}`)}
	changed, err := portableClassicSkin(&skin)
	if err != nil || !changed {
		t.Fatal(err)
	}
	if skin.PremiumSkin || skin.PlayFabID != "" || !skin.Trusted || !skin.OverrideAppearance || !strings.HasPrefix(skin.SkinID, "LobbyKeeper_") || skin.FullID != skin.SkinID || string(skin.GeometryDataEngineVersion) != "1.12.0" {
		t.Fatal("foreign catalog identity not converted")
	}
	var original, result, patch map[string]any
	json.Unmarshal(legacy, &original)
	json.Unmarshal(skin.SkinGeometry, &result)
	json.Unmarshal(skin.SkinResourcePatch, &patch)
	model := result["minecraft:geometry"].([]any)[0].(map[string]any)
	description := model["description"].(map[string]any)
	if result["format_version"] != "1.12.0" || description["identifier"] != "geometry.humanoid.custom" || description["texture_width"] != float64(64) || description["texture_height"] != float64(64) || !reflect.DeepEqual(model["bones"], original["geometry.netease.fixture"].(map[string]any)["bones"]) || patch["other"] != "keep" || patch["geometry"].(map[string]any)["default"] != description["identifier"] {
		t.Fatal("legacy geometry/UVs/resource binding corrupted")
	}
	before := skin
	portableClassicSkin(&skin)
	if !reflect.DeepEqual(before, skin) {
		t.Fatal("cache identity changes on repeat conversion")
	}
	skin.SkinData = append([]byte(nil), skin.SkinData...)
	skin.SkinData[0] ^= 255
	portableClassicSkin(&skin)
	if skin.SkinID == before.SkinID {
		t.Fatal("changed texture reuses stale skin cache identity")
	}
	persona := before
	persona.PersonaSkin = true
	old := persona
	changed, err = portableClassicSkin(&persona)
	if err != nil || changed || !reflect.DeepEqual(old, persona) {
		t.Fatal("persona store resources were damaged")
	}
}

func TestHighResolutionLoginSkinKeepsGeometryAndClaims(t *testing.T) {
	geometry := base64.StdEncoding.EncodeToString([]byte(`{"format_version":"1.12.0","minecraft:geometry":[{"description":{"identifier":"fixture","texture_width":256,"texture_height":256},"bones":[{"cubes":[{"uv":[128,64],"size":[8,8,8]}]}]}]}`))
	fields := map[string]any{"SkinImageWidth": 256, "SkinImageHeight": 256, "SkinData": base64.StdEncoding.EncodeToString(skinTestPixels(256)), "SkinGeometryData": geometry, "SkinId": "hd-fixture", "SkinResourcePatch": base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"fixture"}}`)), "UnknownNetEaseSkinField": map[string]any{"preserve": true}}
	claims := make(map[string]json.RawMessage)
	for k, v := range fields {
		claims[k], _ = json.Marshal(v)
	}
	original := make(map[string]json.RawMessage)
	for k, v := range claims {
		original[k] = append([]byte(nil), v...)
	}
	message, err := normalizeLoginSkin(claims)
	if err != nil || message == "" {
		t.Fatal(message, err)
	}
	var w, h int
	var encoded string
	json.Unmarshal(claims["SkinImageWidth"], &w)
	json.Unmarshal(claims["SkinImageHeight"], &h)
	json.Unmarshal(claims["SkinData"], &encoded)
	pixels, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || w != 128 || h != 128 || len(pixels) != 128*128*4 {
		t.Fatal("converted dimensions incorrect", err)
	}
	for _, point := range [][2]int{{0, 0}, {13, 70}, {127, 127}} {
		i := (point[1]*128 + point[0]) * 4
		if !bytes.Equal(pixels[i:i+4], []byte{byte(point[0] * 2), byte(point[1] * 2), 90, 255}) {
			t.Fatal("normalized texture sample changed")
		}
	}
	for k, v := range original {
		if k != "SkinData" && k != "SkinImageWidth" && k != "SkinImageHeight" && k != "SkinId" && k != "SkinGeometryData" && k != "SkinResourcePatch" && !bytes.Equal(v, claims[k]) {
			t.Fatalf("unrelated claim %s changed", k)
		}
	}
	var encodedModel string
	json.Unmarshal(claims["SkinGeometryData"], &encodedModel)
	model, _ := base64.StdEncoding.DecodeString(encodedModel)
	var document map[string]any
	json.Unmarshal(model, &document)
	selected := document["minecraft:geometry"].([]any)[0].(map[string]any)
	if selected["description"].(map[string]any)["texture_width"] != float64(256) || selected["bones"].([]any)[0].(map[string]any)["cubes"].([]any)[0].(map[string]any)["uv"].([]any)[0] != float64(128) {
		t.Fatal("model UV units changed during resizing")
	}
	if message, err = normalizeLoginSkin(claims); err != nil || message != "" {
		t.Fatal("normalization not idempotent", err)
	}
}

func TestPlayerSkinBothDirectionsAndBatchNeighbors(t *testing.T) {
	update := packet.PlayerSkin{UUID: uuid.New(), NewSkinName: "new", OldSkinName: "old", Skin: protocol.Skin{SkinID: "custom-256", SkinImageWidth: 256, SkinImageHeight: 256, SkinData: skinTestPixels(256), SkinResourcePatch: []byte(`{"geometry":{"default":"fixture"}}`), SkinGeometry: []byte(`{"format_version":"1.12.0"}`), OverrideAppearance: true}}
	var body bytes.Buffer
	update.Marshal(protocol.NewWriter(&body, 0))
	header := binary.AppendUvarint(nil, uint64(update.ID())|1<<10)
	wire := append(append([]byte(nil), header...), body.Bytes()...)
	neighbor := []byte{69, 16, 8}
	batch := append([]byte{0xff}, joinGameBatch([][]byte{neighbor, wire})...)
	converted, err := patchClientInitialization860(batch)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := splitGameBatch(converted[1:])
	if err != nil || len(packets) != 2 || !bytes.Equal(packets[0], neighbor) {
		t.Fatal("adjacent packet damaged", err)
	}
	_, n := binary.Uvarint(packets[1])
	buffer := bytes.NewBuffer(packets[1][n:])
	var decoded packet.PlayerSkin
	decoded.Marshal(protocol.NewReader(buffer, 0, true))
	if buffer.Len() != 0 || decoded.UUID != update.UUID || decoded.Skin.SkinImageWidth != 128 || decoded.Skin.SkinImageHeight != 128 || len(decoded.Skin.SkinData) != 128*128*4 || !bytes.Equal(decoded.Skin.SkinGeometry, update.Skin.SkinGeometry) || !bytes.Equal(decoded.Skin.SkinResourcePatch, update.Skin.SkinResourcePatch) || !decoded.Skin.OverrideAppearance || decoded.Skin.Trusted {
		t.Fatal("client skin conversion lost fields")
	}
	returned, err := (&bedrockBridge{}).patchInitialization860(converted)
	if err != nil {
		t.Fatal(err)
	}
	packets, err = splitGameBatch(returned[1:])
	if err != nil || len(packets) != 3 || packets[2][0] != 228 || !bytes.Equal(packets[0], neighbor) || !bytes.Equal(packets[1][:n], header) {
		t.Fatal("server conversion lost subclient/header", err)
	}
	buffer = bytes.NewBuffer(packets[1][n:])
	var server packet.PlayerSkin
	server.Marshal(protocol.NewReader(buffer, 0, true))
	decoded.Skin.Trusted, decoded.Skin.PremiumSkin, decoded.Skin.OverrideAppearance = true, true, false
	if buffer.Len() != 0 || !reflect.DeepEqual(server, decoded) {
		t.Fatal("server skin conversion changed more than NetEase flags")
	}
	for _, broken := range [][]byte{wire[:len(wire)-1], append(append([]byte(nil), wire...), 0)} {
		if _, err := patchPlayerSkin860(broken, false); err == nil {
			t.Fatal("invalid update accepted")
		}
	}
}

func TestSkinPixelLimits(t *testing.T) {
	for _, invalid := range []struct {
		w, h uint32
		size int
	}{{0, 1, 0}, {256, 256, 10}, {1025, 1025, 0}, {256, 64, 256 * 64 * 4}} {
		if _, _, _, err := bedrockSkinPixels(invalid.w, invalid.h, make([]byte, invalid.size)); err == nil {
			t.Fatal("invalid skin accepted", invalid)
		}
	}
	for _, width := range []uint32{64, 128} {
		pixels := skinTestPixels(int(width))
		w, h, result, err := bedrockSkinPixels(width, width, pixels)
		if err != nil || w != width || h != width || !bytes.Equal(result, pixels) {
			t.Fatal("supported pixels changed", err)
		}
	}
}

func TestBDSSkinSizeCompatibility(t *testing.T) {
	if os.Getenv("LOBBYKEEPER_BDS_TEST") == "" {
		t.Skip("requires local BDS")
	}
	geometry, err := os.ReadFile("vendor-src/nemc-tan-lobby-solver/minecraft/skin_geometry.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{64, 128, 256} {
		for _, premium := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d-premium-%v", width, premium), func(t *testing.T) {
				pixels := make([]byte, width*width*4)
				for i := 0; i < len(pixels); i += 4 {
					pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = 24, 120, 240, 255
				}
				testBDSWorldExchange(t, false, false, map[string]any{
					"SkinId": fmt.Sprintf("KeeperSizeTest-%d-%v", width, premium), "SkinImageWidth": width, "SkinImageHeight": width,
					"SkinData": base64.StdEncoding.EncodeToString(pixels), "SkinGeometryData": base64.StdEncoding.EncodeToString(geometry),
					"SkinResourcePatch": base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.customSlim"}}`)),
					"PremiumSkin":       premium,
				})
			})
		}
	}
}

func TestPlayerListIsFollowedByConfirmSkin(t *testing.T) {
	id := uuid.New()
	identity, claims := netEaseLoginFixture(t, id)
	_, record, ok := originalLoginSkin(identity, claims)
	if !ok || record.uid != "1261597877" {
		t.Fatal("NetEase uid not captured", record.uid)
	}
	rememberNetEaseSkin(id, record)
	defer forgetNetEaseSkin(id)
	foreign := uuid.New()
	list := packet.PlayerList{Entries: []protocol.PlayerListEntry{
		{UUID: id, Username: "NetEasePlayer", Skin: protocol.Skin{SkinImageWidth: 64, SkinImageHeight: 64, SkinData: skinTestPixels(64)}},
		{UUID: foreign, Username: "Foreign", Skin: protocol.Skin{SkinImageWidth: 64, SkinImageHeight: 64, SkinData: skinTestPixels(64)}},
	}}
	var payload bytes.Buffer
	list.Marshal(protocol.NewWriter(&payload, 0))
	neighbor := []byte{69, 16, 8}
	batch := append([]byte{0xff}, joinGameBatch([][]byte{append([]byte{63}, payload.Bytes()...), neighbor})...)
	converted, err := (&bedrockBridge{}).patchInitialization860(batch)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := splitGameBatch(converted[1:])
	if err != nil || len(packets) != 3 || packets[0][0] != 63 || !bytes.Equal(packets[2], neighbor) {
		t.Fatal("ConfirmSkin not inserted directly after PlayerList", err)
	}
	header, n := binary.Uvarint(packets[1])
	if header != 228 {
		t.Fatal("unexpected ConfirmSkin id", header)
	}
	r := protocol.NewReader(bytes.NewBuffer(packets[1][n:]), 0, true)
	var count uint32
	r.Varuint32(&count)
	if count != 2 {
		t.Fatal("ConfirmSkin player count", count)
	}
	ids := []uuid.UUID{}
	for range count {
		var confirmed bool
		var player uuid.UUID
		var extra []byte
		r.Bool(&confirmed)
		r.UUID(&player)
		r.ByteSlice(&extra)
		if !confirmed || len(extra) != 0 {
			t.Fatal("ConfirmSkin entry not confirmed")
		}
		ids = append(ids, player)
	}
	var uids, tail [2]string
	for i := range uids {
		r.String(&uids[i])
	}
	for i := range tail {
		r.String(&tail[i])
	}
	if ids[0] != id || ids[1] != foreign || uids[0] != "1261597877" || uids[1] != "" {
		t.Fatal("ConfirmSkin players or uids incorrect", ids, uids)
	}
}
