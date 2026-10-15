package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAccountSessionAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	lease, err := AcquireAt(dir, "test-only-credential")
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
	other, err := AcquireAt(dir, "other-test-credential")
	if err != nil {
		t.Fatal(err)
	}
	other.Close()
	lease.Close()
	lease.Close()
	runHelper("acquired")
	// The helper exits without Close, simulating a process crash. The OS must
	// release its lock so no stale file prevents the next start.
	next, err := AcquireAt(dir, "test-only-credential")
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
	lease, err := AcquireAt(dir, "test-only-credential")
	if errors.Is(err, ErrBusy) {
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

func TestLegacyConsoleBlocksCheckBeforeAuth(t *testing.T) {
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
			err := RejectRunningRoom(context.Background(), port)
			if running && !errors.Is(err, ErrBusy) || !running && err != nil {
				t.Fatalf("guard: %v", err)
			}
		})
	}
}
