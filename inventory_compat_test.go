package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func encodeInventoryFixture(p packet.Packet) []byte {
	var body bytes.Buffer
	p.Marshal(protocol.NewWriter(&body, 0))
	return append(binary.AppendUvarint(nil, uint64(p.ID())), body.Bytes()...)
}

func TestContainerMapBoundariesAndUnknownIDs(t *testing.T) {
	for _, pair := range [][2]byte{{0, 0}, {16, 16}, {17, 18}, {27, 28}, {63, 64}, {65, 65}, {255, 255}} {
		if standardContainerToNetEase(pair[0]) != pair[1] || netEaseContainerToStandard(pair[1]) != pair[0] {
			t.Fatal("container map differs from binary table", pair)
		}
	}
	if netEaseContainerToStandard(17) != 17 || standardContainerToNetEase(64) != 64 {
		t.Fatal("unmapped container ID changed")
	}
}

func TestInventoryContainerKeepsItemBytesAndDynamicID(t *testing.T) {
	item := protocol.ItemInstance{StackNetworkID: 8, Stack: protocol.ItemStack{ItemType: protocol.ItemType{NetworkID: 5, MetadataValue: 2}, Count: 3, BlockRuntimeID: 7, NBTData: map[string]any{"fixture": "keep"}, CanBePlacedOn: []string{"stone"}}}
	content := &packet.InventoryContent{WindowID: 123, Content: []protocol.ItemInstance{{}, item}, Container: protocol.FullContainerName{ContainerID: 27, DynamicContainerID: protocol.Option(uint32(0x12345678))}}
	slot := &packet.InventorySlot{WindowID: 123, Slot: 9, NewItem: item, Container: content.Container}
	for _, p := range []packet.Packet{content, slot} {
		wire := encodeInventoryFixture(p)
		result, err := patchInventoryContainer860(wire)
		if err != nil {
			t.Fatal(err)
		}
		changes := 0
		if len(result) != len(wire) {
			t.Fatal("inventory item payload length changed")
		}
		for i := range result {
			if result[i] != wire[i] {
				changes++
				if wire[i] != 27 || result[i] != 28 {
					t.Fatal("item or dynamic-container bytes changed")
				}
			}
		}
		if changes != 1 {
			t.Fatal("wrong inventory field patched")
		}
	}
}

func TestIndependentInventoryRequestsAndResponses(t *testing.T) {
	a := &protocol.TakeStackRequestAction{}
	a.Count = 3
	a.Source.Container.ContainerID, a.Destination.Container.ContainerID = 28, 64
	a.Source.StackNetworkID, a.Destination.StackNetworkID = 5, 6
	request := &packet.ItemStackRequest{Requests: []protocol.ItemStackRequest{{RequestID: -7, Actions: []protocol.StackRequestAction{a}}}}
	converted, err := patchClientInventory860(encodeInventoryFixture(request))
	if err != nil {
		t.Fatal(err)
	}
	a.Source.Container.ContainerID, a.Destination.Container.ContainerID = 27, 63
	if !bytes.Equal(converted, encodeInventoryFixture(request)) {
		t.Fatal("source/destination IDs not translated")
	}
	transaction := &packet.InventoryTransaction{LegacyRequestID: -2, LegacySetItemSlots: []protocol.LegacySetItemSlot{{ContainerID: 28, Slots: []byte{1, 2}}}, TransactionData: &protocol.NormalTransactionData{}}
	converted, err = patchClientInventory860(encodeInventoryFixture(transaction))
	if err != nil {
		t.Fatal(err)
	}
	transaction.LegacySetItemSlots[0].ContainerID = 27
	if !bytes.Equal(converted, encodeInventoryFixture(transaction)) {
		t.Fatal("legacy container ID not translated")
	}
	response := &packet.ItemStackResponse{Responses: []protocol.ItemStackResponse{
		{RequestID: -7, ContainerInfo: []protocol.StackResponseContainerInfo{{Container: protocol.FullContainerName{ContainerID: 27, DynamicContainerID: protocol.Option(uint32(12))}, SlotInfo: []protocol.StackResponseSlotInfo{{Slot: 1, HotbarSlot: 1, Count: 3, StackNetworkID: 7}}}}},
		{Status: 1, RequestID: -8},
	}}
	converted, err = patchStackResponseContainers860(encodeInventoryFixture(response))
	if err != nil {
		t.Fatal(err)
	}
	response.Responses[0].ContainerInfo[0].Container.ContainerID = 28
	if !bytes.Equal(converted, encodeInventoryFixture(response)) {
		t.Fatal("response container/slot/status changed incorrectly")
	}
}
