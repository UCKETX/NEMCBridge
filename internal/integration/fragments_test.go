package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"nemcbridge/internal/relay/relaytest"
)

func TestWebRTCMoreThan255Fragments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	backend := relaytest.New()
	stream := localWebRTCFrontend(t, ctx, backend)
	// The old uint8 fragment counter split this into two different messages.
	large := append([]byte{0xfe}, bytes.Repeat([]byte{0x83, 0x22, 0x71}, 900000)...)
	for _, size := range []int{len(large), 12} {
		want := large[:size]
		if _, err := stream.Write(want); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-backend.Out:
			if !bytes.Equal(got, want) {
				t.Fatalf("upstream message boundary changed: got=%d want=%d", len(got), len(want))
			}
		case <-ctx.Done():
			t.Fatal("upstream large transfer timed out")
		}
		backend.In <- want
		got, err := stream.ReadPacket()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("downstream message boundary changed: got=%d want=%d err=%v", len(got), len(want), err)
		}
	}
}
