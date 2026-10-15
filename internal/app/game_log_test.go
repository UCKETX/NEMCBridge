package app

import (
	"bytes"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"nemcbridge/internal/bridge"
	"nemcbridge/internal/config"
)

func TestGameLogBroadcastsAndPrimaryHandoff(t *testing.T) {
	var lines []string
	l := newGameLogs(func(s string) { lines = append(lines, s) })
	a, b := l.attach(), l.attach()
	l.observe(a, bridge.GameEvent{Kind: "join", Player: "Alex", Local: true})
	l.observe(a, bridge.GameEvent{Kind: "join", Player: "Alex"})
	l.observe(b, bridge.GameEvent{Kind: "join", Player: "Steve", Local: true})
	chat := bridge.GameEvent{Kind: "chat", Player: "Alex", Message: "hello"}
	l.observe(a, chat)
	l.observe(b, chat)
	// Repeating a message intentionally on the elected source is not deduped.
	l.observe(a, chat)
	death := bridge.GameEvent{Kind: "death", Player: "Alex", Message: "Alex 摔死了"}
	l.observe(a, death)
	l.observe(b, death)
	l.detach(a)
	l.observe(b, bridge.GameEvent{Kind: "leave", Player: "Alex"})
	l.observe(b, chat)
	l.detach(b)
	l.observe(b, chat)
	want := "玩家加入：Alex\n玩家加入：Steve\n聊天 <Alex> hello\n聊天 <Alex> hello\n玩家死亡：Alex 摔死了\n玩家离开：Alex\n聊天 <Alex> hello\n玩家离开：Steve"
	if got := strings.Join(lines, "\n"); got != want {
		t.Fatalf("logs:\n%s", got)
	}
}

func TestDebugLogsDefaultOffLiveToggleAndPersistence(t *testing.T) {
	var output bytes.Buffer
	dir := t.TempDir()
	m := newManager(dir, config.Default(), "test-secret-token", log.New(&output, "", 0))
	m.event("房间已上线")
	m.debug("世界数据包：PacketID=1")
	if len(m.snapshot().Events) != 1 || strings.Contains(output.String(), "PacketID") {
		t.Fatal("default logs contain diagnostics")
	}
	if err := m.setDebug(true); err != nil {
		t.Fatal(err)
	}
	m.debug("test-secret-token")
	if !strings.Contains(output.String(), "[DEBUG]") || strings.Contains(output.String(), "test-secret-token") {
		t.Fatal("debug gating or redaction failed")
	}
	c, err := config.Load(filepath.Join(dir, "config.json"))
	if err != nil || !c.Debug {
		t.Fatal("debug setting was not saved")
	}
	if err := m.setDebug(false); err != nil {
		t.Fatal(err)
	}
	n := len(m.snapshot().Events)
	m.debug("hidden")
	m.event("玩家加入：Alex")
	if len(m.snapshot().Events) != n+1 || strings.Contains(output.String(), "hidden") {
		t.Fatal("disabled debug still logged diagnostics")
	}
}

func TestDebugAPIWorksWhileRoomIsRunning(t *testing.T) {
	m := newManager(t.TempDir(), config.Default(), "", log.New(&bytes.Buffer{}, "", 0))
	m.cancel = func() {} // Other configuration changes are locked while running.
	h := makeHandler(m, testConsoleAuth(t))
	if w := testConsoleRequest(h, http.MethodPost, "/api/debug", `{"enabled":true}`, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous debug: %d", w.Code)
	}
	login := testConsoleRequest(h, http.MethodPost, "/api/login", `{"username":"admin","password":"test-admin-password"}`, nil)
	cookie := login.Result().Cookies()[0]
	for _, body := range []string{`{}`, `{"enabled":"true"}`, `{"enabled":true,"extra":1}`} {
		if w := testConsoleRequest(h, http.MethodPost, "/api/debug", body, cookie); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid body %s: %d", body, w.Code)
		}
	}
	if w := testConsoleRequest(h, http.MethodPost, "/api/debug", `{"enabled":true}`, cookie); w.Code != http.StatusOK || !m.snapshot().Config.Debug {
		t.Fatalf("live toggle: %d", w.Code)
	}
}
