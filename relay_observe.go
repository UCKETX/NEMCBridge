package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Inspect only the initial unencrypted network settings request. Later login
// and encrypted game packets are counted without recording their contents.
type observedStream struct {
	packetStream
	label string
	trace func(string)
	count uint64
}

func (s *observedStream) ReadPacket() ([]byte, error) {
	b, err := s.packetStream.ReadPacket()
	if err != nil {
		return b, err
	}
	s.count++
	if s.count <= 3 && s.trace != nil {
		message := fmt.Sprintf("%s：第 %d 包，长度 %d", s.label, s.count, len(b))
		game := b
		if len(game) > 0 && game[0] == 0xfe {
			game = game[1:]
		}
		if len(game) == 7 && game[0] == 6 && game[1] == 0xc1 && game[2] == 1 {
			message += fmt.Sprintf("，RequestNetworkSettings 游戏协议 %d", binary.BigEndian.Uint32(game[3:]))
		} else if len(game) > 0 {
			message += fmt.Sprintf("，批次首字节 0x%02x", game[0])
		}
		message += describeEarlyBatch(game, s.count > 1)
		s.trace(message)
	}
	return b, nil
}

func describeEarlyBatch(b []byte, compressed bool) string {
	if len(b) == 0 {
		return ""
	}
	if compressed {
		switch b[0] {
		case 0xff:
			b = b[1:]
		case 0:
			r := flate.NewReader(bytes.NewReader(b[1:]))
			defer r.Close()
			var err error
			b, err = io.ReadAll(io.LimitReader(r, 2<<20))
			if err != nil {
				return "，无法解码 Flate 批次"
			}
		default:
			return "，暂不解码此压缩类型"
		}
	}
	out := ""
	for len(b) > 0 {
		length, n := binary.Uvarint(b)
		if n <= 0 || length > uint64(len(b)-n) {
			return out + "，包边界无法解析"
		}
		p := b[n : n+int(length)]
		b = b[n+int(length):]
		id, n := binary.Uvarint(p)
		if n <= 0 {
			return out
		}
		p = p[n:]
		out += fmt.Sprintf("，PacketID=%d", id&0x3ff)
		switch id & 0x3ff {
		case 143:
			if len(p) >= 4 {
				out += fmt.Sprintf("(压缩阈值=%d，算法=%d)", binary.LittleEndian.Uint16(p), binary.LittleEndian.Uint16(p[2:]))
			}
		case 2:
			if len(p) >= 4 {
				out += fmt.Sprintf("(PlayStatus=%d)", binary.BigEndian.Uint32(p))
			}
		case 5:
			reason, n := binary.Varint(p)
			if n > 0 && len(p) > n {
				out += fmt.Sprintf("(断开原因=%d)", reason)
				p = p[n:]
				if p[0] == 0 {
					l, n := binary.Uvarint(p[1:])
					if n > 0 && l <= uint64(len(p)-1-n) {
						out += "：" + sanitize(string(p[1+n:1+n+int(l)]), "")
					}
				}
			}
		case 1:
			if len(p) < 4 {
				break
			}
			p = p[4:]
			l, n := binary.Uvarint(p)
			if n <= 0 || l > uint64(len(p)-n) {
				break
			}
			p = p[n : n+int(l)]
			if len(p) < 4 {
				break
			}
			l = uint64(binary.LittleEndian.Uint32(p))
			p = p[4:]
			if l > uint64(len(p)) {
				break
			}
			var identity map[string]json.RawMessage
			if json.Unmarshal(p[:int(l)], &identity) == nil {
				_, legacy := identity["chain"]
				_, modern := identity["Certificate"]
				out += fmt.Sprintf("(登录格式：chain=%t，Certificate=%t)", legacy, modern)
			}
		}
	}
	return out
}
