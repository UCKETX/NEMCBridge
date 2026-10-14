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
	"github.com/google/uuid"
	bdsraknet "github.com/sandertv/go-raknet"
	stdprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	stdpacket "github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"os"
	"testing"
	"time"
)

func makeOfflineTestLogin(t *testing.T) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	identityID := uuid.NewString()
	publicRaw, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	pub, _ := signGameJWT(key, map[string]any{"extraData": map[string]any{"identity": identityID, "displayName": "KeeperTest" + identityID[:6], "XUID": ""}, "identityPublicKey": base64.StdEncoding.EncodeToString(publicRaw)})
	chain, _ := json.Marshal(map[string]any{"chain": []string{pub}})
	envelope, _ := json.Marshal(map[string]any{"Certificate": string(chain), "AuthenticationType": 2, "Token": ""})
	skin, _ := signGameJWT(key, map[string]any{
		"ClientRandomId": time.Now().UnixNano(), "CurrentInputMode": 1, "DefaultInputMode": 1, "DeviceOS": 7, "DeviceModel": "Windows", "DeviceId": uuid.NewString(), "GameVersion": "1.21.124", "LanguageCode": "en_US", "SelfSignedId": uuid.NewString(), "ServerAddress": "127.0.0.1:19132", "SkinId": "KeeperTest", "SkinData": base64.StdEncoding.EncodeToString(make([]byte, 64*64*4)), "SkinImageHeight": 64, "SkinImageWidth": 64, "SkinResourcePatch": base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)), "SkinGeometryData": base64.StdEncoding.EncodeToString([]byte(`{"format_version":"1.12.0","minecraft:geometry":[]}`)), "SkinGeometryDataEngineVersion": "1.21.124", "AnimatedImageData": []any{}, "PersonaPieces": []any{}, "PieceTintColors": []any{}, "PersonaSkin": false, "PremiumSkin": false, "TrustedSkin": true, "PlayFabId": "", "CapeData": "", "CapeImageHeight": 0, "CapeImageWidth": 0,
	})
	request := binary.LittleEndian.AppendUint32(nil, uint32(len(envelope)))
	request = append(request, envelope...)
	request = binary.LittleEndian.AppendUint32(request, uint32(len(skin)))
	request = append(request, skin...)
	pk := []byte{1, 0, 0, 3, 0x5c}
	pk = binary.AppendUvarint(pk, uint64(len(request)))
	pk = append(pk, request...)
	return append([]byte{0xfe, 0xff}, joinGameBatch([][]byte{pk})...)
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

