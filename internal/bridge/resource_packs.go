package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/netease"
)

// Native iOS can answer 3 before downloading an unfamiliar pack, but accepts
// DataInfo and requests its chunks. Hold that original response until every
// requested pack has been forwarded, then resume BDS's ordinary stack exchange.
// This state belongs to one player, never to a room or a shared pack cache.
type resourceDownloads struct {
	mu      sync.Mutex
	trace   func(string)
	send    func([]byte) error
	delay   time.Duration
	timer   *time.Timer
	closed  bool
	decided bool
	forced  bool
	acked   bool
	stacked bool
	offered []string
	resume  []byte
	packs   map[string]*resourceDownload
}

type resourceDownload struct {
	info     *packet.ResourcePackDataInfo
	seen     []uint64
	received uint32
}

func newResourceDownloads(trace func(string), send func([]byte) error) *resourceDownloads {
	return &resourceDownloads{trace: trace, send: send, delay: time.Second}
}

func readResourcePacket(pk []byte, p packet.Packet) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("资源包协商数据格式无效")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 {
		return errors.New("资源包协商包头无效")
	}
	buffer := bytes.NewBuffer(pk[n:])
	p.Marshal(protocol.NewReader(buffer, 0, true))
	if buffer.Len() != 0 {
		return errors.New("资源包协商含未知字段")
	}
	return nil
}

func writeResourcePacket(header uint64, p packet.Packet) []byte {
	var body bytes.Buffer
	p.Marshal(protocol.NewWriter(&body, 0))
	return append(binary.AppendUvarint(nil, header), body.Bytes()...)
}

func (r *resourceDownloads) client(batch []byte) ([]byte, error) {
	r.mu.Lock()
	inactive := r.closed || r.stacked || r.decided && !r.forced
	r.mu.Unlock()
	if inactive {
		return batch, nil
	}
	raw, err := gamebatch.Decode(batch)
	if err != nil {
		return nil, err
	}
	parts, err := gamebatch.Split(raw)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := false
	kept := parts[:0]
	for _, pk := range parts {
		header, n := binary.Uvarint(pk)
		if n <= 0 || header&0x3ff != packet.IDResourcePackClientResponse {
			kept = append(kept, pk)
			continue
		}
		var response packet.ResourcePackClientResponse
		if err := readResourcePacket(pk, &response); err != nil {
			return nil, err
		}
		switch response.Response {
		case packet.PackResponseRefused:
			r.closeLocked()
		case packet.PackResponseSendPacks:
			// A client that implements ordinary negotiation owns its completion.
			r.decided = true
		case packet.PackResponseAllPacksDownloaded:
			if !r.closed && !r.decided {
				r.decided = true
				if len(r.offered) > 0 {
					r.forced = true
					r.resume = append([]byte(nil), pk...)
					r.packs = make(map[string]*resourceDownload, len(r.offered))
					for _, name := range r.offered {
						r.packs[name] = &resourceDownload{}
					}
					response.Response = packet.PackResponseSendPacks
					response.PacksToDownload = append([]string(nil), r.offered...)
					pk = writeResourcePacket(header, &response)
					changed = true
					if r.trace != nil {
						r.trace(fmt.Sprintf("资源包兼容下载：客户端跳过下载，向后端请求 %d 个资源包", len(r.offered)))
					}
					break
				}
			}
			if r.forced {
				if !r.acked && r.completeLocked() {
					// An explicit native completion is stronger than the fallback.
					r.acked = true
					r.resume = nil
					if r.timer != nil {
						r.timer.Stop()
					}
				} else {
					changed = true
					continue // Do not advance BDS on an early/duplicate response 3.
				}
			}
		case packet.PackResponseCompleted:
			if r.forced && !r.stacked {
				return nil, errors.New("资源包尚未加载，客户端提前结束协商")
			}
		}
		kept = append(kept, pk)
	}
	if !changed {
		return batch, nil
	}
	if len(kept) == 0 {
		return nil, nil
	}
	return gamebatch.Encode(batch, gamebatch.Join(kept))
}

