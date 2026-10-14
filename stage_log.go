package main

import (
	"context"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	"github.com/pion/logging"
	"io"
	"log/slog"
	"strings"
)

// Record negotiation stages, excluding SDP, candidates and TURN credentials.
type stageLog struct{ trace func(string) }

func (h stageLog) Enabled(context.Context, slog.Level) bool { return true }
func (h stageLog) Handle(_ context.Context, r slog.Record) error {
	message := "WebRTC：" + r.Message
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "error" {
			message += "；" + a.Value.String()
		}
		return true
	})
	if h.trace != nil {
		h.trace(message)
	}
	return nil
}
func (h stageLog) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h stageLog) WithGroup(string) slog.Handler      { return h }

type tracedSignaling struct {
	nethernet.Signaling
	trace func(string)
}

func (s tracedSignaling) Notify(n nethernet.Notifier) func() {
	return s.Signaling.Notify(tracedNotifier{Notifier: n, trace: s.trace})
}
func (s tracedSignaling) Signal(p *nethernet.Signal) error {
	if s.trace != nil {
		s.trace("发送 WebRTC 信令：" + p.Type)
	}
	return s.Signaling.Signal(p)
}

type tracedNotifier struct {
	nethernet.Notifier
	trace func(string)
}

func (n tracedNotifier) NotifySignal(p *nethernet.Signal) {
	if n.trace != nil {
		n.trace("收到 WebRTC 信令：" + p.Type)
	}
	n.Notifier.NotifySignal(p)
}

type transportLogs struct{ trace func(string) }

func (f transportLogs) NewLogger(scope string) logging.LeveledLogger {
	if scope != "sctp" {
		return logging.NewDefaultLeveledLoggerForScope(scope, logging.LogLevelDisabled, io.Discard)
	}
	return logging.NewDefaultLeveledLoggerForScope(scope, logging.LogLevelDebug, transportWriter{trace: f.trace})
}

type transportWriter struct{ trace func(string) }

func (w transportWriter) Write(b []byte) (int, error) {
	s := strings.TrimSpace(string(b))
	if strings.Contains(strings.ToLower(s), "cookie") {
		return len(b), nil
	}
	// Only handshake metadata. Exclude cookie and game payload details.
	for _, key := range []string{"checksum", "unable to parse SCTP", "failed validating", "sending INIT", "chunkInit", "handshake", "readLoop exited"} {
		if strings.Contains(s, key) {
			if w.trace != nil {
				w.trace(s)
			}
			break
		}
	}
	return len(b), nil
}
