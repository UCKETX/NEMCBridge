package neteasetest

import (
	"bytes"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// Independently walk the NetEase biome layout (fixed fields, extra fields,
// optional tags, no client terrain generation, dictionary) to catch shifted
// entry boundaries. This reader does not call the conversion implementation.
// VerifyBiomePayload checks a NetEase BiomeDefinitionList payload layout.
func VerifyBiomePayload(t *testing.T, payload []byte) {
	t.Helper()
	b := bytes.NewBuffer(payload)
	r := protocol.NewReader(b, 0, true)
	var count uint32
	r.Varuint32(&count)
	if count == 0 {
		t.Fatal("biomes unexpectedly empty")
	}
	for range count {
		if len(b.Next(29)) != 29 || !bytes.Equal(b.Next(5), make([]byte, 5)) {
			t.Fatal("NetEase biome extra fields not at expected wire offset")
		}
		var hasTags bool
		r.Bool(&hasTags)
		if hasTags {
			var tags uint32
			r.Varuint32(&tags)
			if len(b.Next(int(tags)*2)) != int(tags)*2 {
				t.Fatal("biome tag indices truncated")
			}
		}
		var generation bool
		r.Bool(&generation)
		if generation {
			t.Fatal("client generation still advertised")
		}
	}
	r.Varuint32(&count)
	for range count {
		var s string
		r.String(&s)
	}
	if b.Len() != 0 {
		t.Fatal("NetEase biome dictionary is misaligned")
	}
}