func (r *resourceDownloads) server(batch []byte) error {
	r.mu.Lock()
	inactive := r.closed || r.stacked || r.decided && !r.forced
	r.mu.Unlock()
	if inactive {
		return nil
	}
	raw, err := gamebatch.Decode(batch)
	if err != nil {
		return err
	}
	parts, err := gamebatch.Split(raw)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, pk := range parts {
		id, n := binary.Uvarint(pk)
		if n <= 0 || r.closed {
			continue
		}
		switch id & 0x3ff {
		case packet.IDResourcePacksInfo:
			var info packet.ResourcePacksInfo
			if err := readResourcePacket(pk, &info); err != nil {
				return err
			}
			if r.decided {
				continue
			}
			if len(info.TexturePacks) > 128 {
				return errors.New("后端资源包超过 128 个")
			}
			unique, err := netease.DeduplicateTexturePacks(info.TexturePacks)
			if err != nil {
				return err
			}
			r.offered = nil
			for _, p := range unique {
				name := p.UUID.String() + "_" + p.Version
				if len(p.Version) == 0 || len(p.Version) > 64 {
					return errors.New("后端资源包版本无效")
				}
				r.offered = append(r.offered, name)
			}
		case packet.IDResourcePackDataInfo:
			if !r.forced {
				continue
			}
			var info packet.ResourcePackDataInfo
			if err := readResourcePacket(pk, &info); err != nil {
				return err
			}
			p := r.packs[info.UUID]
			if p == nil || p.info != nil || info.Size == 0 || info.Size > 1<<30 || info.DataChunkSize == 0 || info.DataChunkSize > 4<<20 || info.ChunkCount > 1<<20 || uint64(info.ChunkCount) != (info.Size-1)/uint64(info.DataChunkSize)+1 || len(info.Hash) != sha256.Size {
				return errors.New("后端资源包分片参数无效")
			}
			p.info = &info
			p.seen = make([]uint64, (info.ChunkCount+63)/64)
		case packet.IDResourcePackChunkData:
			if !r.forced {
				continue
			}
			var chunk packet.ResourcePackChunkData
			if err := readResourcePacket(pk, &chunk); err != nil {
				return err
			}
			p := r.packs[chunk.UUID]
			if p == nil || p.info == nil || chunk.ChunkIndex >= p.info.ChunkCount {
				return errors.New("收到未请求的资源包分片")
			}
			offset := uint64(chunk.ChunkIndex) * uint64(p.info.DataChunkSize)
			want := min(uint64(p.info.DataChunkSize), p.info.Size-offset)
			if chunk.DataOffset != offset || uint64(len(chunk.Data)) != want {
				return errors.New("资源包分片大小或偏移无效")
			}
			word, mask := chunk.ChunkIndex/64, uint64(1)<<(chunk.ChunkIndex%64)
			if p.seen[word]&mask == 0 {
				p.seen[word] |= mask
				p.received++
			}
		case packet.IDResourcePackStack:
			if r.forced && !r.acked {
				return errors.New("后端提前发送资源包加载顺序")
			}
			r.stacked = true
			r.packs = nil
		}
	}
	return nil
}

func (r *resourceDownloads) completeLocked() bool {
	if len(r.packs) == 0 {
		return false
	}
	for _, p := range r.packs {
		if p.info == nil || p.received != p.info.ChunkCount {
			return false
		}
	}
	return true
}

// ReadPacket's consumer forwards each returned batch before asking for the next.
// Start the grace period here, after the last chunk was queued on the reliable
// frontend stream. Native pack extraction/cache registration runs asynchronously;
// an explicit response 3 cancels this fallback. It is not a render confirmation.
func (r *resourceDownloads) forwarded() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || !r.forced || r.acked || r.timer != nil || !r.completeLocked() {
		return
	}
	r.timer = time.AfterFunc(r.delay, func() {
		r.mu.Lock()
		if r.closed || r.acked {
			r.mu.Unlock()
			return
		}
		r.acked = true
		pk := r.resume
		r.resume = nil
		r.mu.Unlock()
		if err := r.send(pk); err != nil {
			if r.trace != nil {
				r.trace("资源包兼容下载：恢复完成响应失败")
			}
		} else if r.trace != nil {
			r.trace("资源包兼容下载：全部分片已转发，恢复完成响应并等待资源包加载")
		}
	})
}

func (r *resourceDownloads) closeLocked() {
	r.closed = true
	r.resume, r.packs = nil, nil
	if r.timer != nil {
		r.timer.Stop()
	}
}

func (r *resourceDownloads) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeLocked()
}
