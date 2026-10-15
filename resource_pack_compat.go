package main

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// NetEase clients never download server packs in game: given a required
// pack they answer "have all packs" without requesting a chunk, then fail
// to load the stack. Offer NetEase players an empty pack list instead; the
// world still loads with vanilla resources. Players connecting to BDS
// directly are unaffected.
func neteaseResourcePacksInfo(pk []byte) (result []byte, removed int, err error) {
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
func neteaseResourcePackStack(pk []byte) (result []byte, removed int, err error) {
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
	stack.TexturePacks, stack.BehaviourPacks = nil, nil
	stack.TexturePackRequired = false
	var body bytes.Buffer
	stack.Marshal(protocol.NewWriter(&body, 0))
	result = append(append([]byte(nil), pk[:n]...), body.Bytes()...)
	return append(result, 0, 0), removed, nil
}
