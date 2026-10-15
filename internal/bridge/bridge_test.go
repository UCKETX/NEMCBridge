package bridge

import (
	"bytes"
	"testing"
)

func TestGameCipherCountersAndIntegrity(t *testing.T) {
	key := [32]byte{1, 2, 3}
	send, recv := newGameCipher(key), newGameCipher(key)
	for _, plain := range [][]byte{{0xff, 1, 4}, bytes.Repeat([]byte{7}, 17000), {0xff, 2, 3, 4}} {
		encoded := send.encrypt(plain)
		decoded, err := recv.decrypt(encoded)
		if err != nil || !bytes.Equal(plain, decoded) {
			t.Fatal("AES-CTR counter or checksum failed", err)
		}
	}
	bad := send.encrypt([]byte{9})
	bad[0] ^= 1
	if _, err := recv.decrypt(bad); err == nil {
		t.Fatal("corrupt batch accepted")
	}
}
