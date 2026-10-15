package integration

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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	neprotocol "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol"
	nepacket "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol/packet"
	"github.com/google/uuid"
	bdsraknet "github.com/sandertv/go-raknet"
	stdprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	stdpacket "github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/bridge"
	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/netease"
	"nemcbridge/internal/netease/neteasetest"
	"nemcbridge/internal/relay"
)

func makeOfflineTestLogin(t *testing.T, overrides ...map[string]any) []byte {
	return makeTestLoginWithIdentity(t, nil, overrides...)
}

func makeTestLoginWithIdentity(t *testing.T, identityFields map[string]any, overrides ...map[string]any) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	identityID := uuid.NewString()
	identity := map[string]any{"identity": identityID, "displayName": "KeeperTest" + identityID[:6], "XUID": ""}
	for key, value := range identityFields {
		identity[key] = value
	}
	publicRaw, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	pub, _ := bridge.SignJWT(key, map[string]any{"extraData": identity, "identityPublicKey": base64.StdEncoding.EncodeToString(publicRaw)})
	chain, _ := json.Marshal(map[string]any{"chain": []string{pub}})
	envelope, _ := json.Marshal(map[string]any{"Certificate": string(chain), "AuthenticationType": 2, "Token": ""})
	claims := map[string]any{
		"ClientRandomId": time.Now().UnixNano(), "CurrentInputMode": 1, "DefaultInputMode": 1, "DeviceOS": 7, "DeviceModel": "Windows", "DeviceId": uuid.NewString(), "GameVersion": "1.21.124", "LanguageCode": "en_US", "SelfSignedId": uuid.NewString(), "ServerAddress": "127.0.0.1:19132", "SkinId": "KeeperTest", "SkinData": base64.StdEncoding.EncodeToString(make([]byte, 64*64*4)), "SkinImageHeight": 64, "SkinImageWidth": 64, "SkinResourcePatch": base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)), "SkinGeometryData": base64.StdEncoding.EncodeToString([]byte(`{"format_version":"1.12.0","minecraft:geometry":[]}`)), "SkinGeometryDataEngineVersion": "1.21.124", "AnimatedImageData": []any{}, "PersonaPieces": []any{}, "PieceTintColors": []any{}, "PersonaSkin": false, "PremiumSkin": false, "TrustedSkin": true, "PlayFabId": "", "CapeData": "", "CapeImageHeight": 0, "CapeImageWidth": 0,
	}
	for _, fields := range overrides {
		for key, value := range fields {
			claims[key] = value
		}
	}
	skin, _ := bridge.SignJWT(key, claims)
	request := binary.LittleEndian.AppendUint32(nil, uint32(len(envelope)))
	request = append(request, envelope...)
	request = binary.LittleEndian.AppendUint32(request, uint32(len(skin)))
	request = append(request, skin...)
	pk := []byte{1, 0, 0, 3, 0x5c}
	pk = binary.AppendUvarint(pk, uint64(len(request)))
	pk = append(pk, request...)
	return append([]byte{0xfe, 0xff}, gamebatch.Join([][]byte{pk})...)
}

func TestBDSOfflineGameEncryptionBridge(t *testing.T) {
	testBDSWorldExchange(t, false, false)
}

func TestBDSCachedWorldExchange(t *testing.T) {
	testBDSWorldExchange(t, true, false)
}

func TestWebRTCFullWorldExchange(t *testing.T) {
	testBDSWorldExchange(t, false, true)
}

func testBDSWorldExchange(t *testing.T, cache, webRTC bool, skinOverrides ...map[string]any) {
	testBDSWorldExchangeOptions(t, cache, webRTC, false, skinOverrides...)
}

func TestBDSChatRoundTrip(t *testing.T) {
	testBDSWorldExchangeOptions(t, false, false, true)
}

func TestWebRTCChatRoundTrip(t *testing.T) {
	testBDSWorldExchangeOptions(t, false, true, true)
}

func testBDSWorldExchangeOptions(t *testing.T, cache, webRTC, chat bool, skinOverrides ...map[string]any) {
	testBDSIdentityWorldExchange(t, cache, webRTC, chat, nil, skinOverrides...)
}

func TestBDSNetEaseXUIDWorldExchange(t *testing.T) {
	testBDSIdentityWorldExchange(t, false, false, true, map[string]any{"netease_uid": "1261597877"})
}

func testBDSIdentityWorldExchange(t *testing.T, cache, webRTC, chat bool, identity map[string]any, skinOverrides ...map[string]any) {
	testBDSWorldScenario(t, cache, webRTC, chat, false, identity, skinOverrides...)
}

