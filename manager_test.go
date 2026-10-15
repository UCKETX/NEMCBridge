package main

import (
	"errors"
	"io"
	"log"
	"testing"

	"github.com/google/uuid"

	"lobbykeeper/internal/config"
	"lobbykeeper/internal/session"
)

func TestManagerRejectsDuplicateBeforeLogin(t *testing.T) {
	token := "test-only-" + uuid.NewString()
	lease, err := session.Acquire(token)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	m := newManager(t.TempDir(), config.Config{}, token, log.New(io.Discard, "", 0))
	if err := m.start(); !errors.Is(err, session.ErrBusy) {
		t.Fatalf("start: %v", err)
	}
	if m.snapshot().Running || m.cancel != nil {
		t.Fatal("duplicate start changed manager state")
	}
}

func TestManagerReleasesSessionAfterStop(t *testing.T) {
	token := "test-only-" + uuid.NewString()
	m := newManager(t.TempDir(), config.Config{Upstream: "127.0.0.1:0", RetrySeconds: 10}, token, log.New(io.Discard, "", 0))
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
