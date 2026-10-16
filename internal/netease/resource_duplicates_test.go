package netease

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

func TestTextureDuplicatesKeepOrderAndRejectConflicts(t *testing.T) {
	a := protocol.TexturePackInfo{UUID: uuid.New(), Version: "1.1.8", Size: 1024, ContentKey: "fixture-only", SubPackName: "simple"}
	b := protocol.TexturePackInfo{UUID: uuid.New(), Version: "1.14.2", Size: 2048}
	got, err := DeduplicateTexturePacks([]protocol.TexturePackInfo{a, b, a, b})
	if err != nil || !reflect.DeepEqual(got, []protocol.TexturePackInfo{a, b}) {
		t.Fatal("identical dependency entries must collapse in original order", err)
	}
	for _, change := range []func(*protocol.TexturePackInfo){
		func(p *protocol.TexturePackInfo) { p.Version = "2.0.0" },
		func(p *protocol.TexturePackInfo) { p.ContentKey = "different-fixture" },
		func(p *protocol.TexturePackInfo) { p.Size++ },
		func(p *protocol.TexturePackInfo) { p.SubPackName = "different" },
	} {
		conflict := a
		change(&conflict)
		if _, err := DeduplicateTexturePacks([]protocol.TexturePackInfo{a, conflict}); err == nil {
			t.Fatal("conflicting pack metadata silently discarded")
		}
	}
}

func TestStackDuplicatesKeepOrderAndRejectConflicts(t *testing.T) {
	a := protocol.StackResourcePack{UUID: uuid.NewString(), Version: "1.1.8", SubPackName: "simple"}
	b := protocol.StackResourcePack{UUID: uuid.NewString(), Version: "1.14.2"}
	got, err := deduplicateStackPacks([]protocol.StackResourcePack{a, b, a})
	if err != nil || !reflect.DeepEqual(got, []protocol.StackResourcePack{a, b}) {
		t.Fatal("dependency stack entries must collapse in original order", err)
	}
	conflict := a
	conflict.SubPackName = "different"
	if _, err := deduplicateStackPacks([]protocol.StackResourcePack{a, conflict}); err == nil {
		t.Fatal("conflicting stack entries silently discarded")
	}
}
