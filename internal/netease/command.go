package netease

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// NetEase appends UnLimited after the standard CommandRequest.Version.
// Forward only the standard bytes; keep the command, origin and permissions
// unchanged. Sending the extra bool makes BDS reject the packet as unread data.
func patchCommandRequest860(pk []byte) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("命令请求字段不完整或格式无效")
		}
	}()
	id, n := binary.Uvarint(pk)
	if n <= 0 || id&0x3ff != packet.IDCommandRequest {
		return nil, errors.New("命令请求包头无效")
	}
	buffer := bytes.NewBuffer(pk[n:])
	var command packet.CommandRequest
	command.Marshal(protocol.NewReader(buffer, 0, true))
	end := len(pk) - buffer.Len()
	if buffer.Len() == 0 {
		return pk, nil
	} // Standard integration frontends.
	if buffer.Len() != 1 || pk[end] > 1 {
		return nil, errors.New("网易命令请求扩展无效")
	}
	return append([]byte(nil), pk[:end]...), nil
}
