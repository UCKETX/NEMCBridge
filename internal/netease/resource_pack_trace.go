package netease

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/gamebatch"
)

// Resource pack negotiation summaries: counts, codes and sizes only.
func TraceResourcePacks(batch []byte, fromClient bool, trace func(string)) {
	if trace == nil {
		return
	}
	plain, err := gamebatch.Decode(batch)
	if err != nil {
		return
	}
	packets, err := gamebatch.Split(plain)
	if err != nil {
		return
	}
	for _, pk := range packets {
		id, n := binary.Uvarint(pk)
		if n <= 0 {
			continue
		}
		buffer := bytes.NewBuffer(pk[n:])
		reader := protocol.NewReader(buffer, 0, true)
		switch id & 0x3ff {
		case packet.IDResourcePackClientResponse:
			if !fromClient {
				continue
			}
			var response packet.ResourcePackClientResponse
			if decodeQuietly(func() { response.Marshal(reader) }) {
				trace(fmt.Sprintf("资源包协商：客户端响应=%d（1拒绝 2请求下载 3已有全部 4完成），请求下载 %d 个，剩余 %d 字节", response.Response, len(response.PacksToDownload), buffer.Len()))
			} else {
				trace(fmt.Sprintf("资源包协商：客户端响应无法按标准格式解析，长度 %d", len(pk)))
			}
		case packet.IDResourcePacksInfo:
			if fromClient {
				continue
			}
			var info packet.ResourcePacksInfo
			if decodeQuietly(func() { info.Marshal(reader) }) {
				trace(fmt.Sprintf("资源包协商：服务器提供 %d 个资源包，强制=%v，剩余 %d 字节", len(info.TexturePacks), info.TexturePackRequired, buffer.Len()))
			}
		case packet.IDResourcePackChunkRequest:
			if fromClient {
				trace("资源包协商：客户端请求资源包分块")
			}
		case packet.IDResourcePackDataInfo:
			if !fromClient {
				var info packet.ResourcePackDataInfo
				if decodeQuietly(func() { info.Marshal(reader) }) {
					trace(fmt.Sprintf("资源包协商：服务器发送下载信息，大小=%d，分块大小=%d，块数=%d，类型=%d", info.Size, info.DataChunkSize, info.ChunkCount, info.PackType))
				}
			}
		case packet.IDResourcePackChunkData:
			if !fromClient {
				var chunk packet.ResourcePackChunkData
				if decodeQuietly(func() { chunk.Marshal(reader) }) {
					trace(fmt.Sprintf("资源包协商：服务器发送分块，序号=%d，偏移=%d，大小=%d", chunk.ChunkIndex, chunk.DataOffset, len(chunk.Data)))
				}
			}
		}
	}
}

func decodeQuietly(decode func()) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	decode()
	return true
}
