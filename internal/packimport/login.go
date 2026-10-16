package packimport

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	"github.com/google/uuid"
	"nemcbridge/internal/bridge"
	"strconv"
	"time"
)

func makeLogin(info bunker.TanLobbyLoginResponse) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	uid := strconv.FormatUint(uint64(info.UserUniqueID), 10)
	ident := map[string]any{"identity": uuid.NewString(), "displayName": info.UserPlayerName, "XUID": "", "uid": info.UserUniqueID, "netease_uid": uid}
	cert, err := bridge.SignJWT(key, map[string]any{"extraData": ident, "identityPublicKey": base64.StdEncoding.EncodeToString(public), "exp": time.Now().Add(time.Hour).Unix(), "nbf": time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		return nil, err
	}
	chain, _ := json.Marshal(map[string]any{"chain": []string{cert}})
	envelope, _ := json.Marshal(map[string]any{"Certificate": string(chain), "AuthenticationType": 2, "Token": ""})
	claims := map[string]any{"ClientRandomId": time.Now().UnixNano(), "CurrentInputMode": 1, "DefaultInputMode": 1, "DeviceOS": 7, "DeviceModel": "Windows", "DeviceId": uuid.NewString(), "GameVersion": "1.21.120", "LanguageCode": "zh_CN", "SelfSignedId": uuid.NewString(), "ServerAddress": "127.0.0.1:19132", "SkinId": "NEMCBridgeResourceImport", "SkinData": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{60, 120, 180, 255}, 64*64)), "SkinImageHeight": 64, "SkinImageWidth": 64, "SkinResourcePatch": base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)), "SkinGeometryData": base64.StdEncoding.EncodeToString([]byte(`{"format_version":"1.12.0","minecraft:geometry":[]}`)), "SkinGeometryDataEngineVersion": "1.21.120", "AnimatedImageData": []any{}, "PersonaPieces": []any{}, "PieceTintColors": []any{}, "PersonaSkin": false, "PremiumSkin": false, "TrustedSkin": true, "PlayFabId": "", "CapeData": "", "CapeImageHeight": 0, "CapeImageWidth": 0}
	skin, err := bridge.SignJWT(key, claims)
	if err != nil {
		return nil, err
	}
	body := binary.LittleEndian.AppendUint32(nil, uint32(len(envelope)))
	body = append(body, envelope...)
	body = binary.LittleEndian.AppendUint32(body, uint32(len(skin)))
	body = append(body, skin...)
	pk := []byte{1, 0, 0, 3, 0x5c}
	pk = binary.AppendUvarint(pk, uint64(len(body)))
	return append(pk, body...), nil
}
