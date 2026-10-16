package bridge

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"nemcbridge/internal/gamebatch"
)

func resourceBatch(packets ...packet.Packet) []byte {
	var parts [][]byte
	for _, p := range packets {
		parts = append(parts, writeResourcePacket(uint64(p.ID()), p))
	}
	return append([]byte{0xff}, gamebatch.Join(parts)...)
}

func resourceOffer() *packet.ResourcePacksInfo {
	return &packet.ResourcePacksInfo{TexturePackRequired: true, TexturePacks: []protocol.TexturePackInfo{
		{UUID: uuid.MustParse("678ec3c1-e663-4c35-9e15-b579c24bb763"), Version: "1.2.3"},
		{UUID: uuid.MustParse("a345cb27-d2cc-4691-bb08-deab5e9e6516"), Version: "2.0.0"},
	}}
}

func prepareResourceDownloads(t *testing.T) (*resourceDownloads, <-chan []byte, []string) {
	t.Helper()
	sent := make(chan []byte, 2)
	r := newResourceDownloads(nil, func(pk []byte) error { sent <- pk; return nil })
	r.delay = 10 * time.Millisecond
	t.Cleanup(r.close)
	info := resourceOffer()
	if err := r.server(resourceBatch(info)); err != nil {
		t.Fatal(err)
	}
	header := uint64(packet.IDResourcePackClientResponse) | 1<<10 | 2<<12
	original := writeResourcePacket(header, &packet.ResourcePackClientResponse{Response: packet.PackResponseAllPacksDownloaded})
	batch := append([]byte{0xff}, gamebatch.Join([][]byte{original})...)
	converted, err := r.client(batch)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := gamebatch.Decode(converted)
	if err != nil {
		t.Fatal(err)
	}
	parts, err := gamebatch.Split(raw)
	if err != nil || len(parts) != 1 {
		t.Fatal("download request batch", err)
	}
	gotHeader, _ := binary.Uvarint(parts[0])
	var request packet.ResourcePackClientResponse
	if err := readResourcePacket(parts[0], &request); err != nil {
		t.Fatal(err)
	}
	if gotHeader != header || request.Response != packet.PackResponseSendPacks || len(request.PacksToDownload) != 2 || request.PacksToDownload[0] != info.TexturePacks[0].UUID.String()+"_1.2.3" || request.PacksToDownload[1] != info.TexturePacks[1].UUID.String()+"_2.0.0" {
		t.Fatal("backend offer identities or subclient header lost")
	}
	return r, sent, request.PacksToDownload
}

func resourceMetadata(name string, size uint64) *packet.ResourcePackDataInfo {
	return &packet.ResourcePackDataInfo{UUID: name, Size: size, DataChunkSize: 4, ChunkCount: uint32((size + 3) / 4), Hash: make([]byte, 32), PackType: 6}
}

func TestAutomaticDownloadRequestsRepeatedDependenciesOnlyOnce(t *testing.T) {
	r := newResourceDownloads(nil, func([]byte) error { t.Error("unexpected completion"); return nil })
	defer r.close()
	info := resourceOffer()
	info.TexturePacks = append(info.TexturePacks, info.TexturePacks...)
	if err := r.server(resourceBatch(info)); err != nil {
		t.Fatal(err)
	}
	response, err := r.client(resourceBatch(&packet.ResourcePackClientResponse{Response: packet.PackResponseAllPacksDownloaded}))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := gamebatch.Decode(response)
	if err != nil {
		t.Fatal(err)
	}
	parts, err := gamebatch.Split(raw)
	if err != nil || len(parts) != 1 {
		t.Fatal("download request batch", err)
	}
	var request packet.ResourcePackClientResponse
	if err := readResourcePacket(parts[0], &request); err != nil || request.Response != packet.PackResponseSendPacks || len(request.PacksToDownload) != 2 || len(r.packs) != 2 {
		t.Fatal("duplicate dependency created duplicate download", err)
	}
}

func resourceChunk(name string, index uint32, size int) *packet.ResourcePackChunkData {
	return &packet.ResourcePackChunkData{UUID: name, ChunkIndex: index, DataOffset: uint64(index) * 4, Data: make([]byte, size)}
}

