package netease

import (
	"errors"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// DeduplicateTexturePacks keeps the first occurrence of identical metadata. BDS
// may advertise a world pack again through a behaviour pack's dependency.
// Conflicting versions, keys or subpacks under one UUID remain an error.
func DeduplicateTexturePacks(packs []protocol.TexturePackInfo) ([]protocol.TexturePackInfo, error) {
	seen := make(map[uuid.UUID]protocol.TexturePackInfo, len(packs))
	result := make([]protocol.TexturePackInfo, 0, len(packs))
	for _, p := range packs {
		if prior, ok := seen[p.UUID]; ok {
			if prior != p {
				return nil, errors.New("后端同一资源包 UUID 存在冲突的版本或下载参数")
			}
			continue
		}
		seen[p.UUID] = p
		result = append(result, p)
	}
	return result, nil
}

func deduplicateStackPacks(packs []protocol.StackResourcePack) ([]protocol.StackResourcePack, error) {
	seen := make(map[string]protocol.StackResourcePack, len(packs))
	result := make([]protocol.StackResourcePack, 0, len(packs))
	for _, p := range packs {
		if prior, ok := seen[p.UUID]; ok {
			if prior != p {
				return nil, errors.New("后端同一资源包 UUID 存在冲突的加载顺序参数")
			}
			continue
		}
		seen[p.UUID] = p
		result = append(result, p)
	}
	return result, nil
}
