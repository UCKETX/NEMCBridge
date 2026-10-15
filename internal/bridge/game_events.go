package bridge

import (
	"bytes"
	"encoding/binary"
	"strings"
	"unicode"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nemcbridge/internal/gamebatch"
)

type GameEvent struct {
	Kind, Player, Message string
	Local                 bool // The connected player's initialization, not a server broadcast.
}

func gameLogText(s string, limit int) string {
	out := make([]rune, 0, min(len(s), limit))
	skip := false
	for _, r := range s {
		if skip {
			skip = false
			continue
		}
		if r == '§' {
			skip = true
			continue
		}
		if unicode.IsControl(r) {
			r = ' '
		}
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return strings.TrimSpace(string(out))
}

func (b *Bridge) observeClientEvents(compressed []byte) {
	if b.OnGameEvent == nil || b.playerJoined || b.playerName == "" {
		return
	}
	packets, err := gameEventPackets(compressed)
	if err != nil {
		return
	}
	for _, pk := range packets {
		id, n := binary.Uvarint(pk)
		if n > 0 && id&0x3ff == packet.IDSetLocalPlayerAsInitialised {
			b.playerJoined = true
			b.OnGameEvent(GameEvent{Kind: "join", Player: gameLogText(b.playerName, 80), Local: true})
			return
		}
	}
}

func gameEventPackets(compressed []byte) ([][]byte, error) {
	batch, err := gamebatch.Decode(compressed)
	if err != nil {
		return nil, err
	}
	return gamebatch.Split(batch)
}

func (b *Bridge) observeServerEvents(compressed []byte) {
	if b.OnGameEvent == nil {
		return
	}
	packets, err := gameEventPackets(compressed)
	if err != nil {
		return
	}
	for _, pk := range packets {
		b.observeServerPacket(pk)
	}
}

func (b *Bridge) observeServerPacket(pk []byte) {
	if b.OnGameEvent == nil {
		return
	}
	id, n := binary.Uvarint(pk)
	if n <= 0 || id&0x3ff != packet.IDText || len(pk) > 64<<10 {
		return
	}
	if e, ok := decodeGameText(pk[n:]); ok {
		b.OnGameEvent(e)
	}
}

// Logging must never affect packet forwarding, even for malformed or custom
// Text packets. Read the standard server packet before NetEase conversion.
func decodeGameText(data []byte) (e GameEvent, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	var text packet.Text
	text.Marshal(protocol.NewReader(bytes.NewBuffer(data), 0, true))
	if text.TextType == packet.TextTypeChat {
		name := gameLogText(text.SourceName, 80)
		if name == "" {
			name = "玩家"
		}
		return GameEvent{Kind: "chat", Player: name, Message: gameLogText(text.Message, 512)}, true
	}
	if text.TextType != packet.TextTypeTranslation {
		return e, false
	}
	key := strings.TrimPrefix(text.Message, "%")
	if len(text.Parameters) == 0 {
		return e, false
	}
	name := gameLogText(text.Parameters[0], 80)
	if name == "" {
		return e, false
	}
	switch key {
	case "multiplayer.player.joined":
		return GameEvent{Kind: "join", Player: name}, true
	case "multiplayer.player.left":
		return GameEvent{Kind: "leave", Player: name}, true
	}
	if strings.HasPrefix(key, "death.") {
		message := name + " 死亡"
		causes := map[string]string{
			"death.attack.drown": " 溺水身亡", "death.attack.fall": " 摔死了",
			"death.attack.lava": " 试图在熔岩里游泳", "death.attack.inFire": " 被火焰吞噬",
			"death.attack.onFire": " 被烧死了", "death.attack.starve": " 饿死了",
			"death.attack.inWall": " 在墙里窒息", "death.attack.outOfWorld": " 掉出了世界",
			"death.attack.freeze": " 冻死了", "death.attack.magic": " 被魔法杀死了",
			"death.attack.suicide": " 死亡", "death.attack.generic": " 死亡",
		}
		if cause, found := causes[key]; found {
			message = name + cause
		} else if len(text.Parameters) > 1 {
			other := gameLogText(text.Parameters[1], 80)
			if other != "" {
				message = name + " 死亡（" + other + "）"
			}
		}
		return GameEvent{Kind: "death", Player: name, Message: message}, true
	}
	return e, false
}
