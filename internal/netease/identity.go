package netease

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// CommandOrigin starts with a Varuint32 origin, then the UUID. Preserve all
// other bytes, including request IDs, output messages and subclient headers.
func remapCommandUUID(pk []byte, from, to uuid.UUID) ([]byte, error) {
	id, n := binary.Uvarint(pk)
	if n <= 0 || (id&0x3ff != packet.IDCommandRequest && id&0x3ff != packet.IDCommandOutput) {
		return nil, errors.New("命令身份包头无效")
	}
	cursor := n
	if id&0x3ff == packet.IDCommandRequest {
		length, size := binary.Uvarint(pk[cursor:])
		if size <= 0 || length > uint64(len(pk)-cursor-size) {
			return nil, errors.New("命令字符串长度无效")
		}
		cursor += size + int(length)
	}
	_, size := binary.Uvarint(pk[cursor:])
	if size <= 0 {
		return nil, errors.New("命令来源字段无效")
	}
	return remapWireUUID(pk, cursor+size, from, to)
}

func remapSkinUUID(pk []byte, from, to uuid.UUID) ([]byte, error) {
	_, n := binary.Uvarint(pk)
	if n <= 0 {
		return nil, errors.New("换肤包头无效")
	}
	return remapWireUUID(pk, n, from, to)
}

func remapWireUUID(pk []byte, offset int, from, to uuid.UUID) ([]byte, error) {
	if offset < 0 || len(pk)-offset < 16 {
		return nil, errors.New("玩家 UUID 字段不完整")
	}
	var current uuid.UUID
	protocol.NewReader(bytes.NewReader(pk[offset:offset+16]), 0, true).UUID(&current)
	if current != from {
		return pk, nil
	}
	var replacement bytes.Buffer
	protocol.NewWriter(&replacement, 0).UUID(&to)
	result := append([]byte(nil), pk...)
	copy(result[offset:offset+16], replacement.Bytes())
	return result, nil
}

// Restore only the recipient's own presentation UUID. Other players retain
// distinct backend UUIDs even when their original NetEase UUIDs are identical.
// Keep the NetEase extension bytes following the standard list unchanged.
func remapPlayerListUUID(pk []byte, from, to uuid.UUID) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("玩家列表身份格式无效")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 {
		return nil, errors.New("玩家列表包头无效")
	}
	input := bytes.NewBuffer(pk[n:])
	var list packet.PlayerList
	list.Marshal(protocol.NewReader(input, 0, true))
	changed := false
	for i := range list.Entries {
		if list.Entries[i].UUID == from {
			list.Entries[i].UUID = to
			changed = true
		}
	}
	if !changed {
		return pk, nil
	}
	var body bytes.Buffer
	list.Marshal(protocol.NewWriter(&body, 0))
	result = append(append([]byte(nil), pk[:n]...), body.Bytes()...)
	return append(result, input.Bytes()...), nil
}
