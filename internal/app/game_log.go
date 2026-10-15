package app

import (
	"sync"

	"nemcbridge/internal/bridge"
)

// Server broadcasts arrive through every player connection. Elect one source
// so identical chat and death messages are not logged once per recipient.
type gameLogs struct {
	mu            sync.Mutex
	event         func(string)
	next, primary uint64
	sources       map[uint64]string
	online        map[string]bool
}

func newGameLogs(event func(string)) *gameLogs {
	return &gameLogs{event: event, sources: map[uint64]string{}, online: map[string]bool{}}
}

func (l *gameLogs) attach() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next++
	l.sources[l.next] = ""
	if l.primary == 0 {
		l.primary = l.next
	}
	return l.next
}

func (l *gameLogs) detach(id uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	name := l.sources[id]
	delete(l.sources, id)
	if name != "" && l.online[name] {
		l.online[name] = false
		l.event("玩家离开：" + name)
	}
	if l.primary == id {
		l.primary = 0
		for source := range l.sources {
			if l.primary == 0 || source < l.primary {
				l.primary = source
			}
		}
	}
}

func (l *gameLogs) observe(source uint64, e bridge.GameEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.sources[source]; !ok {
		return
	}
	if !e.Local && source != l.primary {
		return
	}
	switch e.Kind {
	case "join":
		if e.Local {
			l.sources[source] = e.Player
		}
		if !l.online[e.Player] {
			l.online[e.Player] = true
			l.event("玩家加入：" + e.Player)
		}
	case "leave":
		online, known := l.online[e.Player]
		if !known || online {
			l.online[e.Player] = false
			l.event("玩家离开：" + e.Player)
		}
	case "chat":
		l.event("聊天 <" + e.Player + "> " + e.Message)
	case "death":
		l.event("玩家死亡：" + e.Message)
	}
}
