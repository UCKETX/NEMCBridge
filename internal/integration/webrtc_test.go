package integration

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	bdsraknet "github.com/sandertv/go-raknet"

	"lobbykeeper/internal/bridge"
	"lobbykeeper/internal/gamebatch"
	"lobbykeeper/internal/relay"
)

// Local signaling lets the integration test exercise real ICE/DTLS/SCTP without
// authenticating a second NetEase account or changing a public room.
type localSignal struct {
	id       uint64
	peer     *localSignal
	incoming chan *nethernet.Signal
	closed   chan struct{}
	once     sync.Once
}

// Present the NetherNet frontend with a RakNet envelope to reuse the complete
// world exchange test without bypassing ICE, DTLS, SCTP or fragmentation.
type framedTestFrontend struct{ *nethernet.Conn }

func (s framedTestFrontend) Write(p []byte) (int, error) {
	n, err := s.Conn.Write(p[1:])
	return n + 1, err
}
func (s framedTestFrontend) ReadPacket() ([]byte, error) {
	p, err := s.Conn.ReadPacket()
	return append([]byte{0xfe}, p...), err
}

func localWebRTCFrontend(t *testing.T, ctx context.Context, backend relay.Stream) relay.Stream {
	t.Helper()
	a := &localSignal{id: 1, incoming: make(chan *nethernet.Signal, 128), closed: make(chan struct{})}
	b := &localSignal{id: 2, incoming: make(chan *nethernet.Signal, 128), closed: make(chan struct{})}
	a.peer, b.peer = b, a
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	listener, err := (nethernet.ListenConfig{Log: logger}).Listen(b)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		incoming, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		result <- relay.Relay(ctx, incoming.(*nethernet.Conn), backend)
	}()
	client, err := (nethernet.Dialer{Log: logger}).DialContext(ctx, 2, a)
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	stop := context.AfterFunc(ctx, func() { client.Close(); listener.Close() })
	t.Cleanup(func() {
		stop()
		client.Close()
		listener.Close()
		select {
		case <-result:
		case <-time.After(3 * time.Second):
			t.Error("world relay did not close")
		}
	})
	return framedTestFrontend{client}
}

func (s *localSignal) Signal(signal *nethernet.Signal) error {
	copy := *signal
	copy.NetworkID = s.id
	select {
	case <-s.closed:
		return net.ErrClosed
	case s.peer.incoming <- &copy:
		return nil
	}
}
func (s *localSignal) Notify(n nethernet.Notifier) func() {
	go func() {
		for {
			select {
			case p := <-s.incoming:
				n.NotifySignal(p)
			case <-s.closed:
				n.NotifyError(nethernet.ErrSignalingStopped)
				return
			}
		}
	}()
	return func() { s.once.Do(func() { close(s.closed) }) }
}
func (s *localSignal) Credentials(context.Context) (*nethernet.Credentials, error) { return nil, nil }
func (s *localSignal) NetworkID() uint64                                           { return s.id }
func (s *localSignal) PongData([]byte)                                             {}

func TestLocalWebRTCToBDS121124(t *testing.T) {
	addr := os.Getenv("LOBBYKEEPER_BDS_TEST")
	if addr == "" {
		t.Skip("set LOBBYKEEPER_BDS_TEST to run real BDS integration")
	}
	version, err := bridge.PingUpstream(context.Background(), addr)
	if err != nil || version != "1.21.124" {
		t.Fatalf("expected real BDS 1.21.124, got %q: %v", version, err)
	}
	a := &localSignal{id: 1, incoming: make(chan *nethernet.Signal, 128), closed: make(chan struct{})}
	b := &localSignal{id: 2, incoming: make(chan *nethernet.Signal, 128), closed: make(chan struct{})}
	a.peer = b
	b.peer = a
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	listener, err := (nethernet.ListenConfig{Log: logger}).Listen(b)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		incoming, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		upstream, err := bdsraknet.DialContext(ctx, addr)
		if err != nil {
			incoming.Close()
			result <- err
			return
		}
		backend, err := bridge.New(upstream, nil)
		if err != nil {
			incoming.Close()
			upstream.Close()
			result <- err
			return
		}
		result <- relay.Relay(ctx, incoming.(*nethernet.Conn), backend)
	}()
	client, err := (nethernet.Dialer{Log: logger}).DialContext(ctx, 2, a)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	context.AfterFunc(ctx, func() { client.Close() })
	// Bedrock RequestNetworkSettings: VarUInt packet ID 193, protocol 860 (BE),
	// wrapped in a length-prefixed uncompressed game batch. No RakNet 0xfe here.
	if _, err = client.Write([]byte{6, 0xc1, 1, 0, 0, 3, 0x5c}); err != nil {
		t.Fatal(err)
	}
	reply := make(chan []byte, 1)
	readErr := make(chan error, 1)
	go func() {
		data, err := client.ReadPacket()
		if err != nil {
			readErr <- err
			return
		}
		reply <- data
	}()
	select {
	case data := <-reply:
		reader := bytes.NewReader(data)
		_, err = binary.ReadUvarint(reader)
		if err != nil {
			t.Fatalf("invalid batch: %x", data)
		}
		id, err := binary.ReadUvarint(reader)
		if err != nil || id&0x3ff != 143 {
			t.Fatalf("BDS did not return NetworkSettings (143): %x, %v", data, err)
		}
		t.Log(fmt.Sprintf("real WebRTC -> RakNet -> BDS %s protocol 860 NetworkSettings round trip passed", version))
	case err := <-readErr:
		select {
		case relayErr := <-result:
			t.Fatalf("client read failed: %v; relay: %v", err, relayErr)
		case <-time.After(time.Second):
			t.Fatalf("client read failed: %v", err)
		}
	case err := <-result:
		t.Fatalf("relay failed before response: %v", err)
	case <-ctx.Done():
		t.Fatal("BDS NetworkSettings response timed out")
	}
	if _, err = client.Write(makeOfflineTestLogin(t)[1:]); err != nil {
		t.Fatal(err)
	}
	loggedIn := false
	for i := 0; i < 8 && !loggedIn; i++ {
		data, err := client.ReadPacket()
		if err != nil {
			t.Fatal(err)
		}
		batch, err := gamebatch.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		packets, err := gamebatch.Split(batch)
		if err != nil {
			t.Fatal(err)
		}
		for _, pk := range packets {
			id, n := binary.Uvarint(pk)
			if id&0x3ff == 3 {
				t.Fatal("RakNet encryption handshake reached NetherNet client")
			}
			if id&0x3ff == 2 && len(pk) >= n+4 && binary.BigEndian.Uint32(pk[n:]) == 0 {
				loggedIn = true
			}
		}
	}
	if !loggedIn {
		t.Fatal("NetherNet client did not receive login success")
	}
	t.Log("real WebRTC -> encryption bridge -> BDS login passed")
	cancel()
	client.Close()
	listener.Close()
	select {
	case <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("integration relay did not clean up")
	}
}
