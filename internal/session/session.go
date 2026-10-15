package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrBusy = errors.New("该机器人令牌已有本机会话，请先停止对应房间；同一账号重复登录可能使现有房间掉线")

// A process-scoped OS lock protects an active login, including its reconnects.
// The token is never written to disk. Keep the lock file after release: deleting
// it would let another process lock a different inode at the same path on Unix.
type Session struct {
	file *os.File
	once sync.Once
}

func Acquire(token string) (*Session, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, errors.New("无法确定机器人会话锁目录")
	}
	// Retain the legacy namespace so older running builds share the same lock.
	return AcquireAt(filepath.Join(cache, "LobbyKeeper", "sessions"), token)
}

func AcquireAt(dir, token string) (*Session, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("请先设置机器人令牌")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("无法创建机器人会话锁目录")
	}
	key := sha256.Sum256([]byte(strings.TrimSpace(token)))
	f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%x.lock", key)), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("无法打开机器人会话锁")
	}
	if err := trySessionFileLock(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Session{file: f}, nil
}

func (s *Session) Close() {
	s.once.Do(func() { s.file.Close() })
}

// Older builds do not hold a session lock. A check command must not invalidate
// their active account session merely to test authentication.
func RejectRunningRoom(ctx context.Context, port int) error {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/status", port), nil)
	if err != nil {
		return err
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil // No legacy console; the OS session lock is authoritative.
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		// Authenticated consoles belong to builds that hold the OS account lock.
		// Acquire already protects this check; only legacy public status needs probing.
		return nil
	}
	var state struct {
		Running bool `json:"running"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&state) != nil {
		return errors.New("无法确认现有控制台是否正在开房，已取消鉴权检查")
	}
	if state.Running {
		return ErrBusy
	}
	return nil
}
