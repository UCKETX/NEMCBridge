package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/raknet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/webrtc"
	nepacket "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol/packet"
	tanpacket "github.com/Happy2018new/nemc-tan-lobby-solver/protocol/packet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/service/signaling"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// The diagnostic guest is explicit and time limited. It does not create a room,
// alter world data or store incoming credentials, textures or player identities.
func fetchNativeAccess(ctx context.Context, cfg Config, token, room string) (bunker.TanLobbyLoginResponse, error) {
	var info bunker.TanLobbyLoginResponse
	if token == "" {
		return info, errors.New("未保存机器人令牌")
	}
	body, _ := json.Marshal(map[string]string{"login_token": token, "room_id": room})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.AuthServer, "/")+"/api/phoenix/tan_lobby_login", bytes.NewReader(body))
	if err != nil {
		return info, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := newAuthHTTP(cfg.AuthServer).Do(req)
	if err != nil {
		return info, errors.New("原生房间鉴权请求失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return info, fmt.Errorf("原生房间鉴权 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || json.Unmarshal(data, &info) != nil {
		return info, errors.New("原生房间鉴权响应无效")
	}
	if !info.Success {
		return info, fmt.Errorf("原生房间鉴权失败：%s", sanitize(info.ErrorInfo, token))
	}
	if len(info.RaknetRand) != 16 || len(info.RaknetAESRand) < 16 || len(info.SignalingSeed) != 16 || len(info.SignalingTicket) < 16 || len(info.EncryptKeyBytes) != 32 || len(info.DecryptKeyBytes) != 32 {
		return info, errors.New("原生房间加密材料长度无效")
	}
	info.RaknetAESRand, info.SignalingTicket = info.RaknetAESRand[:16], info.SignalingTicket[:16]
	return info, nil
}

func joinNativeSkinRoom(ctx context.Context, cfg Config, token, room, password string, trace func(string)) (*Room, *nethernet.Conn, bunker.TanLobbyLoginResponse, error) {
	roomNumber, err := strconv.ParseUint(room, 10, 32)
	if err != nil || roomNumber == 0 {
		return nil, nil, bunker.TanLobbyLoginResponse{}, errors.New("房间号无效")
	}
	info, err := fetchNativeAccess(ctx, cfg, token, room)
	if err != nil {
		return nil, nil, info, err
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, nil, info, err
	}
	r := &Room{networkID: binary.LittleEndian.Uint64(random[:]), done: make(chan struct{}), trace: trace}
	if r.networkID == 0 {
		r.networkID = 1
	}
	ok := false
	defer func() {
		if !ok {
			r.Close()
		}
	}()
	r.control, err = raknet.DialContext(ctx, info.RaknetServerAddress)
	if err != nil {
		return nil, nil, info, errors.New("原生房间节点连接失败")
	}
	stop := context.AfterFunc(ctx, func() { r.Close() })
	defer stop()
	r.encoder = tanpacket.NewEncoder(r.control)
	r.decoder, err = tanpacket.NewDecoder(r.control)
	if err != nil {
		return nil, nil, info, err
	}
	if err = r.write(&tanpacket.TanLoginRequest{PlayerID: info.UserUniqueID, Rand: info.RaknetRand, AESRand: info.RaknetAESRand, PlayerName: info.UserPlayerName}); err != nil {
		return nil, nil, info, err
	}
	p, err := r.read()
	if err != nil {
		return nil, nil, info, err
	}
	login, valid := p.(*tanpacket.TanLoginResponse)
	if !valid || login.ErrorCode != 0 {
		return nil, nil, info, errors.New("原生房间节点拒绝登录")
	}
	if err = r.encoder.EnableEncryption(info.EncryptKeyBytes, info.DecryptKeyBytes); err != nil {
		return nil, nil, info, err
	}
	if err = r.decoder.EnableEncryption(info.EncryptKeyBytes, info.DecryptKeyBytes); err != nil {
		return nil, nil, info, err
	}
	if err = r.write(&tanpacket.TanEnterRoomRequest{OwnerID: info.RoomOwnerID, RoomID: uint32(roomNumber), EnterPassword: password, NetherNetID: strconv.FormatUint(r.networkID, 10), SupportWebRTCCompress: true}); err != nil {
		return nil, nil, info, err
	}
	p, err = r.read()
	if err != nil {
		return nil, nil, info, err
	}
	entered, valid := p.(*tanpacket.TanEnterRoomResponse)
	if !valid || entered.ErrorCode != 0 {
		return nil, nil, info, errors.New("原生房间拒绝进入，请确认房间开放且密码正确")
	}
	var remoteID uint64
	for remoteID == 0 {
		p, err = r.read()
		if err != nil {
			return nil, nil, info, err
		}
		if ready, ok := p.(*tanpacket.TanNotifyServerReady); ok {
			remoteID, _ = strconv.ParseUint(ready.NetherNetID, 10, 64)
		}
		if _, kicked := p.(*tanpacket.TanKickOutResponse); kicked {
			return nil, nil, info, errors.New("原生房间拒绝诊断访客")
		}
	}
	r.signal, err = (signaling.Dialer{NetherNetID: r.networkID, RefreshTime: 30 * time.Minute}).DialContext(ctx, info.SignalingServerAddress, info.UserUniqueID, info.SignalingSeed, info.SignalingTicket)
	if err != nil {
		return nil, nil, info, errors.New("原生房间信令连接失败")
	}
	settings := webrtc.SettingEngine{LoggerFactory: transportLogs{trace: trace}}
	conn, err := (nethernet.Dialer{API: webrtc.NewAPI(webrtc.WithSettingEngine(settings))}).DialContext(ctx, remoteID, tracedSignaling{Signaling: r.signal, trace: trace})
	if err != nil {
		return nil, nil, info, errors.New("原生房间 WebRTC 连接失败")
	}
	ok = true
	return r, conn, info, nil
}

type nativeProbeStream struct{ *nethernet.Conn }

func (s nativeProbeStream) Write(p []byte) (int, error) {
	n, err := s.Conn.Write(p[1:])
	return n + 1, err
}
func (s nativeProbeStream) ReadPacket() ([]byte, error) {
	p, err := s.Conn.ReadPacket()
	return append([]byte{0xfe}, p...), err
}

func nativeProbeLogin(key *ecdsa.PrivateKey, info bunker.TanLobbyLoginResponse) ([]byte, error) {
	public, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	identity, err := signGameJWT(key, map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "nbf": time.Now().Add(-time.Minute).Unix(), "extraData": map[string]any{"identity": uuid.NewString(), "displayName": info.UserPlayerName, "XUID": "", "uid": info.UserUniqueID}, "identityPublicKey": base64.StdEncoding.EncodeToString(public)})
	if err != nil {
		return nil, err
	}
	chain, _ := json.Marshal(map[string]any{"chain": []string{identity}})
	envelope, _ := json.Marshal(map[string]any{"Certificate": string(chain), "AuthenticationType": 2, "Token": ""})
	geometry := []byte(`{"format_version":"1.12.0","minecraft:geometry":[{"description":{"identifier":"geometry.humanoid.custom","texture_width":64,"texture_height":64},"bones":[{"name":"head","pivot":[0,24,0],"cubes":[{"origin":[-4,24,-4],"size":[8,8,8],"uv":[0,0]}]}]}]}`)
	claims := map[string]any{"ClientRandomId": time.Now().UnixNano(), "CurrentInputMode": 1, "DefaultInputMode": 1, "DeviceOS": 2, "DeviceModel": "Android", "DeviceId": uuid.NewString(), "GameVersion": "1.21.120", "LanguageCode": "zh_CN", "SelfSignedId": uuid.NewString(), "ThirdPartyName": info.UserPlayerName, "ThirdPartyNameOnly": false, "ServerAddress": "127.0.0.1:19132", "SkinId": uuid.NewString(), "SkinData": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{100, 150, 200, 255}, 64*64)), "SkinImageWidth": 64, "SkinImageHeight": 64, "SkinGeometryData": base64.StdEncoding.EncodeToString(geometry), "SkinResourcePatch": base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)), "SkinGeometryDataEngineVersion": base64.StdEncoding.EncodeToString([]byte("1.21.120")), "AnimatedImageData": []any{}, "PersonaPieces": []any{}, "PieceTintColors": []any{}, "PersonaSkin": false, "PremiumSkin": false, "TrustedSkin": true, "OverrideSkin": true, "PlayFabId": "", "CapeData": "", "CapeId": "", "CapeImageWidth": 0, "CapeImageHeight": 0, "SkinColor": "#0", "ArmSize": "wide", "SkinAnimationData": "", "UIProfile": 1}
	skin, err := signGameJWT(key, claims)
	if err != nil {
		return nil, err
	}
	request := binary.LittleEndian.AppendUint32(nil, uint32(len(envelope)))
	request = append(request, envelope...)
	request = binary.LittleEndian.AppendUint32(request, uint32(len(skin)))
	request = append(request, skin...)
	wire := binary.BigEndian.AppendUint32([]byte{1}, 860)
	wire = binary.AppendUvarint(wire, uint64(len(request)))
	return append(wire, request...), nil
}

