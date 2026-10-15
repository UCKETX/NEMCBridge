package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// NetEase clients render other players' skins only in the shape NetEase
// itself produces: the original SkinId, legacy geometry, PremiumSkin=true,
// OverrideSkin=false and the client's own TrustedSkin flag. BDS only ever sees
// the downsampled LobbyKeeper custom skin, so restore the login original for
// every NetEase recipient. All bridge players arrive through the NetEase room.
type neteaseSkin struct {
	name        string
	xuid        string        // NetEase identity XUID; BDS only sees an empty one.
	uid         string        // NetEase account (netease_uid) for ConfirmSkin.
	skin        protocol.Skin // Only the fields normalizeLoginSkin rewrites.
	growthLevel uint32
}

var neteaseSkins = struct {
	sync.Mutex
	players map[uuid.UUID]neteaseSkin
}{players: map[uuid.UUID]neteaseSkin{}}

// originalLoginSkin must run before normalizeLoginSkin modifies the claims.
func originalLoginSkin(identity map[string]any, claims map[string]json.RawMessage) (uuid.UUID, neteaseSkin, bool) {
	text, _ := identity["identity"].(string)
	id, err := uuid.Parse(text)
	if err != nil {
		return uuid.Nil, neteaseSkin{}, false
	}
	record := neteaseSkin{}
	record.name, _ = identity["displayName"].(string)
	record.xuid, _ = identity["XUID"].(string)
	switch uid := identity["netease_uid"].(type) {
	case string:
		record.uid = uid
	case float64:
		record.uid = strconv.FormatFloat(uid, 'f', 0, 64)
	}
	decoded := func(key string) []byte {
		var value string
		_ = json.Unmarshal(claims[key], &value)
		raw, _ := base64.StdEncoding.DecodeString(value)
		return raw
	}
	skin := &record.skin
	_ = json.Unmarshal(claims["SkinId"], &skin.SkinID)
	_ = json.Unmarshal(claims["PlayFabId"], &skin.PlayFabID)
	_ = json.Unmarshal(claims["SkinImageWidth"], &skin.SkinImageWidth)
	_ = json.Unmarshal(claims["SkinImageHeight"], &skin.SkinImageHeight)
	_ = json.Unmarshal(claims["ArmSize"], &skin.ArmSize)
	_ = json.Unmarshal(claims["PremiumSkin"], &skin.PremiumSkin)
	_ = json.Unmarshal(claims["TrustedSkin"], &skin.Trusted)
	_ = json.Unmarshal(claims["OverrideSkin"], &skin.OverrideAppearance)
	_ = json.Unmarshal(claims["GrowthLevel"], &record.growthLevel)
	skin.SkinData = decoded("SkinData")
	skin.SkinGeometry = decoded("SkinGeometryData")
	skin.SkinResourcePatch = decoded("SkinResourcePatch")
	skin.GeometryDataEngineVersion = decoded("SkinGeometryDataEngineVersion")
	if skin.SkinImageWidth == 0 || uint64(skin.SkinImageWidth)*uint64(skin.SkinImageHeight)*4 != uint64(len(skin.SkinData)) {
		return uuid.Nil, neteaseSkin{}, false
	}
	return id, record, true
}

func rememberNetEaseSkin(id uuid.UUID, record neteaseSkin) {
	neteaseSkins.Lock()
	defer neteaseSkins.Unlock()
	neteaseSkins.players[id] = record
}

func forgetNetEaseSkin(id uuid.UUID) {
	neteaseSkins.Lock()
	defer neteaseSkins.Unlock()
	delete(neteaseSkins.players, id)
}

// A NetEase client may change skin in game; keep the unconverted update.
func updateNetEaseSkin(id uuid.UUID, skin protocol.Skin) {
	neteaseSkins.Lock()
	defer neteaseSkins.Unlock()
	if record, ok := neteaseSkins.players[id]; ok {
		record.skin = skin
		neteaseSkins.players[id] = record
	}
}

// BDS offline mode may not keep the login UUID; fall back to the unique name.
func lookupNetEaseSkin(id uuid.UUID, name string) (neteaseSkin, bool) {
	neteaseSkins.Lock()
	defer neteaseSkins.Unlock()
	if record, ok := neteaseSkins.players[id]; ok {
		return record, true
	}
	if name == "" {
		return neteaseSkin{}, false
	}
	for _, record := range neteaseSkins.players {
		if record.name == name {
			return record, true
		}
	}
	return neteaseSkin{}, false
}

