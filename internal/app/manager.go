package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	bdsraknet "github.com/sandertv/go-raknet"

	"nemcbridge/internal/auth"
	"nemcbridge/internal/bridge"
	"nemcbridge/internal/config"
	"nemcbridge/internal/fileutil"
	"nemcbridge/internal/friends"
	"nemcbridge/internal/redact"
	"nemcbridge/internal/relay"
	"nemcbridge/internal/room"
	"nemcbridge/internal/secret"
	"nemcbridge/internal/session"
)

type Event struct {
	Time    string `json:"time"`
	Message string `json:"message"`
}

type Status struct {
	Phase           string        `json:"phase"`
	Running         bool          `json:"running"`
	RoomID          uint32        `json:"room_id"`
	Players         int           `json:"players"`
	Account         string        `json:"account"`
	ProtocolID      uint8         `json:"protocol_id"`
	ConnectedAt     string        `json:"connected_at"`
	RetryAt         string        `json:"retry_at"`
	Attempts        int           `json:"attempts"`
	LastError       string        `json:"last_error"`
	UpstreamOnline  bool          `json:"upstream_online"`
	UpstreamVersion string        `json:"upstream_version"`
	TokenSet        bool          `json:"token_set"`
	Config          config.Config `json:"config"`
	Events          []Event       `json:"events"`
}

type Manager struct {
	mu       sync.Mutex
	cfg      config.Config
	token    string
	status   Status
	cancel   context.CancelFunc
	finished chan struct{}
	dir      string
	logger   *log.Logger
	exit     chan struct{}
	exitOnce sync.Once
}

func newManager(dir string, c config.Config, token string, logger *log.Logger) *Manager {
	return &Manager{dir: dir, cfg: c, token: token, logger: logger, exit: make(chan struct{}), status: Status{Phase: "stopped", Events: []Event{}}}
}

func (m *Manager) snapshot() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	s.Config = m.cfg
	s.TokenSet = m.token != ""
	s.Events = append([]Event{}, s.Events...)
	return s
}

func (m *Manager) event(message string) {
	m.logEvent(message, false)
}

func (m *Manager) debug(message string) {
	m.logEvent(message, true)
}

func (m *Manager) logEvent(message string, debug bool) {
	m.mu.Lock()
	if debug && !m.cfg.Debug {
		m.mu.Unlock()
		return
	}
	if debug {
		message = "[DEBUG] " + message
	}
	message = redact.Sanitize(message, m.token)
	m.status.Events = append(m.status.Events, Event{time.Now().Format("15:04:05"), message})
	if len(m.status.Events) > 120 {
		m.status.Events = m.status.Events[len(m.status.Events)-120:]
	}
	m.mu.Unlock()
	m.logger.Println(message)
}

func (m *Manager) setDebug(enabled bool) error {
	m.mu.Lock()
	c := m.cfg
	c.Debug = enabled
	if err := config.Save(filepath.Join(m.dir, "config.json"), c); err != nil {
		m.mu.Unlock()
		return err
	}
	m.cfg = c
	m.mu.Unlock()
	if enabled {
		m.event("Debug 模式已开启")
	} else {
		m.event("Debug 模式已关闭")
	}
	return nil
}

func (m *Manager) update(fn func(*Status)) {
	m.mu.Lock()
	fn(&m.status)
	m.mu.Unlock()
	m.persistStatus()
}

func (m *Manager) persistStatus() {
	s := m.snapshot()
	s.Config.RoomPassword = ""
	s.Events = nil
	b, _ := json.MarshalIndent(s, "", "  ")
	_ = fileutil.AtomicWrite(filepath.Join(m.dir, "logs", "status.json"), b, 0600)
}

func (m *Manager) configure(c config.Config, token *string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return fmt.Errorf("请先停止房间，再修改配置")
	}
	if c.WebPort != m.cfg.WebPort {
		return fmt.Errorf("控制台端口需关闭程序后在 config.json 修改")
	}
	if token != nil && strings.TrimSpace(*token) == "" {
		return fmt.Errorf("新令牌为空；保持原令牌请留空输入框")
	}
	if err := config.Save(filepath.Join(m.dir, "config.json"), c); err != nil {
		return err
	}
	if token != nil {
		if err := secret.Save(filepath.Join(m.dir, "secrets.dat"), strings.TrimSpace(*token)); err != nil {
			return err
		}
		m.token = strings.TrimSpace(*token)
	}
	m.cfg = c
	return nil
}

func (m *Manager) start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return nil
	}
	if m.token == "" {
		return fmt.Errorf("请先保存机器人令牌")
	}
	session, err := session.Acquire(m.token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.finished = make(chan struct{})
	m.status.Running = true
	m.status.LastError = ""
	m.status.Attempts = 0
	go m.run(ctx, m.cfg, m.token, m.finished, session)
	return nil
}

func (m *Manager) stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
}

