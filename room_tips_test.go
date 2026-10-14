package main

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/encoding"
)

func TestCurrentRoomTipsMatchesObservedClient(t *testing.T) {
	// Captured via the official room query endpoint from a current 3.9 room.
	captured := "000c336b4342557573796c6c6b3d0000047465737400002a000a312e32312e3132302e30"
	want, err := hex.DecodeString(captured)
	if err != nil {
		t.Fatal(err)
	}
	tips := encoding.RoomTips{LevelID: "3kCBUusyllk=", ConstantTestString: "test", ProtocolID: 42, GameVersion: "1.21.120.0"}
	buf := bytes.NewBuffer(nil)
	encoding.NewWriter(buf, 0).RoomTips(&tips)
	if !bytes.Equal(buf.Bytes()[2:], want) {
		t.Fatalf("room tips differ from observed current client: %x", buf.Bytes()[2:])
	}
	var roundTrip encoding.RoomTips
	encoding.NewReader(bytes.NewBuffer(buf.Bytes())).RoomTips(&roundTrip)
	if roundTrip != tips {
		t.Fatalf("extended tips reader lost fields: %#v", roundTrip)
	}
}
