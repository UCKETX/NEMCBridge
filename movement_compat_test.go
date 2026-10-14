package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestNetEaseMovementCommonFieldsAndFlagMapping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ne, std []int
	}{
		{"walking", []int{10, 20, 44, 45, 46, 66}, []int{10, 20, 44, 64}},
		{"vehicle", []int{47}, []int{45}},
		{"interaction", []int{34}, []int{34}},
		{"block_actions", []int{35}, []int{35}},
		{"stack_request", []int{36}, []int{36}},
		{"combined", []int{34, 35, 36, 47, 66}, []int{34, 35, 36, 45, 64}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := packet.PlayerAuthInput{
				Pitch: 13, Yaw: 15, Position: [3]float32{2, 64, 5}, MoveVector: [2]float32{.5, 1}, HeadYaw: 17,
				InputData: protocol.NewBitset(65), InputMode: 2, PlayMode: 0, InteractionModel: 1,
				InteractPitch: 3, InteractYaw: 7, Tick: 300, Delta: [3]float32{.1, .2, .3},
				AnalogueMoveVector: [2]float32{.2, .8}, CameraOrientation: [3]float32{.3, .5, .7}, RawMoveVector: [2]float32{.4, .9},
			}
			for _, index := range tc.std {
				p.InputData.Set(index)
			}
			if p.InputData.Load(45) {
				p.VehicleRotation = [2]float32{19, 23}
				p.ClientPredictedVehicle = -700
			}
			if p.InputData.Load(35) {
				p.BlockActions = []protocol.PlayerBlockAction{{Action: 0, BlockPos: protocol.BlockPos{5, 63, 7}, Face: 2}}
			}
			if p.InputData.Load(36) {
				p.ItemStackRequest.RequestID = 7
				p.ItemStackRequest.FilterStrings = []string{"fixture"}
				a := &protocol.TakeStackRequestAction{}
				a.Count = 2
				a.Source.Container.ContainerID, a.Destination.Container.ContainerID = 17, 63
				a.Source.StackNetworkID, a.Destination.StackNetworkID = 5, 6
				p.ItemStackRequest.Actions = []protocol.StackRequestAction{a}
			}
			wire := encodeNetEaseMovementFixture(p, tc.ne)
			converted, err := patchPlayerAuthInput860(wire)
			if err != nil {
				t.Fatal(err)
			}
			_, n := binary.Uvarint(converted)
			buffer := bytes.NewBuffer(converted[n:])
			var decoded packet.PlayerAuthInput
			decoded.Marshal(protocol.NewReader(buffer, 0, true))
			var expected bytes.Buffer
			p.Marshal(protocol.NewWriter(&expected, 0))
			if buffer.Len() != 0 || !bytes.Equal(converted[n:], expected.Bytes()) {
				t.Fatal("standard decoder lost movement or optional action fields")
			}
			if _, err := patchPlayerAuthInput860(wire[:45]); err == nil {
				t.Fatal("truncated movement accepted")
			}
		})
	}
}

// Assemble the independently recovered NetEase wire layout, with nonzero
// extensions so leaving them in the packet cannot accidentally pass decoding.
func encodeNetEaseMovementFixture(p packet.PlayerAuthInput, indices []int) []byte {
	var body bytes.Buffer
	w := protocol.NewWriter(&body, 0)
	w.Float32(&p.Pitch)
	w.Float32(&p.Yaw)
	w.Vec3(&p.Position)
	w.Vec2(&p.MoveVector)
	w.Float32(&p.HeadYaw)
	flags := protocol.NewBitset(67)
	for _, i := range indices {
		flags.Set(i)
	}
	w.Bitset(&flags, 67)
	w.Varuint32(&p.InputMode)
	w.Varuint32(&p.PlayMode)
	w.Varuint32(&p.InteractionModel)
	w.Float32(&p.InteractPitch)
	w.Float32(&p.InteractYaw)
	w.Varuint64(&p.Tick)
	w.Vec3(&p.Delta)
	extraBool := true
	w.Bool(&extraBool)
	if flags.Load(34) {
		w.PlayerInventoryAction(&p.ItemInteractionData)
	}
	if flags.Load(36) {
		request := p.ItemStackRequest
		request.Actions = nil
		for _, action := range p.ItemStackRequest.Actions {
			a := *action.(*protocol.TakeStackRequestAction)
			a.Source.Container.ContainerID, a.Destination.Container.ContainerID = 18, 64
			request.Actions = append(request.Actions, &a)
		}
		protocol.Single(w, &request)
	}
	if flags.Load(35) {
		protocol.SliceVarint32Length(w, &p.BlockActions)
	}
	if flags.Load(47) {
		w.Vec2(&p.VehicleRotation)
		w.Varint64(&p.ClientPredictedVehicle)
	}
	w.Vec2(&p.AnalogueMoveVector)
	w.Vec3(&p.CameraOrientation)
	w.Vec2(&p.RawMoveVector)
	w.Bool(&extraBool)
	extraFloat := float32(99)
	w.Float32(&extraFloat)
	w.Float32(&extraFloat)
	w.Bool(&extraBool)
	w.Bool(&extraBool)
	opaque := []byte("fixture-proprietary-input")
	w.Bytes(&opaque)
	return append(binary.AppendUvarint(nil, uint64(packet.IDPlayerAuthInput)), body.Bytes()...)
}
