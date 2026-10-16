package netease

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Component preloading and streamed backend textures keep their own UUIDs,
// versions and keys. Nothing is served under another component's identity.
func mergeResourceInfo(pk []byte, profile *ResourceProfile) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = errors.New("合并资源包信息失败")
		}
	}()
	pk, _, err = neteaseResourcePacksInfo(pk, true)
	if err != nil {
		return nil, err
	}
	_, n := binary.Uvarint(pk)
	_, m := binary.Uvarint(profile.Info)
	var backend, native packet.ResourcePacksInfo
	backend.Marshal(protocol.NewReader(bytes.NewBuffer(pk[n:]), 0, true))
	native.Marshal(protocol.NewReader(bytes.NewBuffer(profile.Info[m:]), 0, true))
	seen := make(map[string]bool, len(native.TexturePacks))
	for _, p := range native.TexturePacks {
		seen[p.UUID.String()] = true
	}
	for _, p := range backend.TexturePacks {
		if seen[p.UUID.String()] {
			return nil, errors.New("后端材质包 UUID 与网易组件重复，请使用独立的资源包 UUID")
		}
		seen[p.UUID.String()] = true
	}
	backend.TexturePacks = append(native.TexturePacks, backend.TexturePacks...)
	backend.WorldTemplateUUID = native.WorldTemplateUUID
	backend.WorldTemplateVersion = native.WorldTemplateVersion
	backend.TexturePackRequired = backend.TexturePackRequired || native.TexturePackRequired
	backend.HasAddons = backend.HasAddons || native.HasAddons
	backend.HasScripts = backend.HasScripts || native.HasScripts
	var body bytes.Buffer
	backend.Marshal(protocol.NewWriter(&body, 0))
	return append(append([]byte(nil), pk[:n]...), body.Bytes()...), nil
}

func mergeResourceStack(pk []byte, profile *ResourceProfile) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = errors.New("合并资源包加载顺序失败")
		}
	}()
	_, n := binary.Uvarint(pk)
	_, m := binary.Uvarint(profile.Stack)
	var backend, native packet.ResourcePackStack
	backendBuffer := bytes.NewBuffer(pk[n:])
	backend.Marshal(protocol.NewReader(backendBuffer, 0, true))
	if backendBuffer.Len() != 0 {
		return nil, errors.New("后端资源包加载顺序包含未知字段")
	}
	backend.TexturePacks, err = deduplicateStackPacks(backend.TexturePacks)
	if err != nil {
		return nil, err
	}
	backend.BehaviourPacks, err = deduplicateStackPacks(backend.BehaviourPacks)
	if err != nil {
		return nil, err
	}
	nativeBuffer := bytes.NewBuffer(profile.Stack[m:])
	native.Marshal(protocol.NewReader(nativeBuffer, 0, true))
	backend.TexturePacks = append(native.TexturePacks, backend.TexturePacks...)
	backend.BehaviourPacks = append(native.BehaviourPacks, backend.BehaviourPacks...)
	backend.BaseGameVersion = native.BaseGameVersion
	backend.TexturePackRequired = backend.TexturePackRequired || native.TexturePackRequired
	var body bytes.Buffer
	backend.Marshal(protocol.NewWriter(&body, 0))
	result = append(append([]byte(nil), pk[:n]...), body.Bytes()...)
	return append(result, nativeBuffer.Bytes()...), nil
}
