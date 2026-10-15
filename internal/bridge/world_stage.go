package bridge

import (
	"encoding/binary"
	"fmt"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/relay"
)

// Inspect packet IDs and fixed loading milestones; never record login or skin
// payloads. Disconnect strings are bounded and passed through credential filters.
func (b *Bridge) observeWorldStage(compressed []byte, client bool) {
	if b.trace == nil {
		return
	}
	batch, err := gamebatch.Decode(compressed)
	if err != nil {
		return
	}
	packets, err := gamebatch.Split(batch)
	if err != nil {
		return
	}
	for _, pk := range packets {
		id, n := binary.Uvarint(pk)
		if n <= 0 {
			continue
		}
		id &= 0x3ff
		if !client {
			if b.serverPacketIDs == nil {
				b.serverPacketIDs = make(map[uint64]bool)
			}
			if !b.serverPacketIDs[id] {
				b.serverPacketIDs[id] = true
				b.trace(fmt.Sprintf("世界数据包：PacketID=%d，包长=%d，传输批次=%d 字节", id, len(pk), len(compressed)))
			}
		}
		if id == 5 {
			label := "本地服务器"
			if client {
				label = "客户端"
			}
			// Reuse the safe early-batch decoder on just this packet.
			b.trace(label + "主动断开" + relay.DescribeEarlyBatch(append([]byte{0xff}, gamebatch.Join([][]byte{pk})...), true))
		}
		if client {
			var bit uint32
			var stage string
			switch id {
			case packet.IDClientCacheStatus:
				if len(pk) == n+1 {
					b.trace(fmt.Sprintf("世界加载阶段：客户端区块缓存=%v", pk[n] != 0))
				}
			case packet.IDRequestChunkRadius:
				bit, stage = 1, "请求区块半径"
			case packet.IDSetLocalPlayerAsInitialised:
				bit, stage = 2, "确认本地玩家初始化完成"
			case packet.IDPlayerAuthInput:
				bit, stage = 4, "开始发送玩家移动输入"
			case packet.IDSubChunkRequest:
				bit, stage = 8, "请求地形子区块"
			case packet.IDClientCacheBlobStatus:
				bit, stage = 16, "报告区块缓存命中或缺失"
			}
			if bit != 0 && b.clientStages&bit == 0 {
				b.clientStages |= bit
				b.trace(fmt.Sprintf("世界加载阶段：客户端%s（PacketID=%d）", stage, id))
			}
		}
	}
}
