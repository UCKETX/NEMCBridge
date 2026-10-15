package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Capture alone (as used by neomega-admin's viewer) can render legacy models
// and foreign store assets. Game clients also need a portable resource identity.
// Import classic textures as local custom skins instead of referring to a store
// catalogue belonging to the other edition. Persona assets are left intact.
func portableClassicSkin(skin *protocol.Skin) (bool, error) {
	if skin.PersonaSkin || len(skin.SkinData) == 0 || len(skin.SkinResourcePatch) == 0 || len(skin.SkinGeometry) == 0 {
		return false, nil
	}
	var patch map[string]any
	var geometry map[string]any
	if json.Unmarshal(skin.SkinResourcePatch, &patch) != nil || json.Unmarshal(skin.SkinGeometry, &geometry) != nil {
		return false, errors.New("皮肤模型或资源映射不是 JSON")
	}
	patchGeometry, ok := patch["geometry"].(map[string]any)
	if !ok {
		return false, nil
	}
	identifier, ok := patchGeometry["default"].(string)
	if !ok || identifier == "" {
		return false, nil
	}
	var models []any
	if list, ok := geometry["minecraft:geometry"].([]any); ok {
		models = list
	} else if legacy, ok := geometry[identifier].(map[string]any); ok {
		description := map[string]any{"identifier": identifier, "texture_width": legacy["texturewidth"], "texture_height": legacy["textureheight"]}
		for _, key := range []string{"visible_bounds_width", "visible_bounds_height", "visible_bounds_offset"} {
			if value, exists := legacy[key]; exists {
				description[key] = value
			}
		}
		model := make(map[string]any)
		for key, value := range legacy {
			if key != "texturewidth" && key != "textureheight" && !strings.HasPrefix(key, "visible_bounds_") {
				model[key] = value
			}
		}
		model["description"] = description
		models = []any{model}
		geometry = map[string]any{"format_version": "1.12.0", "minecraft:geometry": models}
	} else {
		return false, nil // Resource-pack models cannot be reconstructed here.
	}
	portableID := "geometry.humanoid.custom"
	if skin.ArmSize == "slim" || strings.HasSuffix(identifier, "Slim") {
		portableID = "geometry.humanoid.customSlim"
		skin.ArmSize = "slim"
	} else {
		skin.ArmSize = "wide"
	}
	found := false
	for _, value := range models {
		model, ok := value.(map[string]any)
		if !ok {
			continue
		}
		description, ok := model["description"].(map[string]any)
		if !ok || description["identifier"] != identifier {
			continue
		}
		description["identifier"] = portableID
		found = true
	}
	if !found {
		return false, nil
	}
	patchGeometry["default"] = portableID
	var err error
	skin.SkinGeometry, err = json.Marshal(geometry)
	if err != nil {
		return false, err
	}
	skin.SkinResourcePatch, err = json.Marshal(patch)
	if err != nil {
		return false, err
	}
	hash := sha256.New()
	hash.Write(skin.SkinData)
	hash.Write(skin.SkinGeometry)
	hash.Write(skin.SkinResourcePatch)
	skin.SkinID = fmt.Sprintf("LobbyKeeper_%x", hash.Sum(nil)[:16])
	skin.FullID = skin.SkinID
	skin.PlayFabID = ""
	skin.PremiumSkin = false
	skin.Trusted = true
	skin.OverrideAppearance = true
	skin.GeometryDataEngineVersion = []byte("1.12.0")
	return true, nil
}

// BDS 1.21.124 drops 256x256 skins from its player list entirely. Downsample
// the texture before login/skin updates so BDS retains it for all recipients.
// Geometry texture_width/height and UVs stay together in their original units:
// their normalized texture coordinates do not change when the image shrinks.
// Scaling only box UV origins would corrupt the implicitly-sized cube faces.
func bedrockSkinPixels(width, height uint32, pixels []byte) (uint32, uint32, []byte, error) {
	if width == 0 && height == 0 && len(pixels) == 0 {
		return width, height, pixels, nil
	}
	if width == 0 || height == 0 || width > 1024 || height > 1024 || uint64(width)*uint64(height)*4 != uint64(len(pixels)) {
		return 0, 0, nil, errors.New("皮肤 RGBA 尺寸与长度不一致或超出限制")
	}
	if width <= 128 && height <= 128 {
		return width, height, pixels, nil
	}
	var targetW, targetH uint32
	switch {
	case width == height:
		targetW, targetH = 128, 128
	case width == height*2:
		targetW, targetH = 64, 32
	default:
		return 0, 0, nil, errors.New("无法转换非方形或非 2:1 的高清皮肤")
	}
	result := make([]byte, targetW*targetH*4)
	for y := range targetH {
		for x := range targetW {
			from := ((y*height/targetH)*width + x*width/targetW) * 4
			to := (y*targetW + x) * 4
			copy(result[to:to+4], pixels[from:from+4])
		}
	}
	return targetW, targetH, result, nil
}

func normalizeLoginSkin(claims map[string]json.RawMessage) (string, error) {
	original, _ := json.Marshal(claims)
	var width, height uint32
	var data string
	if json.Unmarshal(claims["SkinImageWidth"], &width) != nil || json.Unmarshal(claims["SkinImageHeight"], &height) != nil || json.Unmarshal(claims["SkinData"], &data) != nil {
		return "", errors.New("登录皮肤尺寸或像素字段无效")
	}
	pixels, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", errors.New("登录皮肤像素编码无效")
	}
	w, h, converted, err := bedrockSkinPixels(width, height, pixels)
	if err != nil {
		return "", err
	}
	var model, patch string
	_ = json.Unmarshal(claims["SkinGeometryData"], &model)
	_ = json.Unmarshal(claims["SkinResourcePatch"], &patch)
	geometry, _ := base64.StdEncoding.DecodeString(model)
	resourcePatch, _ := base64.StdEncoding.DecodeString(patch)
	skin := protocol.Skin{SkinImageWidth: w, SkinImageHeight: h, SkinData: converted, SkinGeometry: geometry, SkinResourcePatch: resourcePatch}
	_ = json.Unmarshal(claims["PersonaSkin"], &skin.PersonaSkin)
	_ = json.Unmarshal(claims["ArmSize"], &skin.ArmSize)
	portable, err := portableClassicSkin(&skin)
	if err != nil {
		return "", err
	}
	if w == width && h == height && !portable {
		return "", nil
	}
	claims["SkinImageWidth"], _ = json.Marshal(w)
	claims["SkinImageHeight"], _ = json.Marshal(h)
	claims["SkinData"], _ = json.Marshal(base64.StdEncoding.EncodeToString(converted))
	if portable {
		claims["SkinId"], _ = json.Marshal(skin.SkinID)
		claims["SkinGeometryData"], _ = json.Marshal(base64.StdEncoding.EncodeToString(skin.SkinGeometry))
		claims["SkinResourcePatch"], _ = json.Marshal(base64.StdEncoding.EncodeToString(skin.SkinResourcePatch))
		claims["SkinGeometryDataEngineVersion"], _ = json.Marshal(base64.StdEncoding.EncodeToString(skin.GeometryDataEngineVersion))
		claims["PremiumSkin"] = json.RawMessage("false")
		claims["PlayFabId"] = json.RawMessage(`""`)
		claims["TrustedSkin"] = json.RawMessage("true")
		claims["OverrideSkin"] = json.RawMessage("true")
		claims["ArmSize"], _ = json.Marshal(skin.ArmSize)
	}
	modified, _ := json.Marshal(claims)
	if bytes.Equal(original, modified) {
		return "", nil
	}
	return fmt.Sprintf("皮肤兼容：%dx%d → %dx%d；自定义模型与资源标识规范化=%v", width, height, w, h, portable), nil
}

