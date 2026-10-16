package room

import (
	"sync"
	"time"
)

// A client may be downloading components when the first Ready arrives. Retry
// for a bounded period; stop as soon as it connects, leaves or the room closes.
type readyAttempt struct {
	network string
	stop    chan struct{}
}
type readyRetries struct {
	mu      sync.Mutex
	pending map[uint32]*readyAttempt
	done    <-chan struct{}
	send    func() error
	trace   func(string)
	delays  []time.Duration
}

func newReadyRetries(done <-chan struct{}, send func() error, trace func(string)) *readyRetries {
	return &readyRetries{pending: map[uint32]*readyAttempt{}, done: done, send: send, trace: trace, delays: []time.Duration{2 * time.Second, 3 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 20 * time.Second, 30 * time.Second, 30 * time.Second}}
}
func (r *readyRetries) start(player uint32, network string) {
	r.mu.Lock()
	if old := r.pending[player]; old != nil {
		close(old.stop)
		delete(r.pending, player)
	}
	if len(r.pending) >= 64 {
		r.mu.Unlock()
		return
	}
	a := &readyAttempt{network: network, stop: make(chan struct{})}
	r.pending[player] = a
	r.mu.Unlock()
	go func() {
		defer func() {
			r.mu.Lock()
			if r.pending[player] == a {
				delete(r.pending, player)
			}
			r.mu.Unlock()
		}()
		for _, delay := range r.delays {
			timer := time.NewTimer(delay)
			select {
			case <-r.done:
				timer.Stop()
				return
			case <-a.stop:
				timer.Stop()
				return
			case <-timer.C:
			}
			// Check cancellation again if a timer and connection arrived together.
			select {
			case <-r.done:
				return
			case <-a.stop:
				return
			default:
			}
			if r.send() != nil {
				return
			}
			if r.trace != nil {
				r.trace("已补发服务器就绪通知，等待客户端下载组件并连接")
			}
		}
	}()
}
func (r *readyRetries) stopPlayer(player uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.pending[player]; a != nil {
		close(a.stop)
		delete(r.pending, player)
	}
}
func (r *readyRetries) stopNetwork(network string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for player, a := range r.pending {
		if a.network == network {
			close(a.stop)
			delete(r.pending, player)
		}
	}
}
