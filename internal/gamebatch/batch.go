package gamebatch

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// BDS may combine many terrain packets in one compressed batch while moving.
// The inflated size is distinct from the much smaller WebRTC wire size.
const MaxDecoded = 32 << 20

func Decode(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	if b[0] == 0xff {
		if len(b)-1 > MaxDecoded {
			return nil, errors.New("游戏批次超出 32MiB（解压后）")
		}
		return b[1:], nil
	}
	if b[0] != 0 {
		return nil, fmt.Errorf("暂不支持压缩算法 %d", b[0])
	}
	r := flate.NewReader(bytes.NewReader(b[1:]))
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, MaxDecoded+1))
	if len(data) > MaxDecoded {
		return nil, errors.New("游戏批次超出 32MiB（解压后）")
	}
	return data, err
}

func Split(b []byte) ([][]byte, error) {
	var packets [][]byte
	for len(b) > 0 {
		l, n := binary.Uvarint(b)
		if n <= 0 || l > uint64(len(b)-n) {
			return nil, errors.New("游戏批次边界无效")
		}
		packets = append(packets, b[n:n+int(l)])
		b = b[n+int(l):]
	}
	return packets, nil
}

func Join(packets [][]byte) []byte {
	var out []byte
	for _, p := range packets {
		out = binary.AppendUvarint(out, uint64(len(p)))
		out = append(out, p...)
	}
	return out
}

func Encode(compressed, converted []byte) ([]byte, error) {
	if compressed[0] == 0 {
		// Preserve negotiated DEFLATE for large recipe/command batches.
		// Escaping the complete batch made a ~1MB fragmented world message.
		var output bytes.Buffer
		output.WriteByte(0)
		writer, err := flate.NewWriter(&output, flate.DefaultCompression)
		if err != nil {
			return nil, err
		}
		if _, err = writer.Write(converted); err != nil {
			writer.Close()
			return nil, err
		}
		if err = writer.Close(); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}
	return append([]byte{0xff}, converted...), nil
}
