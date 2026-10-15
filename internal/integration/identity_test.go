package integration

import (
	"encoding/binary"
	"testing"

	"nemcbridge/internal/bridge"
	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/relay/relaytest"
)

func TestBridgePreservesUUIDAndForwardsBackendXUID(t *testing.T) {
	for _, xuid := range []string{"", "2535421234567890"} {
		stream := relaytest.New()
		b, err := bridge.New(stream, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer b.ForgetSkin()
		fields := map[string]any{"identity": "12345678-1234-5678-9abc-123456789abc", "displayName": "NetEaseIdentityTest", "netease_uid": "1261597877", "XUID": xuid}
		if _, err := b.Write([]byte{0xfe, 0}); err != nil {
			t.Fatal(err)
		}
		<-stream.Out
		if _, err := b.Write(makeTestLoginWithIdentity(t, fields)); err != nil {
			t.Fatal(err)
		}
		wire := <-stream.Out
		plain, err := gamebatch.Decode(wire[1:])
		if err != nil {
			t.Fatal(err)
		}
		packets, err := gamebatch.Split(plain)
		if err != nil || len(packets) != 1 {
			t.Fatal("invalid backend login", err)
		}
		_, header := binary.Uvarint(packets[0])
		_, length := binary.Uvarint(packets[0][header+4:])
		identity, _, err := bridge.ParseIdentity(packets[0][header+4+length:])
		if err != nil {
			t.Fatalf("rewritten login signatures invalid: %v", err)
		}
		if identity["identity"] != fields["identity"] || identity["displayName"] != fields["displayName"] {
			t.Fatal("player UUID/name changed")
		}
		if identity["XUID"] != bridge.BackendXUID(fields) {
			t.Fatalf("backend XUID = %v", identity["XUID"])
		}
	}
}
