package netease

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Default to the known-compatible empty offer. Experimental forwarding retains
// pack IDs, sizes and content keys, filling only native metadata defaults.
// The bridge separately negotiates downloads for native clients that initially
// skip them; packet preservation alone is insufficient for those clients.
func neteaseResourcePacksInfo(pk []byte, forward bool) (result []byte, removed int, err error) {
	defer func() {
		if recover() != nil {
			result, removed, err = nil, 0, errors.New("资源包信息格式无效")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 {
		return nil, 0, errors.New("资源包信息包头无效")
	}
	buffer := bytes.NewBuffer(pk[n:])
	var info packet.ResourcePacksInfo
	info.Marshal(protocol.NewReader(buffer, 0, true))
	if buffer.Len() != 0 {
		return nil, 0, errors.New("资源包信息包含未知字段")
	}
	if forward {
		unique, err := DeduplicateTexturePacks(info.TexturePacks)
		if err != nil {
			return nil, 0, err
		}
		// Native offers give unpacked resources a content identity even when
		// there is no encryption key. Use the pack's own UUID, never another
		// component's identity. Vanilla BDS also omits the template version.
		changed := len(unique) != len(info.TexturePacks)
		info.TexturePacks = unique
		if info.WorldTemplateVersion == "" {
			info.WorldTemplateVersion = "0.0.0"
			changed = true
		}
		for i := range info.TexturePacks {
			if info.TexturePacks[i].ContentIdentity == "" {
				info.TexturePacks[i].ContentIdentity = info.TexturePacks[i].UUID.String()
				changed = true
			}
		}
		if !changed {
			return pk, 0, nil
		}
		var body bytes.Buffer
		info.Marshal(protocol.NewWriter(&body, 0))
		return append(append([]byte(nil), pk[:n]...), body.Bytes()...), 0, nil
	}
	removed = len(info.TexturePacks)
	if removed == 0 && !info.TexturePackRequired {
		return pk, 0, nil
	}
	info.TexturePacks = nil
	info.TexturePackRequired, info.HasAddons, info.HasScripts = false, false, false
	var body bytes.Buffer
	info.Marshal(protocol.NewWriter(&body, 0))
	return append(append([]byte(nil), pk[:n]...), body.Bytes()...), removed, nil
}

// The stack must match the empty pack list, or the client reports that a
// pack failed to load. Base game version and experiments are kept. Eulogist
// 4.8.0's NetEase ResourcePackStack then appends two empty pack arrays.
func neteaseResourcePackStack(pk []byte, forward bool) (result []byte, removed int, err error) {
	defer func() {
		if recover() != nil {
			result, removed, err = nil, 0, errors.New("资源包顺序格式无效")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 {
		return nil, 0, errors.New("资源包顺序包头无效")
	}
	buffer := bytes.NewBuffer(pk[n:])
	var stack packet.ResourcePackStack
	stack.Marshal(protocol.NewReader(buffer, 0, true))
	if buffer.Len() != 0 {
		return nil, 0, errors.New("资源包顺序包含未知字段")
	}
	removed = len(stack.TexturePacks) + len(stack.BehaviourPacks)
	if forward {
		stack.TexturePacks, err = deduplicateStackPacks(stack.TexturePacks)
		if err != nil {
			return nil, 0, err
		}
		stack.BehaviourPacks, err = deduplicateStackPacks(stack.BehaviourPacks)
		if err != nil {
			return nil, 0, err
		}
		// The standard 860 stack is followed by two NetEase-only pack arrays.
		// Native hosts use '*' so the client resolves its own vanilla resources
		// instead of the international server's different patch version.
		stack.BaseGameVersion = "*"
		var body bytes.Buffer
		stack.Marshal(protocol.NewWriter(&body, 0))
		result = append(append([]byte(nil), pk[:n]...), body.Bytes()...)
		return append(result, 0, 0), 0, nil
	}
	stack.TexturePacks, stack.BehaviourPacks = nil, nil
	stack.TexturePackRequired = false
	var body bytes.Buffer
	stack.Marshal(protocol.NewWriter(&body, 0))
	result = append(append([]byte(nil), pk[:n]...), body.Bytes()...)
	return append(result, 0, 0), removed, nil
}