func TestAutomaticResourceDownloadWaitsForEveryPackAndChunk(t *testing.T) {
	r, sent, names := prepareResourceDownloads(t)
	if err := r.server(resourceBatch(resourceMetadata(names[0], 9), resourceMetadata(names[1], 4),
		resourceChunk(names[0], 2, 1), resourceChunk(names[0], 0, 4), resourceChunk(names[0], 2, 1), resourceChunk(names[1], 0, 4))); err != nil {
		t.Fatal(err)
	}
	r.forwarded()
	select {
	case <-sent:
		t.Fatal("duplicate/out-of-order chunks caused premature completion")
	case <-time.After(30 * time.Millisecond):
	}
	if err := r.server(resourceBatch(resourceChunk(names[0], 1, 4))); err != nil {
		t.Fatal(err)
	}
	// Merely reading the final server frame must not start the completion timer.
	select {
	case <-sent:
		t.Fatal("completion before frontend forwarding")
	case <-time.After(30 * time.Millisecond):
	}
	r.forwarded()
	r.forwarded()
	select {
	case pk := <-sent:
		var response packet.ResourcePackClientResponse
		if err := readResourcePacket(pk, &response); err != nil || response.Response != packet.PackResponseAllPacksDownloaded {
			t.Fatal("original completion not restored", err)
		}
		header, _ := binary.Uvarint(pk)
		if header != uint64(packet.IDResourcePackClientResponse)|1<<10|2<<12 {
			t.Fatal("completion subclient header changed")
		}
	case <-time.After(time.Second):
		t.Fatal("completion missing")
	}
	select {
	case <-sent:
		t.Fatal("completion sent twice")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestAutomaticResourceDownloadCancelsOnDisconnectAndRefusal(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect", true: "refusal"}[refuse], func(t *testing.T) {
			r, sent, names := prepareResourceDownloads(t)
			r.delay = 50 * time.Millisecond
			for _, name := range names {
				if err := r.server(resourceBatch(resourceMetadata(name, 4), resourceChunk(name, 0, 4))); err != nil {
					t.Fatal(err)
				}
			}
			r.forwarded()
			if refuse {
				batch := resourceBatch(&packet.ResourcePackClientResponse{Response: packet.PackResponseRefused})
				out, err := r.client(batch)
				if err != nil || !bytes.Equal(out, batch) {
					t.Fatal("refusal must reach BDS unchanged", err)
				}
			} else {
				r.close()
			}
			select {
			case <-sent:
				t.Fatal("completion sent after cancellation")
			case <-time.After(80 * time.Millisecond):
			}
		})
	}
}

func TestOrdinaryResourceDownloadAndEmptyOfferRemainUnchanged(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit-download", true: "empty-offer"}[empty], func(t *testing.T) {
			r := newResourceDownloads(nil, func([]byte) error { t.Error("unexpected generated completion"); return nil })
			defer r.close()
			info := resourceOffer()
			response := &packet.ResourcePackClientResponse{Response: packet.PackResponseSendPacks, PacksToDownload: []string{info.TexturePacks[0].UUID.String() + "_1.2.3"}}
			if empty {
				info.TexturePacks = nil
				response = &packet.ResourcePackClientResponse{Response: packet.PackResponseAllPacksDownloaded}
			}
			if err := r.server(resourceBatch(info)); err != nil {
				t.Fatal(err)
			}
			batch := resourceBatch(response)
			out, err := r.client(batch)
			if err != nil || !bytes.Equal(out, batch) {
				t.Fatal("ordinary negotiation changed", err)
			}
			r.forwarded()
		})
	}
}

func TestAutomaticResourceDownloadRejectsIncompleteOrInvalidTransfer(t *testing.T) {
	for _, kind := range []string{"missing-info", "bad-offset", "bad-size", "bad-count"} {
		t.Run(kind, func(t *testing.T) {
			r, _, names := prepareResourceDownloads(t)
			meta := resourceMetadata(names[0], 9)
			chunk := resourceChunk(names[0], 0, 4)
			if kind == "bad-count" {
				meta.ChunkCount++
				if err := r.server(resourceBatch(meta)); err == nil {
					t.Fatal("invalid metadata accepted")
				}
				return
			}
			if kind != "missing-info" {
				if err := r.server(resourceBatch(meta)); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "bad-offset" {
				chunk.DataOffset++
			}
			if kind == "bad-size" {
				chunk.Data = chunk.Data[:3]
			}
			if err := r.server(resourceBatch(chunk)); err == nil {
				t.Fatal("invalid transfer accepted")
			}
		})
	}
}

func TestEarlyResourceResponseDoesNotDropOtherCompressedPackets(t *testing.T) {
	r, _, _ := prepareResourceDownloads(t)
	cache := &packet.ClientCacheStatus{Enabled: true}
	plain, err := gamebatch.Decode(resourceBatch(&packet.ResourcePackClientResponse{Response: packet.PackResponseAllPacksDownloaded}, cache))
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := gamebatch.Encode([]byte{0}, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.client(compressed)
	if err != nil {
		t.Fatal(err)
	}
	plain, err = gamebatch.Decode(got)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := gamebatch.Decode(resourceBatch(cache))
	if got[0] != 0 || !bytes.Equal(plain, want) {
		t.Fatal("suppressing an early resource response lost another packet or compression")
	}
}
