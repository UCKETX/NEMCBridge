package netease

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// ClassicPreloadComponentID is the unencrypted Classic Textures component.
const ClassicPreloadComponentID uint64 = 4632802189343756763

// Only unencrypted protocol metadata is embedded, not third-party pack files,
// account credentials or server-provided content keys.
//
//go:embed assets/classic-preload.json
var classicPreload []byte

// BuiltinResourceProfile returns a fresh profile so per-room callers cannot
// mutate the shared embedded metadata. Clients fetch the component via NetEase.
func BuiltinResourceProfile() (*ResourceProfile, error) {
	var p ResourceProfile
	if err := json.Unmarshal(classicPreload, &p); err != nil {
		return nil, errors.New("内置经典材质配置无效")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	_, n := binary.Uvarint(p.Info)
	var info packet.ResourcePacksInfo
	info.Marshal(protocol.NewReader(bytes.NewBuffer(p.Info[n:]), 0, true))
	for _, pack := range info.TexturePacks {
		if pack.ContentKey != "" || pack.DownloadURL != "" {
			return nil, errors.New("内置经典材质配置不得包含内容密钥或下载地址")
		}
	}
	return &p, nil
}
