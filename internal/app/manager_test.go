package app

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"nemcbridge/internal/config"
	"nemcbridge/internal/netease"
	"nemcbridge/internal/secret"
	"nemcbridge/internal/session"
)

func TestManagerRejectsDuplicateBeforeLogin(t *testing.T) {
	token := "test-only-" + uuid.NewString()
	lease, err := session.Acquire(token)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	m := newManager(t.TempDir(), config.Config{}, []string{token}, log.New(io.Discard, "", 0))
	if err := m.start(); !errors.Is(err, session.ErrBusy) {
		t.Fatalf("start: %v", err)
	}
	if m.snapshot().Running || m.cancel != nil {
		t.Fatal("duplicate start changed manager state")
	}
}

func TestManagerUsesBuiltinPreloadWithoutImport(t *testing.T) {
	token := "test-only-" + uuid.NewString()
	for _, components := range []bool{false, true} {
		m := newManager(t.TempDir(), config.Config{Upstream: "127.0.0.1:0", RetrySeconds: 10, ForwardResourcePacks: true, NetEaseResourcePacks: components}, []string{token}, log.New(io.Discard, "", 0))
		p, err := m.resourceProfile()
		if err != nil || p == nil || len(p.ItemIDs) != 1 || p.ItemIDs[0] != netease.ClassicPreloadComponentID {
			t.Fatalf("builtin context (components=%v): %v", components, err)
		}
		if err := m.start(); err != nil {
			t.Fatal("builtin preload must not require an imported file", err)
		}
		m.stop()
		m.wait()
		lease, err := session.Acquire(token)
		if err != nil {
			t.Fatalf("session lock leaked: %v", err)
		}
		lease.Close()
	}
}

func TestManagerKeepsCustomPreloadOptional(t *testing.T) {
	m := newManager(t.TempDir(), config.Config{}, nil, log.New(io.Discard, "", 0))
	if p, err := m.resourceProfile(); p != nil || err != nil {
		t.Fatal("disabled resources unexpectedly load a profile", err)
	}
	p, err := netease.BuiltinResourceProfile()
	if err != nil {
		t.Fatal(err)
	}
	p.ItemIDs[0] = 12345
	path := filepath.Join(m.dir, netease.ResourceProfileFile)
	if err := netease.SaveResourceProfile(path, p); err != nil {
		t.Fatal(err)
	}
	m.cfg.ForwardResourcePacks = true
	if got, err := m.resourceProfile(); err != nil || got.ItemIDs[0] != netease.ClassicPreloadComponentID {
		t.Fatal("custom file applied without opting in", err)
	}
	m.cfg.NetEaseResourcePacks = true
	if got, err := m.resourceProfile(); err != nil || got.ItemIDs[0] != 12345 {
		t.Fatal("custom override ignored", err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.resourceProfile(); err == nil {
		t.Fatal("invalid explicit override silently replaced")
	}
}

func TestManagerReleasesSessionAfterStop(t *testing.T) {
	token := "test-only-" + uuid.NewString()
	m := newManager(t.TempDir(), config.Config{Upstream: "127.0.0.1:0", RetrySeconds: 10}, []string{token}, log.New(io.Discard, "", 0))
	if err := m.start(); err != nil {
		t.Fatal(err)
	}
	m.stop()
	m.wait()
	lease, err := session.Acquire(token)
	if err != nil {
		t.Fatalf("session still locked after stop: %v", err)
	}
	lease.Close()
}

func TestManagerStartsOneRoomPerToken(t *testing.T) {
	tokens := []string{"test-only-" + uuid.NewString(), "test-only-" + uuid.NewString()}
	m := newManager(t.TempDir(), config.Config{Upstream: "127.0.0.1:0", RetrySeconds: 10}, tokens, log.New(io.Discard, "", 0))
	if err := m.start(); err != nil {
		t.Fatal(err)
	}
	if s := m.snapshot(); len(s.Rooms) != 2 || s.TokenCount != 2 || !s.Running {
		t.Fatalf("expected two running rooms, got %+v", s.Rooms)
	}
	m.stop()
	m.wait()
	if s := m.snapshot(); s.Running || len(s.Rooms) != 0 {
		t.Fatal("rooms still reported after stop")
	}
	for _, token := range tokens {
		lease, err := session.Acquire(token)
		if err != nil {
			t.Fatalf("session still locked after stop: %v", err)
		}
		lease.Close()
	}
}

func TestManagerBusyTokenStartsNoRoom(t *testing.T) {
	free, busy := "test-only-"+uuid.NewString(), "test-only-"+uuid.NewString()
	lease, err := session.Acquire(busy)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	m := newManager(t.TempDir(), config.Config{Upstream: "127.0.0.1:0", RetrySeconds: 10}, []string{free, busy}, log.New(io.Discard, "", 0))
	err = m.start()
	if !errors.Is(err, session.ErrBusy) || !strings.Contains(err.Error(), "房间 2") {
		t.Fatalf("start: %v", err)
	}
	if m.snapshot().Running {
		t.Fatal("a busy token must not leave other rooms running")
	}
	other, err := session.Acquire(free)
	if err != nil {
		t.Fatalf("lock of the free token leaked: %v", err)
	}
	other.Close()
}

func TestConfigureRoomTokens(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	m := newManager(dir, cfg, []string{"token-a", "token-b"}, log.New(io.Discard, "", 0))
	keep := func(i int) roomToken { return roomToken{Keep: &i} }
	stored := func() []string {
		raw, err := secret.Load(filepath.Join(dir, "secrets.dat"))
		if err != nil {
			t.Fatal(err)
		}
		return parseTokens(raw)
	}
	rooms := []roomToken{keep(1), {Token: " token-c "}, keep(0)}
	if err := m.configure(cfg, nil, &rooms); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(stored(), ","); got != "token-b,token-c,token-a" || strings.Join(m.tokens, ",") != got {
		t.Fatalf("reordered tokens = %q", got)
	}
	for _, bad := range [][]roomToken{
		{keep(0), keep(0)},
		{keep(0), {Token: "token-b"}},
		{keep(7)},
		{{Token: "   "}},
		{{Token: "two tokens"}},
		make([]roomToken, maxRooms+1),
	} {
		if err := m.configure(cfg, nil, &bad); err == nil {
			t.Fatalf("accepted invalid rooms %+v", bad)
		}
	}
	// The single-token field of older clients replaces only the first room.
	legacy := "token-d"
	if err := m.configure(cfg, &legacy, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(stored(), ","); got != "token-d,token-c,token-a" {
		t.Fatalf("legacy token update = %q", got)
	}
	none := []roomToken{}
	if err := m.configure(cfg, nil, &none); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets.dat")); !os.IsNotExist(err) || m.snapshot().TokenSet {
		t.Fatal("removing every room should delete the stored tokens")
	}
}

func TestSnapshotCombinesRooms(t *testing.T) {
	m := newManager(t.TempDir(), config.Default(), []string{"a", "b"}, log.New(io.Discard, "", 0))
	m.status.Running = true
	m.status.Rooms = []RoomStatus{
		{Phase: "retrying", RoomID: 0, Players: 0, LastError: "鉴权失败"},
		{Phase: "online", RoomID: 4242, Players: 4},
	}
	m.status.Rooms[0].Players = 3
	s := m.snapshot()
	if s.Phase != "online" || s.Players != 7 || s.LastError != "[房间 1] 鉴权失败" {
		t.Fatalf("combined status = %q %d %q", s.Phase, s.Players, s.LastError)
	}
}