func TestBDSCommandRoundTrip(t *testing.T) {
	testBDSWorldScenario(t, false, false, true, true, map[string]any{"netease_uid": "1261597877"})
}

func TestWebRTCCommandRoundTrip(t *testing.T) {
	testBDSWorldScenario(t, false, true, true, true, map[string]any{"netease_uid": "1261597877"})
}

func testBDSWorldScenario(t *testing.T, cache, webRTC, chat, commands bool, identity map[string]any, skinOverrides ...map[string]any) {
	addr := os.Getenv("LOBBYKEEPER_BDS_TEST")
	if addr == "" {
		t.Skip("requires local BDS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	server, err := bdsraknet.DialContext(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	context.AfterFunc(ctx, func() { server.Close() })
	relayBridge, err := bridge.New(server, func(s string) { t.Log(s) })
	if err != nil {
		t.Fatal(err)
	}
	var stream relay.Stream = relayBridge
	if webRTC {
		stream = localWebRTCFrontend(t, ctx, relayBridge)
	}
	if _, err = stream.Write([]byte{0xfe, 6, 0xc1, 1, 0, 0, 3, 0x5c}); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.ReadPacket(); err != nil {
		t.Fatal(err)
	}
	login := makeTestLoginWithIdentity(t, identity, skinOverrides...)
	loginPackets, err := gamebatch.Split(login[2:])
	if err != nil || len(loginPackets) != 1 {
		t.Fatal("test login batch invalid", err)
	}
	_, loginHeaderSize := binary.Uvarint(loginPackets[0])
	_, loginLengthSize := binary.Uvarint(loginPackets[0][loginHeaderSize+4:])
	loginIdentity, _, err := bridge.ParseIdentity(loginPackets[0][loginHeaderSize+4+loginLengthSize:])
	if err != nil {
		t.Fatal(err)
	}
	loginUUID, err := uuid.Parse(loginIdentity["identity"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Write(login); err != nil {
		t.Fatal(err)
	}
	cacheFlag := byte(0)
	if cache {
		cacheFlag = 1
	}
	loggedIn := false
	worldLoaded := false
	biomeLoaded, playerListed, chunkProvided := false, false, false
	skinVerified := len(skinOverrides) == 0
	subChunkRequested, terrainProvided := false, false
	chatSent, chatReceived := false, false
	chatMessage := "NEMCBridge 中文双向聊天 🌍 " + loginUUID.String()[:8]
	commandLines := []string{"/help", "/help 1", "/nemcbridge_command_missing", "/help"}
	commandIndex := 0
	commandReceived := false
	commandRequestID := ""
	var runtimeID uint64
	seenIDs := map[uint64]bool{}
	for i := 0; i < 512; i++ {
		p, err := stream.ReadPacket()
		if err != nil {
			if commands && commandIndex > 0 {
				t.Fatalf("command %q did not complete: %v", commandLines[commandIndex-1], err)
			}
			t.Fatal(err)
		}
		plain, err := gamebatch.Decode(p[1:])
		if err != nil {
			t.Fatal(err)
		}
		packets, err := gamebatch.Split(plain)
		if err != nil {
			t.Fatal(err)
		}
		for _, pk := range packets {
			id, n := binary.Uvarint(pk)
			if commands && id&0x3ff == stdpacket.IDCommandOutput {
				// Independently decode BDS output using the NetEase protocol.
				buffer := bytes.NewBuffer(pk[n:])
				var output nepacket.CommandOutput
				output.Marshal(neprotocol.NewReader(buffer, 0, true))
				if buffer.Len() != 0 {
					t.Fatal("unread bytes in NetEase command output")
				}
				// Vanilla player-origin replies clear RequestID, retaining UUID.
				matchesRequest := output.CommandOrigin.RequestID == commandRequestID || output.CommandOrigin.RequestID == "" && output.CommandOrigin.UUID == loginUUID
				if matchesRequest && commandIndex > 0 && !commandReceived {
					if (commandIndex == 3) != (output.SuccessCount == 0) {
						t.Fatalf("unexpected result for %q: success=%d", commandLines[commandIndex-1], output.SuccessCount)
					}
					commandReceived = true
					t.Logf("command round trip passed: request=%d success=%d WebRTC=%v", commandIndex, output.SuccessCount, webRTC)
				}
			}
			if os.Getenv("LOBBYKEEPER_TRACE_WORLD_TEST") != "" && !seenIDs[id&0x3ff] {
				t.Logf("world packet id=%d bytes=%d batch=%d wire=%d cache=%v", id&0x3ff, len(pk), i, len(p), cache)
			}
			seenIDs[id&0x3ff] = true
			if id&0x3ff == stdpacket.IDText && chatSent {
				buffer := bytes.NewBuffer(pk[n:])
				var message stdpacket.Text
				message.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if message.Message == chatMessage {
					if !bytes.Equal(buffer.Bytes(), []byte{0}) {
						t.Fatal("chat reply missing NetEase extension")
					}
					chatReceived = true
					t.Log("BDS echoed Unicode chat through both protocol conversions")
				}
			}
			if id&0x3ff == 3 {
				t.Fatal("RakNet encryption handshake leaked to NetherNet")
			}
			if id&0x3ff == 2 && len(pk) >= n+4 && binary.BigEndian.Uint32(pk[n:]) == 0 {
				t.Log("BDS accepted rewritten login and encrypted ClientToServerHandshake; plaintext PlayStatus=0 returned")
				loggedIn = true
				if _, err = stream.Write(append([]byte{0xfe, 0xff}, gamebatch.Join([][]byte{{0x81, 1, cacheFlag}})...)); err != nil {
					t.Fatal(err)
				}
			}
			if id&0x3ff == 6 || id&0x3ff == 7 {
				response := byte(3)
				if id&0x3ff == 7 {
					response = 4
				}
				if _, err = stream.Write(append([]byte{0xfe, 0xff}, gamebatch.Join([][]byte{{8, response, 0, 0}})...)); err != nil {
					t.Fatal(err)
				}
			}
			if id&0x3ff == 11 {
				if capture := os.Getenv("LOBBYKEEPER_CAPTURE_STARTGAME"); capture != "" {
					if err := os.WriteFile(capture, pk[:len(pk)-netease.WorldExtensionSize], 0600); err != nil {
						t.Fatal(err)
					}
				}
				if !loggedIn {
					t.Fatal("world settings were not converted")
				}
				if len(pk) < 28+netease.WorldExtensionSize || !bytes.Equal(pk[len(pk)-netease.WorldExtensionSize:], make([]byte, netease.WorldExtensionSize)) {
					t.Fatal("real BDS packet did not gain the NetEase world extension")
				}
				tail := pk[len(pk)-28-netease.WorldExtensionSize : len(pk)-netease.WorldExtensionSize]
				if binary.LittleEndian.Uint64(tail) != 0 || tail[24] != 0 || tail[25] != 1 {
					t.Fatal("real BDS world compatibility fields incorrect")
				}
				buffer := bytes.NewBuffer(pk[n:])
				var world stdpacket.StartGame
				world.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if buffer.Len() != netease.WorldExtensionSize || world.ServerBlockStateChecksum != 0 || world.ClientSideGeneration || !world.UseBlockNetworkIDHashes {
					t.Fatal("independent protocol 860 reader rejected converted common StartGame layout")
				}
				t.Log("real BDS resource-pack exchange and independent StartGame 860 decoding passed")
				worldLoaded = true
				runtimeID = world.EntityRuntimeID
			}
			if id&0x3ff == 122 {
				if !worldLoaded {
					t.Fatal("BDS world/biome conversion did not complete")
				}
				neteasetest.VerifyBiomePayload(t, pk[n:])
				t.Log("real BDS biome conversion and NetEase extension positions passed")
				biomeLoaded = true
				if _, err := stream.Write(append([]byte{0xfe, 0xff}, gamebatch.Join([][]byte{{69, 16, 8}})...)); err != nil {
					t.Fatal(err)
				}
			}
			if id&0x3ff == 63 {
				buffer := bytes.NewBuffer(pk[n:])
				var list stdpacket.PlayerList
				list.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if list.ActionType == 0 && len(list.Entries) > 0 {
					for _, entry := range list.Entries {
						if identity != nil && entry.Username == loginIdentity["displayName"] {
							t.Logf("BDS account compatibility: player UUID preserved=%v", entry.UUID == loginUUID)
						}
						t.Logf("BDS skin: pixels=%dx%d bytes=%d geometryBytes=%d resourcePatchBytes=%d trusted=%v persona=%v", entry.Skin.SkinImageWidth, entry.Skin.SkinImageHeight, len(entry.Skin.SkinData), len(entry.Skin.SkinGeometry), len(entry.Skin.SkinResourcePatch), entry.Skin.Trusted, entry.Skin.PersonaSkin)
						if os.Getenv("LOBBYKEEPER_TRACE_SKIN_TEST") != "" {
							var geometry map[string]any
							_ = json.Unmarshal(entry.Skin.SkinGeometry, &geometry)
							var descriptions []any
							if models, ok := geometry["minecraft:geometry"].([]any); ok {
								for _, model := range models {
									if object, ok := model.(map[string]any); ok {
										descriptions = append(descriptions, object["description"])
									}
								}
							}
							for name, model := range geometry {
								if object, ok := model.(map[string]any); ok {
									descriptions = append(descriptions, map[string]any{"legacy_model": name, "texture_width": object["texturewidth"], "texture_height": object["textureheight"]})
								}
							}
							description, _ := json.Marshal(descriptions)
							var patch bytes.Buffer
							_ = json.Compact(&patch, entry.Skin.SkinResourcePatch)
							var opaque int
							for i := 3; i < len(entry.Skin.SkinData); i += 4 {
								if entry.Skin.SkinData[i] > 0 {
									opaque++
								}
							}
							t.Logf("skin model: format=%v description=%s patch=%s engine=%s opaquePixels=%d premium=%v override=%v primary=%v", geometry["format_version"], description, patch.Bytes(), entry.Skin.GeometryDataEngineVersion, opaque, entry.Skin.PremiumSkin, entry.Skin.OverrideAppearance, entry.Skin.PrimaryUser)
						}
						for _, override := range skinOverrides {
							if entry.UUID != loginUUID {
								continue
							}
							// BDS relays the downsampled NEMCBridge skin; a NetEase recipient
							// must instead receive the player's original login skin.
							original, _ := override["SkinId"].(string)
							if original == "" {
								original = "KeeperTest"
							}
							if entry.Skin.SkinID != original || strings.HasPrefix(entry.Skin.SkinID, "NEMCBridge_") || entry.Skin.PremiumSkin != (override["PremiumSkin"] == true) || !entry.Skin.Trusted {
								t.Fatal("NetEase recipient did not receive the original login skin identity")
							}
							width := uint32(override["SkinImageWidth"].(int))
							if entry.Skin.SkinImageWidth != width || entry.Skin.SkinImageHeight != width || len(entry.Skin.SkinData) != int(width*width*4) || len(entry.Skin.SkinResourcePatch) == 0 {
								t.Fatal("original skin dimensions/texture/model not restored")
							}
							skinVerified = true
						}
					}
					if !bytes.Equal(buffer.Bytes(), make([]byte, len(list.Entries)*8)) {
						t.Fatal("real BDS player list missing NetEase extra groups")
					}
					playerListed = true
				}
			}
			if id&0x3ff == stdpacket.IDUpdateAbilities {
				buffer := bytes.NewBuffer(pk[n:])
				var abilities stdpacket.UpdateAbilities
				abilities.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if buffer.Len() != 12 || !bytes.Equal(buffer.Bytes(), make([]byte, 12)) || len(abilities.AbilityData.Layers) == 0 {
					t.Fatal("real BDS ability core or NetEase extension invalid")
				}
			}
			if id&0x3ff == stdpacket.IDCraftingData {
				buffer := bytes.NewBuffer(pk[n:])
				var recipes stdpacket.CraftingData
				// Vanilla BDS sends more than the library's defensive 1024
				// recipe limit. This is our known local server test payload.
				reader := stdprotocol.NewReader(buffer, 0, false)
				recipes.Marshal(reader)
				var signed1, signed2 int32
				var unsigned uint32
				var text string
				var flag bool
				reader.Varint32(&signed1)
				reader.Varint32(&signed2)
				reader.Varuint32(&unsigned)
				reader.String(&text)
				reader.Bool(&flag)
				if buffer.Len() != 0 || len(recipes.Recipes) == 0 || signed1 != 0 || signed2 != 0 || unsigned != 0 || text != "" || flag {
					t.Fatal("real BDS recipe core or NetEase extensions invalid")
				}
			}
			if id&0x3ff == 58 {
				buffer := bytes.NewBuffer(pk[n:])
				var chunk stdpacket.LevelChunk
				chunk.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if buffer.Len() != 0 || len(chunk.RawPayload) == 0 {
					t.Fatal("BDS chunk body invalid")
				}
				t.Logf("LevelChunk: cache=%v hashes=%d subchunks=%d payload=%d", chunk.CacheEnabled, len(chunk.BlobHashes), chunk.SubChunkCount, len(chunk.RawPayload))
				chunkProvided = true
				if !subChunkRequested {
					request := &stdpacket.SubChunkRequest{Dimension: chunk.Dimension, Position: stdprotocol.SubChunkPos{chunk.Position[0], 0, chunk.Position[1]}}
					for y := int8(-4); y < 20; y++ {
						request.Offsets = append(request.Offsets, stdprotocol.SubChunkOffset{0, y, 0})
					}
					writeTestGamePacket(t, stream, request)
					subChunkRequested = true
				}
			}
			if id&0x3ff == stdpacket.IDSubChunk {
				buffer := bytes.NewBuffer(pk[n:])
				var sub stdpacket.SubChunk
				sub.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if buffer.Len() != 0 || len(sub.SubChunkEntries) == 0 {
					t.Fatal("invalid terrain subchunks")
				}
				var missing []uint64
				for _, entry := range sub.SubChunkEntries {
					if entry.Result == stdprotocol.SubChunkResultSuccess {
						if sub.CacheEnabled {
							missing = append(missing, entry.BlobHash)
						} else if len(entry.RawPayload) > 0 {
							terrainProvided = true
						}
					}
				}
				if len(missing) > 0 {
					writeTestGamePacket(t, stream, &stdpacket.ClientCacheBlobStatus{MissHashes: missing})
				}
			}
			if id&0x3ff == stdpacket.IDClientCacheMissResponse {
				buffer := bytes.NewBuffer(pk[n:])
				var blobs stdpacket.ClientCacheMissResponse
				blobs.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if buffer.Len() != 0 || len(blobs.Blobs) == 0 {
					t.Fatal("invalid terrain blob response")
				}
				for _, blob := range blobs.Blobs {
					terrainProvided = terrainProvided || len(blob.Payload) > 0
				}
			}
			if id&0x3ff == 5 {
				t.Fatalf("BDS disconnected: %s", relay.DescribeEarlyBatch(p[1:], true))
			}
		}
		if worldLoaded && biomeLoaded && playerListed && chunkProvided && terrainProvided {
			if !skinVerified {
				t.Fatal("BDS never broadcast the test player's skin")
			}
			if chat && !chatSent {
				writeTestGamePacket(t, stream, &stdpacket.SetLocalPlayerAsInitialised{EntityRuntimeID: runtimeID})
				message := &stdpacket.Text{TextType: stdpacket.TextTypeChat, SourceName: loginIdentity["displayName"].(string), Message: chatMessage}
				var body bytes.Buffer
				message.Marshal(stdprotocol.NewWriter(&body, 0))
				wire := append([]byte{9}, body.Bytes()...)
				wire = append(wire, 0)
				if _, err := stream.Write(append([]byte{0xfe, 0xff}, gamebatch.Join([][]byte{wire})...)); err != nil {
					t.Fatal(err)
				}
				chatSent = true
			}
			if chat && !chatReceived {
				continue
			}
			if commands {
				if commandIndex < len(commandLines) && (commandIndex == 0 || commandReceived) {
					commandRequestID = fmt.Sprintf("nemcbridge-command-%d", commandIndex)
					request := &nepacket.CommandRequest{CommandLine: commandLines[commandIndex], CommandOrigin: neprotocol.CommandOrigin{Origin: neprotocol.CommandOriginPlayer, UUID: loginUUID, RequestID: commandRequestID}, Version: 42, UnLimited: commandIndex%2 == 1}
					var body bytes.Buffer
					request.Marshal(neprotocol.NewWriter(&body, 0))
					wire := append([]byte{byte(stdpacket.IDCommandRequest)}, body.Bytes()...)
					if _, err := stream.Write(append([]byte{0xfe, 0xff}, gamebatch.Join([][]byte{wire})...)); err != nil {
						t.Fatal(err)
					}
					commandIndex++
					commandReceived = false
				}
				if commandIndex < len(commandLines) || !commandReceived {
					continue
				}
			}
			t.Logf("real BDS initialization and terrain subchunk delivery passed: cache=%v WebRTC=%v", cache, webRTC)
			return
		}
	}
	t.Fatal("BDS did not confirm login")
}

func writeTestGamePacket(t *testing.T, stream relay.Stream, p stdpacket.Packet) {
	t.Helper()
	var encoded bytes.Buffer
	p.Marshal(stdprotocol.NewWriter(&encoded, 0))
	wire := binary.AppendUvarint(nil, uint64(p.ID()))
	wire = append(wire, encoded.Bytes()...)
	if _, err := stream.Write(append([]byte{0xfe, 0xff}, gamebatch.Join([][]byte{wire})...)); err != nil {
		t.Fatal(err)
	}
}