// Restore what normalizeLoginSkin/patchPlayerSkin860 replaced. Animations,
// cape and persona data were never modified and stay as BDS relays them.
func restoreNetEaseSkin(skin *protocol.Skin, original protocol.Skin) {
	portableID := skin.SkinID
	skin.SkinID = original.SkinID
	skin.PlayFabID = original.PlayFabID
	skin.SkinResourcePatch = original.SkinResourcePatch
	skin.SkinImageWidth, skin.SkinImageHeight = original.SkinImageWidth, original.SkinImageHeight
	skin.SkinData = original.SkinData
	skin.SkinGeometry = original.SkinGeometry
	skin.GeometryDataEngineVersion = original.GeometryDataEngineVersion
	skin.ArmSize = original.ArmSize
	skin.PremiumSkin = original.PremiumSkin
	skin.Trusted = original.Trusted
	skin.OverrideAppearance = original.OverrideAppearance
	if portableID != "" && strings.HasPrefix(skin.FullID, portableID) {
		skin.FullID = original.SkinID + strings.TrimPrefix(skin.FullID, portableID)
	} else {
		skin.FullID = original.SkinID + skin.CapeID
	}
}

// Skins of players outside the NetEase room (international clients) are shown
// with the flags NetEase clients themselves send for a textured skin.
func neteaseForeignSkin(skin *protocol.Skin) error {
	if _, err := portableClassicSkin(skin); err != nil {
		return err
	}
	if len(skin.SkinData) > 0 && !skin.PersonaSkin {
		skin.Trusted = true
		skin.PremiumSkin = true
		skin.OverrideAppearance = false
	}
	return nil
}

// Convert one skin for a NetEase recipient; growth level 0 if unknown.
func neteaseRecipientSkin(id uuid.UUID, name string, skin *protocol.Skin) (uint32, error) {
	level, _, err := neteaseRecipientEntry(id, name, skin)
	return level, err
}

// Like neteaseRecipientSkin, also returning the NetEase player's own XUID.
func neteaseRecipientEntry(id uuid.UUID, name string, skin *protocol.Skin) (uint32, string, error) {
	if record, ok := lookupNetEaseSkin(id, name); ok {
		restoreNetEaseSkin(skin, record.skin)
		return record.growthLevel, record.xuid, nil
	}
	return 0, "", neteaseForeignSkin(skin)
}

// Names and shapes only: never identifiers, URLs, tokens or texture data.
// The event log truncates long lines, so split the field list.
func loginShapeSummary(identity map[string]any, claims map[string]json.RawMessage) []string {
	shape := func(value string) string {
		digits := value != ""
		for _, r := range value {
			if r < '0' || r > '9' {
				digits = false
			}
		}
		return fmt.Sprintf("len=%d digits=%v", len(value), digits)
	}
	xuid, _ := identity["XUID"].(string)
	var keys []string
	for key := range identity {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := []string{fmt.Sprintf("网易登录身份字段：%s；XUID %s", strings.Join(keys, ","), shape(xuid))}
	keys = keys[:0]
	for key := range claims {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for start := 0; start < len(keys); start += 12 {
		end := min(start+12, len(keys))
		lines = append(lines, "网易登录客户端字段："+strings.Join(keys[start:end], ","))
	}
	var skinItem string
	if json.Unmarshal(claims["SkinIID"], &skinItem) == nil {
		lines = append(lines, "网易登录 SkinIID "+shape(skinItem))
	}
	return lines
}

// confirmSkinPacket builds NetEase ConfirmSkin (228). Eulogist 4.8.0
// (hsMDyQat.Ggkoeh.Marshal 0x140e42400): Varuint32 count of {Bool, UUID,
// ByteSlice}, then one String per entry (launcher uid, per neomega-core),
// then another String per entry. Each player is confirmed with its uid.
func confirmSkinPacket(players []protocol.PlayerListEntry) []byte {
	var body bytes.Buffer
	w := protocol.NewWriter(&body, 0)
	count := uint32(len(players))
	w.Varuint32(&count)
	uids := make([]string, len(players))
	for i := range players {
		confirmed := true
		var extra []byte
		w.Bool(&confirmed)
		w.UUID(&players[i].UUID)
		w.ByteSlice(&extra)
		if record, ok := lookupNetEaseSkin(players[i].UUID, players[i].Username); ok {
			uids[i] = record.uid
		}
	}
	for i := range uids {
		w.String(&uids[i])
	}
	for range players {
		empty := ""
		w.String(&empty)
	}
	return append(binary.AppendUvarint(nil, 228), body.Bytes()...)
}
