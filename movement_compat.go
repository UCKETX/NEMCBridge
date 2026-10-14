package main

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Eulogist 4.8.0 NetEase PlayerAuthInput has two additional flag positions,
// a boolean after Delta, and proprietary fields following the common vectors.
// Decode the common fields, discard the extensions, and write standard 860.
func patchPlayerAuthInput860(pk []byte) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("网易玩家输入包格式无效或不完整")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 {
		return nil, errors.New("网易玩家输入包头无效")
	}
	input := bytes.NewBuffer(pk[n:])
	r := protocol.NewReader(input, 0, true)
	var p packet.PlayerAuthInput
	r.Float32(&p.Pitch)
	r.Float32(&p.Yaw)
	r.Vec3(&p.Position)
	r.Vec2(&p.MoveVector)
	r.Float32(&p.HeadYaw)
	var flags protocol.Bitset
	r.Bitset(&flags, 67)
	p.InputData = protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	for i := 0; i < 67; i++ {
		if i == 44 || i == 45 || !flags.Load(i) {
			continue
		}
		index := i
		if i >= 46 {
			index -= 2
		}
		p.InputData.Set(index)
	}
	r.Varuint32(&p.InputMode)
	r.Varuint32(&p.PlayMode)
	r.Varuint32(&p.InteractionModel)
	r.Float32(&p.InteractPitch)
	r.Float32(&p.InteractYaw)
	r.Varuint64(&p.Tick)
	r.Vec3(&p.Delta)
	var extension bool
	r.Bool(&extension)
	if p.InputData.Load(packet.InputFlagPerformItemInteraction) {
		r.PlayerInventoryAction(&p.ItemInteractionData)
		for i := range p.ItemInteractionData.LegacySetItemSlots {
			id := &p.ItemInteractionData.LegacySetItemSlots[i].ContainerID
			*id = netEaseContainerToStandard(*id)
		}
	}
	if p.InputData.Load(packet.InputFlagPerformItemStackRequest) {
		protocol.Single(r, &p.ItemStackRequest)
		mapNetEaseStackRequest(&p.ItemStackRequest)
	}
	if p.InputData.Load(packet.InputFlagPerformBlockActions) {
		protocol.SliceVarint32Length(r, &p.BlockActions)
	}
	if p.InputData.Load(packet.InputFlagClientPredictedVehicle) {
		r.Vec2(&p.VehicleRotation)
		r.Varint64(&p.ClientPredictedVehicle)
	}
	r.Vec2(&p.AnalogueMoveVector)
	r.Vec3(&p.CameraOrientation)
	r.Vec2(&p.RawMoveVector)
	r.Bool(&extension)
	var repeatPitch, repeatYaw float32
	r.Float32(&repeatPitch)
	r.Float32(&repeatYaw)
	r.Bool(&extension)
	r.Bool(&extension)
	// The final Bytes field is the remaining payload, not a prefixed string.
	// It carries no standard field and is intentionally not logged or forwarded.
	var opaque []byte
	r.Bytes(&opaque)
	var output bytes.Buffer
	p.Marshal(protocol.NewWriter(&output, 0))
	return append(append([]byte(nil), pk[:n]...), output.Bytes()...), nil
}

func netEaseContainerToStandard(id byte) byte {
	if id >= 18 && id <= 64 {
		return id - 1
	}
	return id
}

func mapNetEaseStackRequest(request *protocol.ItemStackRequest) {
	mapSlot := func(slot *protocol.StackRequestSlotInfo) {
		slot.Container.ContainerID = netEaseContainerToStandard(slot.Container.ContainerID)
	}
	for _, action := range request.Actions {
		switch a := action.(type) {
		case *protocol.TakeStackRequestAction:
			mapSlot(&a.Source)
			mapSlot(&a.Destination)
		case *protocol.PlaceStackRequestAction:
			mapSlot(&a.Source)
			mapSlot(&a.Destination)
		case *protocol.SwapStackRequestAction:
			mapSlot(&a.Source)
			mapSlot(&a.Destination)
		case *protocol.DropStackRequestAction:
			mapSlot(&a.Source)
		case *protocol.DestroyStackRequestAction:
			mapSlot(&a.Source)
		case *protocol.ConsumeStackRequestAction:
			mapSlot(&a.Source)
		case *protocol.PlaceInContainerStackRequestAction:
			mapSlot(&a.Source)
			mapSlot(&a.Destination)
		case *protocol.TakeOutContainerStackRequestAction:
			mapSlot(&a.Source)
			mapSlot(&a.Destination)
		}
	}
}
