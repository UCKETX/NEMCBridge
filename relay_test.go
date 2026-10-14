package main

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

type memoryPackets struct {
	in     chan []byte
	out    chan []byte
	closed chan struct{}
	once   sync.Once
}

func newPackets() *memoryPackets {
	return &memoryPackets{make(chan []byte, 4), make(chan []byte, 4), make(chan struct{}), sync.Once{}}
}
func (p *memoryPackets) ReadPacket() ([]byte, error) {
	select {
	case <-p.closed:
		return nil, io.EOF
	case b := <-p.in:
		return b, nil
	}
}
func (p *memoryPackets) Write(b []byte) (int, error) {
	select {
	case <-p.closed:
		return 0, io.EOF
	case p.out <- append([]byte(nil), b...):
		return len(b), nil
	}
}
func (p *memoryPackets) Close() error { p.once.Do(func() { close(p.closed) }); return nil }
func TestRelayPacketBoundariesAndCancellation(t *testing.T) {
	client, server := newPackets(), newPackets()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- relayPackets(ctx, client, server) }()
	for _, payload := range [][]byte{{0, 1, 2}, {0xff, 0xfe, 0, 0x80}, bytes.Repeat([]byte{0x13}, 15000)} {
		client.in <- append([]byte(nil), payload...)
		select {
		case b := <-server.out:
			if !bytes.Equal(b, append([]byte{0xfe}, payload...)) {
				t.Fatal("client packet changed")
			}
		case <-time.After(time.Second):
			t.Fatal("client forwarding stalled")
		}
		server.in <- append([]byte{0xfe}, payload...)
		select {
		case b := <-client.out:
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
	client, server := newPackets(), newPackets()
	done := make(chan error, 1)
	go func() { done <- relayPackets(context.Background(), client, server) }()
	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("EOF did not stop relay")
	}
	select {
	case <-server.closed:
	default:
		t.Fatal("upstream left open")
	}
}
