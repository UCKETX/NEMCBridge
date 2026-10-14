package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// The international protocol 860 StartGame ends with checksum(u64), UUID(16),
// and four booleans. NetEase additionally expects its world options extension.
// Eulogist 4.8.0's NetEase Marshal at 0x140e4ec80 confirms the common core and
// the extension sequence. Its zero/default encoding is exactly 50 bytes:
// bool,string,string,bool,bool,int32*5,bool*2,int32,bool*9,
// slice<{uint64,float32}>,bool*3,string,bool*5.
const neteaseWorldExtensionSize = 50

func patchWorldSettings860(compressed []byte, trace func(string)) ([]byte, bool, error) {
	batch, err := decodeGameCompression(compressed)
	if err != nil {
		return nil, false, err
	}
	packets, err := splitGameBatch(batch)
	if err != nil {
		return nil, false, err
	}
	patched := false
	for i, pk := range packets {
		id, n := binary.Uvarint(pk)
		if n <= 0 || id&0x3ff != 11 {
			continue
		}
		packets[i], err = patchWorldPacket860(pk, trace)
		if err != nil {
			return nil, false, err
		}
		patched = true
	}
	if !patched {
		return compressed, false, nil
	}
	return append([]byte{0xff}, joinGameBatch(packets)...), true, nil
}

func patchWorldPacket860(pk []byte, trace func(string)) ([]byte, error) {
	_, n := binary.Uvarint(pk)
	if n <= 0 || len(pk)-n < 28 {
		return nil, errors.New("StartGame 860 尾部不完整")
	}
	tail := pk[len(pk)-28:]
	for _, flag := range tail[24:] {
		if flag > 1 {
			return nil, errors.New("StartGame 860 尾部字段不兼容")
		}
	}
	if tail[25] != 1 {
		return nil, errors.New("网易方块兼容需要后端 block-network-ids-are-hashes=true")
	}
	old := binary.LittleEndian.Uint64(tail)
	modified := append([]byte(nil), pk...)
	tail = modified[len(modified)-28:]
	clear(tail[:8])
	tail[24] = 0
	modified = append(modified, make([]byte, neteaseWorldExtensionSize)...)
	if trace != nil {
		trace(fmt.Sprintf("StartGame 网易转换：补齐 %d 字节世界设置；方块校验和 %d → 0；关闭客户端地形生成，保留哈希方块 ID", neteaseWorldExtensionSize, old))
	}
	return modified, nil
}
