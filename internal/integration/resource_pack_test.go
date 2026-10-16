package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	bdsraknet "github.com/sandertv/go-raknet"
	stdprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	stdpacket "github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/bridge"
	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/netease"
	"nemcbridge/internal/relay"
)

func TestBDSResourcePackSkippedForNetEase(t *testing.T) {
	testBDSResourcePackExchange(t, false, false)
}

func TestWebRTCResourcePackSkippedForNetEase(t *testing.T) {
	testBDSResourcePackExchange(t, true, false)
}

func TestBDSResourcePackDownloadForNetEase(t *testing.T) {
	testBDSResourcePackExchange(t, false, true)
}

func TestWebRTCResourcePackDownloadForNetEase(t *testing.T) {
	testBDSResourcePackExchange(t, true, true)
}

func TestBDSAutomaticResourcePackDownloadForNetEase(t *testing.T) {
	testBDSResourcePackScenario(t, false, true, nil, true)
}

func TestWebRTCAutomaticResourcePackDownloadForNetEase(t *testing.T) {
	testBDSResourcePackScenario(t, true, true, nil, true)
}

func TestNativeResourceProfileWithBDSAndWebRTC(t *testing.T) {
	path := os.Getenv("NEMCBRIDGE_NATIVE_RP_PROFILE")
	if path == "" {
		t.Skip("requires an imported private native resource profile")
	}
	p, err := netease.LoadResourceProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, webRTC := range []bool{false, true} {
		t.Run(map[bool]string{false: "BDS", true: "WebRTC"}[webRTC], func(t *testing.T) { testBDSResourcePackExchange(t, webRTC, false, p) })
	}
}

func TestNativeResourceProfileWithBackendDownloads(t *testing.T) {
	path := os.Getenv("NEMCBRIDGE_NATIVE_RP_PROFILE")
	if path == "" {
		t.Skip("requires an imported private native resource profile")
	}
	p, err := netease.LoadResourceProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, webRTC := range []bool{false, true} {
		t.Run(map[bool]string{false: "BDS", true: "WebRTC"}[webRTC], func(t *testing.T) {
			testBDSResourcePackScenario(t, webRTC, true, p, true)
		})
	}
}

func TestBuiltinClassicProfileWithBackendDownloads(t *testing.T) {
	p, err := netease.BuiltinResourceProfile()
	if err != nil {
		t.Fatal(err)
	}
	for _, webRTC := range []bool{false, true} {
		t.Run(map[bool]string{false: "BDS", true: "WebRTC"}[webRTC], func(t *testing.T) {
			testBDSResourcePackScenario(t, webRTC, true, p, true)
		})
	}
}

// Requires a BDS whose world requires at least one resource pack, such as
// runtime/bedrock-rp-test (LOBBYKEEPER_BDS_RP_TEST=127.0.0.1:19142). Reproduces
// the NetEase client: it answers "have all packs" without downloading.
func testBDSResourcePackExchange(t *testing.T, webRTC, forward bool, native ...*netease.ResourceProfile) {
	var profile *netease.ResourceProfile
	if len(native) > 0 {
		profile = native[0]
	}
	testBDSResourcePackScenario(t, webRTC, forward, profile, false)
}

