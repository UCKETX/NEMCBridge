package bridge

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/netease"
)

func gameTextBatch(text *packet.Text) []byte {
	var buf bytes.Buffer
	text.Marshal(protocol.NewWriter(&buf, 0))
	pk := binary.AppendUvarint(nil, uint64(packet.IDText))
	pk = append(pk, buf.Bytes()...)
	return append([]byte{0xff}, gamebatch.Join([][]byte{pk})...)
}

func TestGameEventsContinueAfterWorldInitialization(t *testing.T) {
	var got []GameEvent
	b := &Bridge{reads: 2000, OnGameEvent: func(e GameEvent) { got = append(got, e) }}
	for _, text := range []*packet.Text{
		{TextType: packet.TextTypeChat, SourceName: "Alex", Message: "你好🌍"},
		{TextType: packet.TextTypeTranslation, Message: "%multiplayer.player.joined", Parameters: []string{"Steve"}},
		{TextType: packet.TextTypeTranslation, Message: "multiplayer.player.left", Parameters: []string{"Steve"}},
		{TextType: packet.TextTypeTranslation, Message: "death.attack.drown", Parameters: []string{"Alex"}},
		{TextType: packet.TextTypeTranslation, Message: "commands.time.set", Parameters: []string{"1000"}},
	} {
		b.observeServerEvents(gameTextBatch(text))
	}
	want := []GameEvent{
		{Kind: "chat", Player: "Alex", Message: "你好🌍"},
		{Kind: "join", Player: "Steve"},
		{Kind: "leave", Player: "Steve"},
		{Kind: "death", Player: "Alex", Message: "Alex 溺水身亡"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v", got)
	}
}

type gameEventStream struct{ packet []byte }

func (s *gameEventStream) ReadPacket() ([]byte, error) { return s.packet, nil }
func (*gameEventStream) Write(b []byte) (int, error)   { return len(b), nil }
func (*gameEventStream) Close() error                  { return nil }

func TestGameEventsFromEncryptedServerPreserveForwarding(t *testing.T) {
	plain := gameTextBatch(&packet.Text{TextType: packet.TextTypeChat, SourceName: "Alex", Message: "你好🌍"})
	key := [32]byte{1, 2, 3}
	s := &gameEventStream{packet: append([]byte{0xfe}, newGameCipher(key).encrypt(plain)...)}
	b, err := New(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	b.recv = newGameCipher(key)
	b.reads = 2000
	var events []GameEvent
	b.OnGameEvent = func(e GameEvent) { events = append(events, e) }
	got, err := b.ReadPacket()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := new(netease.ServerConverter).Convert(plain)
	if err != nil || !bytes.Equal(got, append([]byte{0xfe}, expected...)) {
		t.Fatalf("observer changed forwarded game packet: %v", err)
	}
	if len(events) != 1 || events[0].Message != "你好🌍" {
		t.Fatalf("events = %#v", events)
	}
}

func TestGameEventsIgnoreMalformedAndBoundUntrustedText(t *testing.T) {
	if _, ok := decodeGameText([]byte{packet.TextTypeChat}); ok {
		t.Fatal("accepted truncated Text")
	}
	text := &packet.Text{TextType: packet.TextTypeChat, SourceName: "§aAlex\n\x1b", Message: strings.Repeat("中", 600) + "\n"}
	var buf bytes.Buffer
	text.Marshal(protocol.NewWriter(&buf, 0))
	e, ok := decodeGameText(buf.Bytes())
	if !ok || e.Player != "Alex" || len([]rune(e.Message)) != 512 || strings.ContainsAny(e.Player, "\n\x1b§") {
		t.Fatalf("unsafe event: %#v", e)
	}
	var got int
	b := &Bridge{OnGameEvent: func(GameEvent) { got++ }}
	b.observeServerEvents([]byte{0xff, 0xff})
	if got != 0 {
		t.Fatal("malformed batch generated events")
	}
}

func TestGameJoinWaitsForWorldInitialization(t *testing.T) {
	var got []GameEvent
	b := &Bridge{playerName: "Alex", OnGameEvent: func(e GameEvent) { got = append(got, e) }}
	b.observeClientEvents(gameTextBatch(&packet.Text{TextType: packet.TextTypeChat, SourceName: "Alex", Message: "hello"}))
	pk := binary.AppendUvarint(nil, uint64(packet.IDSetLocalPlayerAsInitialised))
	pk = binary.AppendUvarint(pk, 123)
	batch := append([]byte{0xff}, gamebatch.Join([][]byte{pk})...)
	b.observeClientEvents(batch)
	b.observeClientEvents(batch)
	if len(got) != 1 || got[0].Kind != "join" || got[0].Player != "Alex" || !got[0].Local {
		t.Fatalf("events = %#v", got)
	}
}
