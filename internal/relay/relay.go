package relay

import (
	"context"
	"io"
)

// Relay preserves packet boundaries and the game login/encryption bytes.
// NetherNet omits the RakNet 0xfe envelope, so only that byte is translated.
type Stream interface {
	ReadPacket() ([]byte, error)
	Write([]byte) (int, error)
	Close() error
}

func Relay(ctx context.Context, client, server Stream) error {
	ended := make(chan error, 2)
	copyPackets := func(from, to Stream, toRaknet bool) {
		for {
			b, err := from.ReadPacket()
			if err != nil {
				ended <- err
				return
			}
			if toRaknet {
				b = append([]byte{0xfe}, b...)
			} else {
				if len(b) == 0 || b[0] != 0xfe {
					continue
				}
				b = b[1:]
			}
			n, err := to.Write(b)
			if err == nil && n != len(b) {
				err = io.ErrShortWrite
			}
			if err != nil {
				ended <- err
				return
			}
		}
	}
	go copyPackets(client, server, true)
	go copyPackets(server, client, false)
	var err error
	cancelled := false
	select {
	case err = <-ended:
	case <-ctx.Done():
		err = ctx.Err()
		cancelled = true
	}
	client.Close()
	server.Close()
	// Both reads unblock on Close. Wait so a stopped room has no live relays.
	if cancelled {
		<-ended
		<-ended
	} else {
		<-ended
	}
	return err
}
