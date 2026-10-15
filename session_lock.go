package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var errAccountSessionBusy = errors.New("该机器人令牌已有本机会话，请先停止对应房间或诊断程序；同一账号重复登录可能使现有房间掉线")

// A process-scoped OS lock protects an active login, including its reconnects.
// The token is never written to disk. Keep the lock file after release: deleting
// it would let another process lock a different inode at the same path on Unix.
type accountSession struct {
	file *os.File
	once sync.Once
}

func acquireAccountSession(token string) (*accountSession, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, errors.New("无法确定机器人会话锁目录")
	}
	return acquireAccountSessionAt(filepath.Join(cache, "LobbyKeeper", "sessions"), token)
}

func acquireAccountSessionAt(dir, token string) (*accountSession, error) {
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
	return &accountSession{file: f}, nil
}

func (s *accountSession) Close() {
	s.once.Do(func() { s.file.Close() })
}
