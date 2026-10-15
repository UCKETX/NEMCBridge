package netease

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func standardContainerToNetEase(id byte) byte {
	if id >= 17 && id <= 63 {
		return id + 1
	}
	return id
}

// Patch only the container byte. ItemInstance data is kept byte for byte,
// including item-specific user data and dynamic container IDs.
func patchInventoryContainer860(pk []byte) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("物品栏公共字段不完整")
		}
	}()
	id, n := binary.Uvarint(pk)
	input := bytes.NewBuffer(pk[n:])
	r := protocol.NewReader(input, 0, true)
	var window uint32
	r.Varuint32(&window)
	if id&0x3ff == packet.IDInventoryContent {
		var count uint32
		r.Varuint32(&count)
		if count > 1024 {
			return nil, errors.New("物品栏数量超出限制")
		}
		for range count {
			var item protocol.ItemInstance
			r.ItemInstance(&item)
		}
	} else {
		var slot uint32
		r.Varuint32(&slot)
	}
	cursor := len(pk) - input.Len()
	if cursor >= len(pk) {
		return nil, errors.New("物品栏容器字段不完整")
	}
	result = append([]byte(nil), pk...)
	result[cursor] = standardContainerToNetEase(pk[cursor])
	return result, nil
}

func patchClientInventory860(pk []byte) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("网易物品请求公共字段不完整")
		}
	}()
	id, n := binary.Uvarint(pk)
	input := bytes.NewBuffer(pk[n:])
	r := protocol.NewReader(input, 0, true)
	if id&0x3ff == packet.IDItemStackRequest {
		var request packet.ItemStackRequest
		request.Marshal(r)
		if input.Len() != 0 {
			return nil, errors.New("网易物品请求含未知尾部")
		}
		for i := range request.Requests {
			mapNetEaseStackRequest(&request.Requests[i])
		}
		var out bytes.Buffer
		request.Marshal(protocol.NewWriter(&out, 0))
		return append(append([]byte(nil), pk[:n]...), out.Bytes()...), nil
	}
	// InventoryTransaction uses a separate legacy slot list at its beginning.
	// The remaining transaction and item payloads already share the wire layout.
	var requestID int32
	r.Varint32(&requestID)
	result = append([]byte(nil), pk...)
	if requestID != 0 {
		var count uint32
		r.Varuint32(&count)
		if count > 1024 {
			return nil, errors.New("旧版物品请求数量超出限制")
		}
		for range count {
			cursor := len(pk) - input.Len()
			var slot protocol.LegacySetItemSlot
			slot.Marshal(r)
			result[cursor] = netEaseContainerToStandard(slot.ContainerID)
		}
	}
	return result, nil
}

// ItemStackResponse carries the same FullContainerName semantics as the
// confirmed inventory/request conversions. Apply that mapping to acknowledgments
// too, retaining slot updates and rejected requests unchanged.
func patchStackResponseContainers860(pk []byte) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("物品请求响应公共字段不完整")
		}
	}()
	_, n := binary.Uvarint(pk)
	input := bytes.NewBuffer(pk[n:])
	r := protocol.NewReader(input, 0, true)
	var count uint32
	r.Varuint32(&count)
	if count > 4096 {
		return nil, errors.New("物品响应数量超出限制")
	}
	result = append([]byte(nil), pk...)
	for range count {
		var status byte
		var requestID int32
		r.Uint8(&status)
		r.Varint32(&requestID)
		if status != protocol.ItemStackResponseStatusOK {
			continue
		}
		var containers uint32
		r.Varuint32(&containers)
		if containers > 1024 {
			return nil, errors.New("物品响应容器数量超出限制")
		}
		for range containers {
			cursor := len(pk) - input.Len()
			var container protocol.StackResponseContainerInfo
			container.Marshal(r)
			result[cursor] = standardContainerToNetEase(container.Container.ContainerID)
		}
	}
	if input.Len() != 0 {
		return nil, errors.New("物品响应含未知尾部")
	}
	return result, nil
}