// automatic reproduces native iOS: it initially answers 3, subsequently asks for
// DataInfo's chunks, and never repeats response 3 after the download finishes.
func testBDSResourcePackScenario(t *testing.T, webRTC, forward bool, profile *netease.ResourceProfile, automatic bool) {
	t.Helper()
	addr := os.Getenv("LOBBYKEEPER_BDS_RP_TEST")
	if addr == "" {
		t.Skip("requires local BDS with a required world resource pack")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	server, err := bdsraknet.DialContext(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	context.AfterFunc(ctx, func() { server.Close() })
	relayBridge, err := bridge.NewWithOptions(server, func(s string) { t.Log(s) }, bridge.Options{ForwardResourcePacks: forward, ResourceProfile: profile})
	if err != nil {
		t.Fatal(err)
	}
	defer relayBridge.ForgetSkin()
	defer relayBridge.Close()
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
	if _, err = stream.Write(makeOfflineTestLogin(t)); err != nil {
		t.Fatal(err)
	}
	offered, stacked := false, false
	type download struct {
		info stdpacket.ResourcePackDataInfo
		data []byte
		next uint32
	}
	downloads := map[string]*download{}
	var nativeInfo stdpacket.ResourcePacksInfo
	var nativeStack stdpacket.ResourcePackStack
	nativeTail := []byte{0, 0}
	if profile != nil {
		_, n := binary.Uvarint(profile.Info)
		nativeInfo.Marshal(stdprotocol.NewReader(bytes.NewBuffer(profile.Info[n:]), 0, true))
		_, n = binary.Uvarint(profile.Stack)
		buffer := bytes.NewBuffer(profile.Stack[n:])
		nativeStack.Marshal(stdprotocol.NewReader(buffer, 0, true))
		nativeTail = buffer.Bytes()
	}
	var expectedPacks, downloadedPacks int
	for i := 0; i < 256; i++ {
		p, err := stream.ReadPacket()
		if err != nil {
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
			buffer := bytes.NewBuffer(pk[n:])
			reader := stdprotocol.NewReader(buffer, 0, true)
			switch id & 0x3ff {
			case stdpacket.IDResourcePacksInfo:
				if profile != nil && !forward {
					_, m := binary.Uvarint(profile.Info)
					if !bytes.Equal(pk[n:], profile.Info[m:]) {
						t.Fatal("native offer metadata changed")
					}
					offered = true
					writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseAllPacksDownloaded})
					continue
				}
				var info stdpacket.ResourcePacksInfo
				info.Marshal(reader)
				if buffer.Len() != 0 || !forward && (len(info.TexturePacks) != 0 || info.TexturePackRequired) {
					t.Fatal("NetEase player was still offered server packs")
				}
				offered = true
				if forward {
					nativeCount := len(nativeInfo.TexturePacks)
					if len(info.TexturePacks) <= nativeCount || !info.TexturePackRequired {
						t.Fatal("fixture must require a resource pack")
					}
					if nativeCount > 0 && !reflect.DeepEqual(info.TexturePacks[:nativeCount], nativeInfo.TexturePacks) {
						t.Fatal("native preload metadata changed while merging backend offers")
					}
					expectedPacks = len(info.TexturePacks) - nativeCount
					var names []string
					for _, pack := range info.TexturePacks[nativeCount:] {
						names = append(names, pack.UUID.String()+"_"+pack.Version)
					}
					if automatic {
						writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseAllPacksDownloaded})
					} else {
						writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseSendPacks, PacksToDownload: names})
					}
				} else {
					writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseAllPacksDownloaded})
				}
			case stdpacket.IDResourcePackDataInfo:
				var info stdpacket.ResourcePackDataInfo
				info.Marshal(reader)
				if !forward || buffer.Len() != 0 || info.Size == 0 || info.Size > 32<<20 || info.DataChunkSize == 0 || len(info.Hash) != sha256.Size {
					t.Fatal("invalid resource download metadata")
				}
				downloads[info.UUID] = &download{info: info, data: make([]byte, 0, int(info.Size))}
				writeTestGamePacket(t, stream, &stdpacket.ResourcePackChunkRequest{UUID: info.UUID, ChunkIndex: 0})
			case stdpacket.IDResourcePackChunkData:
				var chunk stdpacket.ResourcePackChunkData
				chunk.Marshal(reader)
				d := downloads[chunk.UUID]
				if d == nil || buffer.Len() != 0 || chunk.ChunkIndex != d.next || chunk.DataOffset != uint64(len(d.data)) || len(chunk.Data) == 0 || uint64(len(d.data)+len(chunk.Data)) > d.info.Size {
					t.Fatal("resource chunk ordering/size corrupted")
				}
				d.data = append(d.data, chunk.Data...)
				d.next++
				if uint64(len(d.data)) == d.info.Size {
					sum := sha256.Sum256(d.data)
					if !bytes.Equal(sum[:], d.info.Hash) {
						t.Fatal("downloaded resource SHA256 mismatch")
					}
					archive, err := zip.NewReader(bytes.NewReader(d.data), int64(len(d.data)))
					if err != nil {
						t.Fatal(err)
					}
					manifestFound := false
					textureFound := false
					for _, file := range archive.File {
						if strings.HasSuffix(file.Name, "manifest.json") {
							input, err := file.Open()
							if err != nil {
								t.Fatal(err)
							}
							raw, err := io.ReadAll(io.LimitReader(input, 1<<20))
							input.Close()
							if err != nil || !json.Valid(raw) {
								t.Fatal("downloaded manifest invalid", err)
							}
							manifestFound = true
						}
						if strings.HasSuffix(file.Name, "textures/blocks/stone.png") {
							textureFound = true
						}
					}
					if !manifestFound || !textureFound {
						t.Fatal("download did not contain diagnostic texture pack")
					}
					downloadedPacks++
					delete(downloads, chunk.UUID)
					t.Logf("resource pack verified: bytes=%d chunks=%d WebRTC=%v", len(d.data), d.next, webRTC)
					if downloadedPacks == expectedPacks && !automatic {
						writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseAllPacksDownloaded})
					}
				} else {
					writeTestGamePacket(t, stream, &stdpacket.ResourcePackChunkRequest{UUID: chunk.UUID, ChunkIndex: d.next})
				}
			case stdpacket.IDResourcePackStack:
				if profile != nil && !forward {
					_, m := binary.Uvarint(profile.Stack)
					if !bytes.Equal(pk[n:], profile.Stack[m:]) {
						t.Fatal("native stack or additional arrays changed")
					}
					stacked = true
					writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseCompleted})
					continue
				}
				var stack stdpacket.ResourcePackStack
				stack.Marshal(reader)
				if !bytes.Equal(buffer.Bytes(), nativeTail) || !forward && len(stack.TexturePacks)+len(stack.BehaviourPacks) != 0 {
					t.Fatal("NetEase stack still references server packs")
				}
				if forward && (len(stack.TexturePacks) != expectedPacks+len(nativeStack.TexturePacks) || downloadedPacks != expectedPacks || !stack.TexturePackRequired) {
					t.Fatal("stack arrived without verified packs")
				}
				if profile != nil && (!reflect.DeepEqual(stack.TexturePacks[:len(nativeStack.TexturePacks)], nativeStack.TexturePacks) || !reflect.DeepEqual(stack.BehaviourPacks, nativeStack.BehaviourPacks) || stack.BaseGameVersion != nativeStack.BaseGameVersion) {
					t.Fatal("native stack context changed while merging backend stack")
				}
				stacked = true
				writeTestGamePacket(t, stream, &stdpacket.ResourcePackClientResponse{Response: stdpacket.PackResponseCompleted})
			case stdpacket.IDStartGame:
				if !offered || !stacked {
					t.Fatal("world started without the pack negotiation")
				}
				t.Logf("BDS sent StartGame after resource negotiation: forwarding=%v downloaded=%d", forward, downloadedPacks)
				return
			}
		}
	}
	t.Fatal("pack negotiation did not reach StartGame")
}
