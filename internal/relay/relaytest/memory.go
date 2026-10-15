package relaytest

import (
	"io"
	"sync"
)

// Packets is an in-memory relay.Stream for tests.
type Packets struct {
	In     chan []byte
	Out    chan []byte
	Closed chan struct{}
	once   sync.Once
}

func New() *Packets {
	return &Packets{make(chan []byte, 4), make(chan []byte, 4), make(chan struct{}), sync.Once{}}
}

func (p *Packets) ReadPacket() ([]byte, error) {
	select {
	case <-p.Closed:
		return nil, io.EOF
	case b := <-p.In:
		return b, nil
	}
}

func (p *Packets) Write(b []byte) (int, error) {
	select {
	case <-p.Closed:
		return 0, io.EOF
	case p.Out <- append([]byte(nil), b...):
		return len(b), nil
	}
}

func (p *Packets) Close() error { p.once.Do(func() { close(p.Closed) }); return nil }
