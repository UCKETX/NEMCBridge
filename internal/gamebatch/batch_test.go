package gamebatch

import (
	"bytes"
	"testing"
)

func TestDecodedBatchSizeLimit(t *testing.T) {
	for _, size := range []int{MaxDecoded, MaxDecoded + 1} {
		plain := bytes.Repeat([]byte{7}, size)
		compressed, err := Encode([]byte{0}, plain)
		if err != nil {
			t.Fatal(err)
		}
		for _, wire := range [][]byte{compressed, append([]byte{0xff}, plain...)} {
			decoded, err := Decode(wire)
			if size == MaxDecoded {
				if err != nil || !bytes.Equal(decoded, plain) {
					t.Fatal("exact limit rejected", err)
				}
			} else if err == nil {
				t.Fatal("over-limit batch accepted")
			}
		}
	}
}