func inspectNativeSkinRoom(ctx context.Context, cfg Config, token, room, password string, trace func(string)) error {
	session, err := acquireAccountSession(token)
	if err != nil {
		return err
	}
	defer session.Close()
	// Older running builds do not own a session lock. Check their configured
	// console before authentication, which itself may invalidate their login.
	if err := rejectRunningRoom(ctx, cfg.WebPort); err != nil {
		return err
	}
	trace("开始原生房间皮肤结构诊断；仅记录包 ID、尺寸及标志")
	r, conn, info, err := joinNativeSkinRoom(ctx, cfg, token, room, password, trace)
	if err != nil {
		return err
	}
	defer r.Close()
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close(); r.Close() })
	defer stop()
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}
	game := &bedrockBridge{packetStream: nativeProbeStream{conn}, key: key, trace: trace}
	var compression packet.Compression = packet.FlateCompression
	nativeFixed := false
	write := func(pk []byte, compressed bool) error {
		data := joinGameBatch([][]byte{pk})
		if compressed {
			var err error
			data, err = compression.Compress(data)
			if err != nil {
				return err
			}
			if !nativeFixed {
				data = append([]byte{byte(compression.EncodeCompression())}, data...)
			}
		}
		if game.send != nil {
			data = game.send.encrypt(data)
		}
		_, err := game.packetStream.Write(append([]byte{0xfe}, data...))
		return err
	}
	if err := write([]byte{0xc1, 1, 0, 0, 3, 0x5c}, false); err != nil {
		return err
	}
	compressed, loggedIn := false, false
	seen := map[uint64]bool{}
	for {
		wire, err := conn.ReadPacket()
		if err != nil {
			if ctx.Err() != nil {
				trace("原生房间诊断时间已结束")
				return nil
			}
			return errors.New("原生房间关闭了游戏连接")
		}
		if game.recv != nil {
			wire, err = game.recv.decrypt(wire)
			if err != nil {
				return err
			}
		}
		if compressed {
			if nativeFixed {
				wire, err = compression.Decompress(wire, maxDecodedGameBatch)
			} else if len(wire) > 0 && wire[0] == 1 {
				wire, err = packet.SnappyCompression.Decompress(wire[1:], maxDecodedGameBatch)
			} else {
				wire, err = decodeGameCompression(wire)
			}
			if err != nil {
				return err
			}
		}
		packets, err := splitGameBatch(wire)
		if err != nil {
			return err
		}
		for _, pk := range packets {
			id, n := binary.Uvarint(pk)
			if n <= 0 {
				return errors.New("原生游戏包头无效")
			}
			id &= 0x3ff
			if !seen[id] {
				seen[id] = true
				trace(fmt.Sprintf("原生数据包：ID=%d bytes=%d", id, len(pk)))
			}
			switch id {
			case 143:
				if len(pk) < n+4 {
					return errors.New("原生网络设置不完整")
				}
				algorithm := binary.LittleEndian.Uint16(pk[n+2:])
				trace(fmt.Sprintf("原生网络设置：threshold=%d algorithm=%d", binary.LittleEndian.Uint16(pk[n:]), algorithm))
				switch algorithm {
				case 0:
					compression = packet.FlateCompression
				case 1:
					compression = packet.SnappyCompression
				case 2:
					compression = nepacket.NeteaseCompression()
					nativeFixed = true
				case 0xffff:
					compression = packet.NopCompression
				default:
					return errors.New("原生压缩算法未支持")
				}
				compressed = true
				login, err := nativeProbeLogin(key, info)
				if err != nil {
					return err
				}
				if err := write(login, true); err != nil {
					return err
				}
			case 3:
				length, m := binary.Uvarint(pk[n:])
				if m <= 0 || length > uint64(len(pk)-n-m) {
					return errors.New("原生握手格式无效")
				}
				if err := game.enableGameEncryption(string(pk[n+m : n+m+int(length)])); err != nil {
					return err
				}
			case 2:
				if len(pk) < n+4 {
					return errors.New("原生登录状态无效")
				}
				status := binary.BigEndian.Uint32(pk[n:])
				trace(fmt.Sprintf("原生登录状态：%d", status))
				if status == 0 && !loggedIn {
					loggedIn = true
					if err := write([]byte{0x81, 1, 0}, true); err != nil {
						return err
					}
				}
			case 5:
				return errors.New("原生房间拒绝了游戏登录或结束了会话")
			case 6, 7:
				status := byte(3)
				if id == 7 {
					status = 4
				}
				if err := write([]byte{8, status, 0, 0}, true); err != nil {
					return err
				}
			case 63:
				trace(nativePlayerListSummary(pk[n:]))
			case 93:
				trace(nativePlayerSkinSummary(pk[n:]))
			}
		}
	}
}

