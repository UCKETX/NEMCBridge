package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccountSessionAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	lease, err := acquireAccountSessionAt(dir, "test-only-credential")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Close)
	runHelper := func(want string) {
		t.Helper()
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestAccountSessionHelper$")
		cmd.Env = append(os.Environ(), "LOBBYKEEPER_LOCK_TEST_DIR="+dir)
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.HasPrefix(string(out), want+"\n") {
			t.Fatalf("helper: %v %q, want %s", err, out, want)
		}
	}
	runHelper("busy")
	// A different token may run concurrently in the same cache directory.
	other, err := acquireAccountSessionAt(dir, "other-test-credential")
	if err != nil {
		t.Fatal(err)
	}
	other.Close()
	lease.Close()
	lease.Close()
	runHelper("acquired")
	// The helper exits without Close, simulating a process crash. The OS must
	// release its lock so no stale file prevents the next start.
	next, err := acquireAccountSessionAt(dir, "test-only-credential")
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestAccountSessionHelper(t *testing.T) {
	dir := os.Getenv("LOBBYKEEPER_LOCK_TEST_DIR")
	if dir == "" {
		return
	}
	lease, err := acquireAccountSessionAt(dir, "test-only-credential")
	if errors.Is(err, errAccountSessionBusy) {
		fmt.Println("busy")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("acquired")
	// Keep the descriptor live until process exit; intentionally no Close.
	_ = lease
	os.Exit(0)
}

func TestManagerRejectsDuplicateBeforeLogin(t *testing.T) {
	token := "test-only-" + uuid.NewString()
	lease, err := acquireAccountSession(token)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	m := newManager(t.TempDir(), Config{}, token, log.New(io.Discard, "", 0))
	if err := m.start(); !errors.Is(err, errAccountSessionBusy) {
		t.Fatalf("start: %v", err)
	}
	if m.snapshot().Running || m.cancel != nil {
		t.Fatal("duplicate start changed manager state")
	}
}

func TestManagerReleasesSessionAfterStop(t *testing.T) {
	token := "test-only-" + uuid.NewString()
	m := newManager(t.TempDir(), Config{Upstream: "127.0.0.1:0", RetrySeconds: 10}, token, log.New(io.Discard, "", 0))
	if err := m.start(); err != nil {
		t.Fatal(err)
	}
	m.stop()
	m.wait()
	lease, err := acquireAccountSession(token)
	if err != nil {
		t.Fatalf("session still locked after stop: %v", err)
	}
	lease.Close()
}

func TestLegacyConsoleBlocksDiagnosticBeforeAuth(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(strconv.FormatBool(running), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/status" {
					t.Error("unexpected endpoint")
				}
				fmt.Fprintf(w, `{"running":%v}`, running)
			}))
			defer server.Close()
			port := server.Listener.Addr().(*net.TCPAddr).Port
			err := rejectRunningRoom(context.Background(), port)
			if running && !errors.Is(err, errAccountSessionBusy) || !running && err != nil {
				t.Fatalf("guard: %v", err)
			}
		})
	}
}
