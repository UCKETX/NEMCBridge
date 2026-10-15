package relay

import (
	"bytes"
	"context"
	"testing"
	"time"

	"nemcbridge/internal/relay/relaytest"
)

func TestRelayPacketBoundariesAndCancellation(t *testing.T) {
	client, server := relaytest.New(), relaytest.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Relay(ctx, client, server) }()
	for _, payload := range [][]byte{{0, 1, 2}, {0xff, 0xfe, 0, 0x80}, bytes.Repeat([]byte{0x13}, 15000)} {
		client.In <- append([]byte(nil), payload...)
		select {
		case b := <-server.Out:
			if !bytes.Equal(b, append([]byte{0xfe}, payload...)) {
				t.Fatal("client packet changed")
			}
		case <-time.After(time.Second):
			t.Fatal("client forwarding stalled")
		}
		server.In <- append([]byte{0xfe}, payload...)
		select {
		case b := <-client.Out:
			if !bytes.Equal(b, payload) {
				t.Fatal("server packet changed")
			}
		case <-time.After(time.Second):
			t.Fatal("server forwarding stalled")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("relay cancellation leaked readers")
	}
}

func TestRelayClosesBothPeersOnEOF(t *testing.T) {
	client, server := relaytest.New(), relaytest.New()
	done := make(chan error, 1)
	go func() { done <- Relay(context.Background(), client, server) }()
	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("EOF did not stop relay")
	}
	select {
	case <-server.Closed:
	default:
		t.Fatal("upstream left open")
	}
}
