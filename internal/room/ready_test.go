package room

import (
	"testing"
	"time"
)

func TestReadyRetriesStopOnConnectLeaveAndDuplicateJoin(t *testing.T) {
	done := make(chan struct{})
	defer close(done)
	r := newReadyRetries(done, func() error { t.Error("cancelled retry sent a packet"); return nil }, nil)
	r.delays = []time.Duration{time.Hour}
	r.start(1, "first")
	r.mu.Lock()
	first := r.pending[1]
	r.mu.Unlock()
	r.start(1, "second")
	select {
	case <-first.stop:
	default:
		t.Fatal("duplicate join did not cancel earlier retries")
	}
	r.stopNetwork("second")
	r.mu.Lock()
	n := len(r.pending)
	r.mu.Unlock()
	if n != 0 {
		t.Fatal("connected client retained retries")
	}
	r.start(2, "third")
	r.stopPlayer(2)
	r.mu.Lock()
	n = len(r.pending)
	r.mu.Unlock()
	if n != 0 {
		t.Fatal("departed client retained retries")
	}
}

func TestReadyRetriesAreFiniteAndStopOnRoomClose(t *testing.T) {
	done := make(chan struct{})
	sent := make(chan struct{}, 4)
	r := newReadyRetries(done, func() error { sent <- struct{}{}; return nil }, nil)
	r.delays = []time.Duration{0, 0}
	r.start(1, "network")
	for range 2 {
		select {
		case <-sent:
		case <-time.After(time.Second):
			t.Fatal("retry not sent")
		}
	}
	deadline := time.After(time.Second)
	for {
		r.mu.Lock()
		n := len(r.pending)
		r.mu.Unlock()
		if n == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("retry window did not finish")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	select {
	case <-sent:
		t.Fatal("unexpected extra retry")
	default:
	}
	r.delays = []time.Duration{time.Hour}
	r.start(2, "later")
	close(done)
	deadline = time.After(time.Second)
	for {
		r.mu.Lock()
		n := len(r.pending)
		r.mu.Unlock()
		if n == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("room close leaked retries")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