func testBDSWorldExchange(t *testing.T, cache, webRTC bool) {
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
	bridge, err := newBedrockBridge(server, func(s string) { t.Log(s) })
	if err != nil {
		t.Fatal(err)
	}
	var stream packetStream = bridge
	if webRTC {
		stream = localWebRTCFrontend(t, ctx, bridge)
	}
	if _, err = stream.Write([]byte{0xfe, 6, 0xc1, 1, 0, 0, 3, 0x5c}); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.ReadPacket(); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Write(makeOfflineTestLogin(t)); err != nil {
		t.Fatal(err)
	}
	cacheFlag := byte(0)
	if cache {
		cacheFlag = 1
	}
	loggedIn := false
	worldLoaded := false
	biomeLoaded, playerListed, chunkProvided := false, false, false
	subChunkRequested, terrainProvided := false, false
	seenIDs := map[uint64]bool{}
	for i := 0; i < 512; i++ {
		p, err := stream.ReadPacket()
		if err != nil {
			t.Fatal(err)
		}
		plain, err := decodeGameCompression(p[1:])
		if err != nil {
			t.Fatal(err)
		}
		packets, err := splitGameBatch(plain)
		if err != nil {
			t.Fatal(err)
		}
		for _, pk := range packets {
			id, n := binary.Uvarint(pk)
			if os.Getenv("LOBBYKEEPER_TRACE_WORLD_TEST") != "" && !seenIDs[id&0x3ff] {
				t.Logf("world packet id=%d bytes=%d batch=%d wire=%d cache=%v", id&0x3ff, len(pk), i, len(p), cache)
			}
			seenIDs[id&0x3ff] = true
			if id&0x3ff == 3 {
				t.Fatal("RakNet encryption handshake leaked to NetherNet")
			}
			if id&0x3ff == 2 && len(pk) >= n+4 && binary.BigEndian.Uint32(pk[n:]) == 0 {
				t.Log("BDS accepted rewritten login and encrypted ClientToServerHandshake; plaintext PlayStatus=0 returned")
				loggedIn = true
				if _, err = stream.Write(append([]byte{0xfe, 0xff}, joinGameBatch([][]byte{{0x81, 1, cacheFlag}})...)); err != nil {
					t.Fatal(err)
				}
			}
			if id&0x3ff == 6 || id&0x3ff == 7 {
				response := byte(3)
				if id&0x3ff == 7 {
					response = 4
				}
				if _, err = stream.Write(append([]byte{0xfe, 0xff}, joinGameBatch([][]byte{{8, response, 0, 0}})...)); err != nil {
					t.Fatal(err)
				}
			}
			if id&0x3ff == 11 {
				if capture := os.Getenv("LOBBYKEEPER_CAPTURE_STARTGAME"); capture != "" {
					if err := os.WriteFile(capture, pk[:len(pk)-neteaseWorldExtensionSize], 0600); err != nil {
						t.Fatal(err)
					}
				}
				if !loggedIn {
					t.Fatal("world settings were not converted")
				}
				if len(pk) < 28+neteaseWorldExtensionSize || !bytes.Equal(pk[len(pk)-neteaseWorldExtensionSize:], make([]byte, neteaseWorldExtensionSize)) {
					t.Fatal("real BDS packet did not gain the NetEase world extension")
				}
				tail := pk[len(pk)-28-neteaseWorldExtensionSize : len(pk)-neteaseWorldExtensionSize]
				if binary.LittleEndian.Uint64(tail) != 0 || tail[24] != 0 || tail[25] != 1 {
					t.Fatal("real BDS world compatibility fields incorrect")
				}
				buffer := bytes.NewBuffer(pk[n:])
				var world stdpacket.StartGame
				world.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if buffer.Len() != neteaseWorldExtensionSize || world.ServerBlockStateChecksum != 0 || world.ClientSideGeneration || !world.UseBlockNetworkIDHashes {
					t.Fatal("independent protocol 860 reader rejected converted common StartGame layout")
				}
				t.Log("real BDS resource-pack exchange and independent StartGame 860 decoding passed")
				worldLoaded = true
			}
			if id&0x3ff == 122 {
				if !worldLoaded {
					t.Fatal("BDS world/biome conversion did not complete")
				}
				verifyNetEaseBiomePayload(t, pk[n:])
				t.Log("real BDS biome conversion and NetEase extension positions passed")
				biomeLoaded = true
				if _, err := stream.Write(append([]byte{0xfe, 0xff}, joinGameBatch([][]byte{{69, 16, 8}})...)); err != nil {
					t.Fatal(err)
				}
			}
			if id&0x3ff == 63 {
				buffer := bytes.NewBuffer(pk[n:])
				var list stdpacket.PlayerList
				list.Marshal(stdprotocol.NewReader(buffer, 0, true))
				if list.ActionType == 0 && len(list.Entries) > 0 {
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
				t.Fatalf("BDS disconnected: %s", describeEarlyBatch(p[1:], true))
			}
		}
		if worldLoaded && biomeLoaded && playerListed && chunkProvided && terrainProvided {
			t.Logf("real BDS initialization and terrain subchunk delivery passed: cache=%v WebRTC=%v", cache, webRTC)
			return
		}
	}
	t.Fatal("BDS did not confirm login")
}

func writeTestGamePacket(t *testing.T, stream packetStream, p stdpacket.Packet) {
	t.Helper()
	var encoded bytes.Buffer
	p.Marshal(stdprotocol.NewWriter(&encoded, 0))
	wire := binary.AppendUvarint(nil, uint64(p.ID()))
	wire = append(wire, encoded.Bytes()...)
	if _, err := stream.Write(append([]byte{0xfe, 0xff}, joinGameBatch([][]byte{wire})...)); err != nil {
		t.Fatal(err)
	}
}

func TestGameCipherCountersAndIntegrity(t *testing.T) {
	key := [32]byte{1, 2, 3}
	send, recv := newGameCipher(key), newGameCipher(key)
	for _, plain := range [][]byte{{0xff, 1, 4}, bytes.Repeat([]byte{7}, 17000), {0xff, 2, 3, 4}} {
		encoded := send.encrypt(plain)
		decoded, err := recv.decrypt(encoded)
		if err != nil || !bytes.Equal(plain, decoded) {
			t.Fatal("AES-CTR counter or checksum failed", err)
		}
	}
	bad := send.encrypt([]byte{9})
	bad[0] ^= 1
	if _, err := recv.decrypt(bad); err == nil {
		t.Fatal("corrupt batch accepted")
	}
}