func rejectRunningRoom(ctx context.Context, port int) error {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/status", port), nil)
	if err != nil {
		return err
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	} // No legacy console; the OS session lock is authoritative.
	defer resp.Body.Close()
	var state struct {
		Running bool `json:"running"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&state) != nil {
		return errors.New("无法确认现有控制台是否正在开房，已取消诊断登录")
	}
	if state.Running {
		return errAccountSessionBusy
	}
	return nil
}

func nativeSkinSummary(s protocol.Skin) string {
	var geometry map[string]any
	_ = json.Unmarshal(s.SkinGeometry, &geometry)
	version, _ := geometry["format_version"].(string)
	if len(version) > 16 {
		version = "unknown"
	}
	engine := string(s.GeometryDataEngineVersion)
	if len(engine) > 20 {
		engine = "unknown"
	}
	return fmt.Sprintf("pixels=%dx%d RGBA=%d geometry=%d format=%q patch=%d engine=%q trusted=%v premium=%v persona=%v override=%v", s.SkinImageWidth, s.SkinImageHeight, len(s.SkinData), len(s.SkinGeometry), version, len(s.SkinResourcePatch), engine, s.Trusted, s.PremiumSkin, s.PersonaSkin, s.OverrideAppearance)
}

func nativePlayerSkinSummary(body []byte) (result string) {
	defer func() {
		if recover() != nil {
			result = "原生换肤包：字段无法按公共格式解析"
		}
	}()
	buffer := bytes.NewBuffer(body)
	var pk packet.PlayerSkin
	pk.Marshal(protocol.NewReader(buffer, 0, true))
	return fmt.Sprintf("原生换肤：%s trailing=%d", nativeSkinSummary(pk.Skin), buffer.Len())
}

func nativePlayerListSummary(body []byte) (result string) {
	stage := "header"
	buffer := bytes.NewBuffer(body)
	defer func() {
		if recover() != nil {
			result += fmt.Sprintf("；原生玩家格式未完全解析 stage=%s offset=%d remaining=%d", stage, len(body)-buffer.Len(), buffer.Len())
		}
	}()
	reader := protocol.NewReader(buffer, 0, true)
	var pk packet.PlayerList
	reader.Uint8(&pk.ActionType)
	var count uint32
	reader.Varuint32(&count)
	result = fmt.Sprintf("原生玩家列表：action=%d entries=%d", pk.ActionType, count)
	if count > 1024 {
		return result + " count out of bounds"
	}
	pk.Entries = make([]protocol.PlayerListEntry, count)
	for i := range pk.Entries {
		e := &pk.Entries[i]
		stage = fmt.Sprintf("entry[%d].identity", i)
		reader.UUID(&e.UUID)
		if pk.ActionType != 0 {
			continue
		}
		reader.Varint64(&e.EntityUniqueID)
		reader.String(&e.Username)
		reader.String(&e.XUID)
		reader.String(&e.PlatformChatID)
		reader.Int32(&e.BuildPlatform)
		result += fmt.Sprintf("\n  entry[%d] skinOffset=%d", i, len(body)-buffer.Len())
		stage = fmt.Sprintf("entry[%d].skin", i)
		readNativeSkinDiagnostic(reader, &e.Skin, &stage, &result)
		result += " " + nativeSkinSummary(e.Skin)
		stage = fmt.Sprintf("entry[%d].flags", i)
		reader.Bool(&e.Teacher)
		reader.Bool(&e.Host)
		reader.Bool(&e.SubClient)
		reader.ARGB(&e.PlayerColour)
	}
	stage = "trusted"
	if pk.ActionType == 0 {
		for i := range pk.Entries {
			reader.Bool(&pk.Entries[i].Skin.Trusted)
		}
	}
	if pk.ActionType != 0 {
		return "原生玩家列表：移除条目"
	}
	result = fmt.Sprintf("原生玩家列表：entries=%d trailing=%d", len(pk.Entries), buffer.Len())
	for i, entry := range pk.Entries {
		result += fmt.Sprintf("\n  皮肤[%d] %s", i, nativeSkinSummary(entry.Skin))
	}
	for group := 0; group < 2; group++ {
		for i := range pk.Entries {
			var present, flag bool
			reader.Bool(&present)
			result += fmt.Sprintf("\n  optional[%d][%d] present=%v", group, i, present)
			if present {
				reader.Bool(&flag)
				lengths := make([]int, 5)
				for j := range lengths {
					var value string
					reader.String(&value)
					lengths[j] = len(value)
				}
				result += fmt.Sprintf(" flag=%v stringLengths=%v", flag, lengths)
			}
		}
	}
	for i := range pk.Entries {
		var value string
		reader.String(&value)
		result += fmt.Sprintf("\n  string[%d] bytes=%d", i, len(value))
	}
	for i := range pk.Entries {
		var value uint32
		reader.Uint32(&value)
		result += fmt.Sprintf("\n  level[%d] zero=%v", i, value == 0)
	}
	for i := range pk.Entries {
		var value bool
		reader.Bool(&value)
		result += fmt.Sprintf("\n  finalFlag[%d]=%v", i, value)
	}
	return result + fmt.Sprintf("\n  remaining=%d", buffer.Len())
}

func readNativeSkinDiagnostic(r protocol.IO, s *protocol.Skin, stage, result *string) {
	fields := []struct {
		name string
		read func()
	}{
		{"id", func() { r.String(&s.SkinID); *result += fmt.Sprintf(" idBytes=%d", len(s.SkinID)) }},
		{"playfab", func() { r.String(&s.PlayFabID); *result += fmt.Sprintf(" playfabBytes=%d", len(s.PlayFabID)) }},
		{"patch", func() {
			r.ByteSlice(&s.SkinResourcePatch)
			*result += fmt.Sprintf(" patchBytes=%d", len(s.SkinResourcePatch))
		}},
		{"width", func() { r.Uint32(&s.SkinImageWidth); *result += fmt.Sprintf(" width=%d", s.SkinImageWidth) }},
		{"height", func() { r.Uint32(&s.SkinImageHeight); *result += fmt.Sprintf(" height=%d", s.SkinImageHeight) }},
		{"pixels", func() { r.ByteSlice(&s.SkinData); *result += fmt.Sprintf(" pixelBytes=%d", len(s.SkinData)) }},
		{"animations", func() { protocol.SliceUint32Length(r, &s.Animations) }},
		{"capeWidth", func() { r.Uint32(&s.CapeImageWidth) }},
		{"capeHeight", func() { r.Uint32(&s.CapeImageHeight) }},
		{"capePixels", func() { r.ByteSlice(&s.CapeData) }},
		{"geometry", func() { r.ByteSlice(&s.SkinGeometry); *result += fmt.Sprintf(" geometryBytes=%d", len(s.SkinGeometry)) }},
		{"engine", func() { r.ByteSlice(&s.GeometryDataEngineVersion) }},
		{"animation", func() { r.ByteSlice(&s.AnimationData) }},
		{"capeID", func() { r.String(&s.CapeID) }},
		{"fullID", func() { r.String(&s.FullID) }},
		{"arms", func() { r.String(&s.ArmSize) }},
		{"colour", func() { r.String(&s.SkinColour) }},
		{"personaPieces", func() { protocol.SliceUint32Length(r, &s.PersonaPieces) }},
		{"tints", func() { protocol.SliceUint32Length(r, &s.PieceTintColours) }},
		{"premium", func() { r.Bool(&s.PremiumSkin) }},
		{"persona", func() { r.Bool(&s.PersonaSkin) }},
		{"personaCape", func() { r.Bool(&s.PersonaCapeOnClassicSkin) }},
		{"primary", func() { r.Bool(&s.PrimaryUser) }},
		{"override", func() { r.Bool(&s.OverrideAppearance) }},
	}
	for _, f := range fields {
		*stage = "skin." + f.name
		f.read()
	}
}
