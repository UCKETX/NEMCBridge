package netease

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"nemcbridge/internal/gamebatch"
)

func TestPlayerIdentityCommandRoundTrip(t *testing.T) {
	client, backend, unrelated := uuid.New(), uuid.New(), uuid.New()
	for _, origin := range []uuid.UUID{client, unrelated} {
		command := &packet.CommandRequest{CommandLine: "/help", CommandOrigin: protocol.CommandOrigin{UUID: origin, RequestID: "keep-request"}, Version: 42}
		var body bytes.Buffer
		command.Marshal(protocol.NewWriter(&body, 0))
		wire := binary.AppendUvarint(nil, uint64(packet.IDCommandRequest)|1<<10)
		wire = append(wire, body.Bytes()...)
		wire = append(wire, 0) // NetEase UnLimited.
		converted, err := ConvertClientForPlayer(append([]byte{0xff}, gamebatch.Join([][]byte{wire})...), client, backend)
		if err != nil {
			t.Fatal(err)
		}
		data, err := gamebatch.Decode(converted)
		if err != nil {
			t.Fatal(err)
		}
		packets, err := gamebatch.Split(data)
		if err != nil {
			t.Fatal(err)
		}
		id, n := binary.Uvarint(packets[0])
		var got packet.CommandRequest
		reader := bytes.NewBuffer(packets[0][n:])
		got.Marshal(protocol.NewReader(reader, 0, true))
		want := origin
		if origin == client {
			want = backend
		}
		if got.CommandOrigin.UUID != want || got.CommandOrigin.RequestID != "keep-request" || id>>10 != 1 || reader.Len() != 0 {
			t.Fatal("upstream command identity/header changed incorrectly")
		}
		output := &packet.CommandOutput{CommandOrigin: got.CommandOrigin, OutputType: 3, SuccessCount: 1, OutputMessages: []protocol.CommandOutputMessage{{Success: true, Message: "ok"}}}
		body.Reset()
		output.Marshal(protocol.NewWriter(&body, 0))
		wire = binary.AppendUvarint(nil, uint64(packet.IDCommandOutput)|1<<10)
		wire = append(wire, body.Bytes()...)
		server := ServerConverter{ClientUUID: client, BackendUUID: backend}
		converted, err = server.Convert(append([]byte{0xff}, gamebatch.Join([][]byte{wire})...))
		if err != nil {
			t.Fatal(err)
		}
		data, _ = gamebatch.Decode(converted)
		packets, _ = gamebatch.Split(data)
		_, n = binary.Uvarint(packets[0])
		var reply packet.CommandOutput
		reply.Marshal(protocol.NewReader(bytes.NewBuffer(packets[0][n:]), 0, true))
		if reply.CommandOrigin.UUID != origin || reply.CommandOrigin.RequestID != "keep-request" || reply.SuccessCount != 1 || reply.OutputMessages[0].Message != "ok" {
			t.Fatal("command response identity/content mismatch")
		}
	}
}

func TestPlayerSkinUsesBackendIdentity(t *testing.T) {
	client, backend := uuid.New(), uuid.New()
	identity, claims := netEaseLoginFixture(t, client)
	_, record, ok := OriginalLoginSkin(identity, claims)
	if !ok {
		t.Fatal("fixture invalid")
	}
	RememberSkin(backend, record)
	defer ForgetSkin(backend)
	update := packet.PlayerSkin{UUID: client, Skin: record.skin}
	update.Skin.SkinID = "changed-skin"
	var body bytes.Buffer
	update.Marshal(protocol.NewWriter(&body, 0))
	wire := append([]byte{byte(packet.IDPlayerSkin)}, body.Bytes()...)
	converted, err := ConvertClientForPlayer(append([]byte{0xff}, gamebatch.Join([][]byte{wire})...), client, backend)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := gamebatch.Decode(converted)
	packets, _ := gamebatch.Split(data)
	_, n := binary.Uvarint(packets[0])
	var got packet.PlayerSkin
	got.Marshal(protocol.NewReader(bytes.NewBuffer(packets[0][n:]), 0, true))
	if got.UUID != backend {
		t.Fatal("skin update kept shared client UUID")
	}
	remembered, ok := lookupNetEaseSkin(backend, "")
	if !ok || remembered.skin.SkinID != "changed-skin" {
		t.Fatal("skin registry update missed backend account")
	}
}

