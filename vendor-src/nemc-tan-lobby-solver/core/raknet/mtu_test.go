package raknet

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/raknet/internal/message"
)

// mtuTestDialer injects the asynchronous error Linux can deliver on recvfrom
// after an oversized UDP probe. All successful traffic uses real UDP sockets.
type mtuTestDialer struct {
	maxMTU  uint16
	onWrite bool
	err     error
	conn    *mtuTestConn
}

func (d *mtuTestDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	c, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	d.conn = &mtuTestConn{Conn: c, maxMTU: d.maxMTU, onWrite: d.onWrite, err: d.err}
	return d.conn, nil
}

type mtuTestConn struct {
	net.Conn
	maxMTU  uint16
	onWrite bool
	err     error
	pending atomic.Bool
	mu      sync.Mutex
	probes  []uint16
}

func (c *mtuTestConn) Write(b []byte) (int, error) {
	if len(b) > 0 && b[0] == message.IDOpenConnectionRequest1 {
		mtu := uint16(len(b) + 28)
		c.mu.Lock()
		c.probes = append(c.probes, mtu)
		c.mu.Unlock()
		if mtu > c.maxMTU {
			if c.onWrite {
				return 0, c.probeError("write")
			}
			c.pending.Store(true)
			return len(b), nil
		}
	}
	return c.Conn.Write(b)
}

func (c *mtuTestConn) Read(b []byte) (int, error) {
	if c.pending.Swap(false) {
		return 0, c.probeError("read")
	}
	return c.Conn.Read(b)
}

func (c *mtuTestConn) probeError(op string) error {
	err := c.err
	if err == nil {
		err = syscall.EMSGSIZE
	}
	return &net.OpError{Op: op, Net: "udp", Err: os.NewSyscallError(op, err)}
}

func TestMTUFallbackUDPHandshake(t *testing.T) {
	for _, tc := range []struct {
		name    string
		maxMTU  uint16
		onWrite bool
		want    []uint16
	}{
		{"normal", 1492, false, []uint16{1492}},
		{"linux_read_error", 1200, false, []uint16{1492, 1200}},
		{"write_error", 1200, true, []uint16{1492, 1200}},
		{"two_fallbacks", 576, false, []uint16{1492, 1200, 576}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := Listen("127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			d := &mtuTestDialer{maxMTU: tc.maxMTU, onWrite: tc.onWrite}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, err := (Dialer{UpstreamDialer: d}).DialContext(ctx, listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			server, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			// The dial timeout must not remain attached to an established socket.
			cancel()
			payload := bytes.Repeat([]byte("MTU"), 1024)
			if _, err := conn.Write(payload); err != nil {
				t.Fatal(err)
			}
			_ = server.SetReadDeadline(time.Now().Add(time.Second))
			b := make([]byte, len(payload))
			n, err := server.Read(b)
			if err != nil || !bytes.Equal(payload, b[:n]) {
				t.Fatalf("fragmented transfer failed: n=%d err=%v", n, err)
			}
			if conn.mtu != tc.maxMTU {
				t.Fatalf("MTU = %d, want %d", conn.mtu, tc.maxMTU)
			}
			d.conn.mu.Lock()
			defer d.conn.mu.Unlock()
			if !reflect.DeepEqual(d.conn.probes, tc.want) {
				t.Fatalf("probes = %v, want %v", d.conn.probes, tc.want)
			}
		})
	}
}

func TestMTUFallbackStopsOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want []uint16
	}{
		{"all_sizes_rejected", syscall.EMSGSIZE, []uint16{1492, 1200, 576}},
		{"other_errors_are_fatal", syscall.EACCES, []uint16{1492}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &mtuTestDialer{err: tc.err}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := (Dialer{UpstreamDialer: d}).DialContext(ctx, "127.0.0.1:9")
			if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
			if !reflect.DeepEqual(d.conn.probes, tc.want) {
				t.Fatalf("probes = %v, want %v", d.conn.probes, tc.want)
			}
			if _, err := d.conn.Conn.Write([]byte{1}); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("failed dial leaked UDP socket: %v", err)
			}
		})
	}
}

type mtuReplyConn struct {
	net.Conn
	writes int
}

func (c *mtuReplyConn) Write(b []byte) (int, error) { c.writes++; return len(b), nil }
func (c *mtuReplyConn) Read(b []byte) (int, error) {
	if c.writes == 1 {
		return 0, syscall.EMSGSIZE
	}
	// A delayed response to the 1492-byte probe arrives after fallback to 1200.
	reply, _ := (&message.OpenConnectionReply1{ServerGUID: 1, MTU: 1492}).MarshalBinary()
	return copy(b, reply), nil
}
func (*mtuReplyConn) SetReadDeadline(time.Time) error { return nil }

func TestMTUFallbackLateReply(t *testing.T) {
	c := &mtuReplyConn{}
	state := &connState{conn: c, protocol: protocolVersion}
	if err := state.discoverMTU(context.Background()); err != nil {
		t.Fatal(err)
	}
	if state.mtu != 1200 {
		t.Fatalf("late reply undid fallback: MTU = %d", state.mtu)
	}
}

type mtuTimeoutConn struct{ mtuReplyConn }

func (*mtuTimeoutConn) Read([]byte) (int, error) { return 0, os.ErrDeadlineExceeded }

func TestMTUFallbackNoReply(t *testing.T) {
	c := &mtuTimeoutConn{}
	state := &connState{conn: c, protocol: protocolVersion}
	if err := state.discoverMTU(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if c.writes != 9 {
		t.Fatalf("probe attempts = %d, want 9", c.writes)
	}
}

func TestMTUCancellationClosesSocket(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (Dialer{UpstreamDialer: mtuPipeDialer{local}}).DialContext(ctx, "unused:1")
		done <- err
	}()
	// Wait until the dialer is sending its first probe, then cancel a context
	// without a deadline. Cancellation must unblock the underlying Write/Read.
	b := make([]byte, 1492)
	if _, err := remote.Read(b); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt handshake")
	}
}

type mtuPipeDialer struct{ net.Conn }

func (d mtuPipeDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return d.Conn, nil
}