func patchPlayerSkin860(pk []byte, toNetEase bool) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("玩家换肤包格式无效")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 {
		return nil, errors.New("玩家换肤包头无效")
	}
	buffer := bytes.NewBuffer(pk[n:])
	var update packet.PlayerSkin
	update.Marshal(protocol.NewReader(buffer, 0, true))
	if buffer.Len() != 0 {
		return nil, errors.New("玩家换肤包包含未知字段")
	}
	if toNetEase {
		// Trusted controls rendering, not account authentication. BDS resets
		// it for offline custom skins even when the client supplied a valid one.
		_, err = neteaseRecipientSkin(update.UUID, "", &update.Skin)
	} else {
		updateNetEaseSkin(update.UUID, update.Skin)
		update.Skin.SkinImageWidth, update.Skin.SkinImageHeight, update.Skin.SkinData, err = bedrockSkinPixels(update.Skin.SkinImageWidth, update.Skin.SkinImageHeight, update.Skin.SkinData)
		if err == nil {
			_, err = portableClassicSkin(&update.Skin)
		}
	}
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	update.Marshal(protocol.NewWriter(&body, 0))
	return append(append([]byte(nil), pk[:n]...), body.Bytes()...), nil
}

// Log only sizes, flags and field names. Never log JWTs, identifiers, URLs or
// texture contents while diagnosing skins supplied by a real client.
func skinClaimsSummary(claims map[string]json.RawMessage) string {
	var width, height int
	var trusted, persona bool
	var armSize string
	_ = json.Unmarshal(claims["ArmSize"], &armSize)
	_ = json.Unmarshal(claims["SkinImageWidth"], &width)
	_ = json.Unmarshal(claims["SkinImageHeight"], &height)
	_ = json.Unmarshal(claims["TrustedSkin"], &trusted)
	_ = json.Unmarshal(claims["PersonaSkin"], &persona)
	size := func(key string) int {
		var value string
		if json.Unmarshal(claims[key], &value) != nil {
			return -1
		}
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return -1
		}
		return len(decoded)
	}
	var fields []string
	for key, value := range claims {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "skin") || strings.Contains(lower, "cape") || strings.Contains(lower, "geometry") || strings.Contains(lower, "outfit") {
			fields = append(fields, fmt.Sprintf("%s(%d)", key, len(value)))
		}
	}
	sort.Strings(fields)
	// Only report known arm-size values, never arbitrary client strings.
	if armSize != "wide" && armSize != "slim" {
		armSize = "unset/unknown"
	}
	return fmt.Sprintf("pixels=%dx%d RGBA=%d geometry=%d patch=%d trusted=%v persona=%v arms=%s fields=[%s]", width, height, size("SkinData"), size("SkinGeometryData"), size("SkinResourcePatch"), trusted, persona, armSize, strings.Join(fields, ","))
}