func TestIdentityRewriteRejectsTruncatedPackets(t *testing.T) {
	for _, pk := range [][]byte{{byte(packet.IDCommandRequest)}, {byte(packet.IDCommandRequest), 255}, {byte(packet.IDCommandOutput), 0, 1}, {byte(packet.IDPlayerSkin), 1}} {
		var err error
		if pk[0] == byte(packet.IDPlayerSkin) {
			_, err = remapSkinUUID(pk, uuid.Nil, uuid.New())
		} else {
			_, err = remapCommandUUID(pk, uuid.Nil, uuid.New())
		}
		if err == nil {
			t.Fatal("accepted truncated identity packet")
		}
	}
}

func TestPlayerListPreservesRecipientIdentityWithoutMergingOtherPlayers(t *testing.T) {
	client, ownBackend, otherBackend := uuid.New(), uuid.New(), uuid.New()
	identity, claims := netEaseLoginFixture(t, client)
	_, record, ok := OriginalLoginSkin(identity, claims)
	if !ok {
		t.Fatal("fixture invalid")
	}
	RememberSkin(ownBackend, record)
	defer ForgetSkin(ownBackend)
	// Both NetEase clients may start with the same original UUID, while their
	// backend identities and list entries must stay distinct for each viewer.
	other := record
	other.name = "OtherPlayer"
	other.uid = "987654321"
	RememberSkin(otherBackend, other)
	defer ForgetSkin(otherBackend)
	entries := []protocol.PlayerListEntry{{UUID: ownBackend, Username: record.name, Skin: record.skin}, {UUID: otherBackend, Username: other.name, Skin: other.skin}}
	server := ServerConverter{ClientUUID: client, BackendUUID: ownBackend}
	visible := map[uuid.UUID]string{client: record.name}
	for _, action := range []byte{0, 0, 1} {
		list := packet.PlayerList{ActionType: action, Entries: entries}
		var body bytes.Buffer
		list.Marshal(protocol.NewWriter(&body, 0))
		wire := append([]byte{byte(packet.IDPlayerList)}, body.Bytes()...)
		converted, err := server.Convert(append([]byte{0xff}, gamebatch.Join([][]byte{wire})...))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := gamebatch.Decode(converted)
		packets, _ := gamebatch.Split(data)
		var got packet.PlayerList
		input := bytes.NewBuffer(packets[0][1:])
		got.Marshal(protocol.NewReader(input, 0, true))
		if got.Entries[0].UUID != client || got.Entries[1].UUID != otherBackend {
			t.Fatal("self UUID not restored or other player merged")
		}
		for _, entry := range got.Entries {
			if action == 0 {
				visible[entry.UUID] = entry.Username
			} else {
				delete(visible, entry.UUID)
			}
		}
		if action == 0 {
			if len(visible) != 2 || input.Len() != len(entries)*8 {
				t.Fatal("duplicate player or corrupted extension")
			}
			_, n := binary.Uvarint(packets[1])
			reader := protocol.NewReader(bytes.NewReader(packets[1][n:]), 0, true)
			var count uint32
			reader.Varuint32(&count)
			if count != 2 {
				t.Fatal("confirmation count")
			}
			for i, want := range []uuid.UUID{client, otherBackend} {
				var flag bool
				var id uuid.UUID
				var extra []byte
				reader.Bool(&flag)
				reader.UUID(&id)
				reader.ByteSlice(&extra)
				if !flag || id != want {
					t.Fatalf("confirmation %d uses wrong UUID", i)
				}
			}
		} else if len(visible) != 0 || input.Len() != 0 {
			t.Fatal("removal left stale player entries")
		}
	}
}