func (m *Manager) wait() {
	m.mu.Lock()
	done := m.finished
	m.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (m *Manager) run(ctx context.Context, cfg config.Config, token string, done chan struct{}, session *session.Session) {
	defer func() {
		m.update(func(s *Status) { s.Running = false; s.Phase = "stopped"; s.RoomID = 0; s.Players = 0; s.RetryAt = "" })
		m.mu.Lock()
		session.Close()
		m.cancel = nil
		close(done)
		m.mu.Unlock()
		m.event("房间已停止")
	}()
	m.event("启动本地联机房主机器人")
	failures := 0
	for ctx.Err() == nil {
		m.update(func(s *Status) {
			s.Phase = "checking_upstream"
			s.RoomID = 0
			s.Players = 0
			s.RetryAt = ""
			s.Attempts++
		})
		version, err := bridge.PingUpstream(ctx, cfg.Upstream)
		if err == nil {
			m.update(func(s *Status) { s.UpstreamOnline = true; s.UpstreamVersion = version; s.Phase = "authenticating" })
			m.debug("本地 Bedrock 服务器在线，版本 " + version + "；开始鉴权")
			var info auth.CreateInfo
			authCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
			info, err = auth.FetchCreate(authCtx, cfg, token)
			cancel()
			if err == nil {
				m.update(func(s *Status) { s.Account = info.UserPlayerName; s.ProtocolID = info.ProtocolID; s.Phase = "creating" })
				m.debug(fmt.Sprintf("鉴权通过，房间协议 %d；连接开房节点", info.ProtocolID))
				var opened *room.Room
				createCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
				opened, err = room.Open(createCtx, cfg, info, m.debug)
				cancel()
				if err == nil {
					failures = 0
					m.update(func(s *Status) {
						s.Phase = "online"
						s.RoomID = opened.ID
						s.ConnectedAt = time.Now().Format(time.RFC3339)
						s.LastError = ""
					})
					m.event(fmt.Sprintf("房间 %d 已上线，房主机器人持续保活", opened.ID))
					err = m.serveRoom(ctx, cfg, opened, info)
					opened.Close()
				}
			}
		} else {
			m.update(func(s *Status) { s.UpstreamOnline = false })
			err = fmt.Errorf("本地服务器 %s 不可达，请先启动 Bedrock 服务器", cfg.Upstream)
		}
		if ctx.Err() != nil {
			return
		}
		failures++
		delay := time.Duration(cfg.RetrySeconds) * time.Second
		for i := 1; i < failures && delay < 90*time.Second; i++ {
			delay *= 2
		}
		if delay > 90*time.Second {
			delay = 90 * time.Second
		}
		if err == nil {
			err = net.ErrClosed
		}
		message := redact.Sanitize(err.Error(), token)
		m.update(func(s *Status) {
			s.Phase = "retrying"
			s.LastError = message
			s.RoomID = 0
			s.Players = 0
			s.RetryAt = time.Now().Add(delay).Format(time.RFC3339)
		})
		m.event(fmt.Sprintf("%s；%.0f 秒后重试", message, delay.Seconds()))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *Manager) serveRoom(ctx context.Context, cfg config.Config, r *room.Room, info auth.CreateInfo) error {
	gameLogs := newGameLogs(m.event)
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	friendDone := make(chan struct{})
	go func() {
		defer close(friendDone)
		if cfg.AutoAcceptFriends {
			friends.Run(sessionCtx, info.UserUniqueID, info.EncryptKeyBytes, m.event, m.debug)
		}
	}()
	defer func() { cancel(); <-friendDone }()
	var bridges sync.WaitGroup
	acceptDone := make(chan struct{})
	defer func() {
		cancel()
		r.Close()
		<-acceptDone
		bridges.Wait()
	}()
	acceptErrors := make(chan error, 1)
	go func() {
		defer close(acceptDone)
		for {
			conn, err := r.Accept()
			if err != nil {
				acceptErrors <- err
				return
			}
			if conn == nil {
				acceptErrors <- net.ErrClosed
				return
			}
			m.mu.Lock()
			full := m.status.Players >= cfg.MaxPlayers-1
			if !full {
				m.status.Players++
			}
			m.mu.Unlock()
			if full {
				conn.Close()
				continue
			}
			bridges.Add(1)
			go func(client *nethernet.Conn) {
				defer bridges.Done()
				defer client.Close()
				defer m.update(func(s *Status) {
					if s.Players > 0 {
						s.Players--
					}
				})
				m.debug("玩家连接已建立，正在转发到本地服务器")
				dialCtx, stop := context.WithTimeout(sessionCtx, 8*time.Second)
				server, err := bdsraknet.DialContext(dialCtx, cfg.Upstream)
				stop()
				if err != nil {
					m.event("玩家转发失败：本地服务器连接失败")
					return
				}
				defer server.Close()
				m.debug("本地服务器 RakNet 连接成功")
				backend, err := bridge.New(server, m.debug)
				if err != nil {
					m.event("无法生成后端连接密钥")
					return
				}
				defer backend.ForgetSkin()
				gameSource := gameLogs.attach()
				defer gameLogs.detach(gameSource)
				backend.OnGameEvent = func(e bridge.GameEvent) { gameLogs.observe(gameSource, e) }
				clientStream := &relay.Observed{Stream: client, Label: "玩家 → 本地服务器", Trace: m.debug}
				serverStream := &relay.Observed{Stream: backend, Label: "本地服务器 → 玩家", Trace: m.debug}
				err = relay.Relay(sessionCtx, clientStream, serverStream)
				m.debug(fmt.Sprintf("玩家转发连接已断开：%v；上行 %d 包，下行 %d 包", err, clientStream.Count, serverStream.Count))
				if err != nil && sessionCtx.Err() == nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
					m.event(fmt.Sprintf("玩家连接异常：%v", err))
				}
			}(conn.(*nethernet.Conn))
		}
	}()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cancel()
			return ctx.Err()
		case <-r.Done():
			cancel()
			return r.Error()
		case err := <-acceptErrors:
			cancel()
			return fmt.Errorf("WebRTC 信令断开: %w", err)
		case <-ticker.C:
			version, err := bridge.PingUpstream(sessionCtx, cfg.Upstream)
			m.update(func(s *Status) {
				s.UpstreamOnline = err == nil
				if err == nil {
					s.UpstreamVersion = version
					s.Phase = "online"
				} else {
					s.Phase = "upstream_offline"
				}
			})
		}
	}
}

func openLogger(dir string) (*log.Logger, *os.File, error) {
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0700); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "logs", "keeper.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, err
	}
	return log.New(f, "", log.LstdFlags), f, nil
}
