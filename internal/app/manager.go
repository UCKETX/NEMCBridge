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
	"nemcbridge/internal/netease"
	"nemcbridge/internal/redact"
	"nemcbridge/internal/relay"
	"nemcbridge/internal/room"
	"nemcbridge/internal/secret"
	"nemcbridge/internal/session"
)

// maxRooms bounds how many bot tokens (one NetEase room each) run at once.
const maxRooms = 8

type Event struct {
	Time    string `json:"time"`
	Message string `json:"message"`
}

// RoomStatus describes one NetEase lobby room hosted by one bot token.
type RoomStatus struct {
	Phase       string `json:"phase"`
	RoomID      uint32 `json:"room_id"`
	Players     int    `json:"players"`
	Account     string `json:"account"`
	ProtocolID  uint8  `json:"protocol_id"`
	ConnectedAt string `json:"connected_at"`
	RetryAt     string `json:"retry_at"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error"`
}

// Status keeps the single-room fields for existing clients: while running they
// mirror the first room, except Players which totals every room.
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
	TokenCount      int           `json:"token_count"`
	Rooms           []RoomStatus  `json:"rooms"`
	Config          config.Config `json:"config"`
	Events          []Event       `json:"events"`
}

type Manager struct {
	mu       sync.Mutex
	cfg      config.Config
	tokens   []string
	status   Status
	cancel   context.CancelFunc
	finished chan struct{}
	dir      string
	logger   *log.Logger
	exit     chan struct{}
	exitOnce sync.Once
}

// parseTokens splits stored or supplied tokens, one room per token. Tokens are
// Base64 and never contain whitespace, so any whitespace separates them.
func parseTokens(raw string) []string { return strings.Fields(raw) }

func newManager(dir string, c config.Config, tokens []string, logger *log.Logger) *Manager {
	return &Manager{dir: dir, cfg: c, tokens: tokens, logger: logger, exit: make(chan struct{}), status: Status{Phase: "stopped", Events: []Event{}}}
}

// phaseOrder ranks room phases for the combined status: a room players can
// join outranks one that is still connecting or failing.
var phaseOrder = []string{"online", "creating", "authenticating", "checking_upstream", "retrying", "upstream_offline", "stopped"}

func (m *Manager) snapshot() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	s.Config = m.cfg
	s.TokenSet = len(m.tokens) > 0
	s.TokenCount = len(m.tokens)
	s.Events = append([]Event{}, s.Events...)
	s.Rooms = append([]RoomStatus{}, s.Rooms...)
	if len(s.Rooms) > 0 {
		first := s.Rooms[0]
		s.RoomID, s.Account, s.ProtocolID = first.RoomID, first.Account, first.ProtocolID
		s.ConnectedAt, s.RetryAt, s.Attempts = first.ConnectedAt, first.RetryAt, first.Attempts
		s.Players, s.LastError = 0, ""
		best := len(phaseOrder)
		for i, r := range s.Rooms {
			s.Players += r.Players
			if s.LastError == "" && r.LastError != "" {
				s.LastError = roomLabel(i, len(s.Rooms)) + r.LastError
			}
			for rank, phase := range phaseOrder {
				if r.Phase == phase && rank < best {
					best = rank
				}
			}
		}
		if best < len(phaseOrder) {
			s.Phase = phaseOrder[best]
		}
	}
	return s
}

// roomLabel prefixes per-room messages only when several rooms run.
func roomLabel(i, n int) string {
	if n <= 1 {
		return ""
	}
	return fmt.Sprintf("[房间 %d] ", i+1)
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
	for _, token := range m.tokens {
		message = redact.Sanitize(message, token)
	}
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

func (m *Manager) updateRoom(i int, fn func(*RoomStatus)) {
	m.mu.Lock()
	if i < len(m.status.Rooms) {
		fn(&m.status.Rooms[i])
	}
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

// roomToken is one entry of the desired token list: keep an existing token by
// index (tokens never leave the server) or supply a new one.
type roomToken struct {
	Keep  *int   `json:"keep"`
	Token string `json:"token"`
}

// configure saves settings and, when given, the token list. rooms replaces the
// whole list; the older single token argument replaces the first room's token.
func (m *Manager) configure(c config.Config, token *string, rooms *[]roomToken) error {
	// Older consoles may still submit the removed experimental opt-in.
	c.ForwardResourcePacks = true
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
	tokens := m.tokens
	if rooms != nil {
		var err error
		if tokens, err = m.resolveRooms(*rooms); err != nil {
			return err
		}
	} else if token != nil {
		if strings.TrimSpace(*token) == "" {
			return fmt.Errorf("新令牌为空；保持原令牌请留空输入框")
		}
		tokens = append([]string{strings.TrimSpace(*token)}, tailTokens(m.tokens)...)
	}
	if err := config.Save(filepath.Join(m.dir, "config.json"), c); err != nil {
		return err
	}
	if !sameTokens(tokens, m.tokens) {
		if err := saveTokens(filepath.Join(m.dir, "secrets.dat"), tokens); err != nil {
			return err
		}
		m.tokens = tokens
	}
	m.cfg = c
	return nil
}

func tailTokens(tokens []string) []string {
	if len(tokens) < 2 {
		return nil
	}
	return tokens[1:]
}

func (m *Manager) resolveRooms(rooms []roomToken) ([]string, error) {
	if len(rooms) > maxRooms {
		return nil, fmt.Errorf("最多同时开 %d 个房间", maxRooms)
	}
	tokens := make([]string, 0, len(rooms))
	seen := map[string]bool{}
	for _, r := range rooms {
		var t string
		switch {
		case r.Keep != nil:
			if *r.Keep < 0 || *r.Keep >= len(m.tokens) {
				return nil, fmt.Errorf("要保留的房间令牌不存在，请刷新页面后重试")
			}
			t = m.tokens[*r.Keep]
		case strings.TrimSpace(r.Token) != "":
			t = strings.TrimSpace(r.Token)
			if len(strings.Fields(t)) != 1 {
				return nil, fmt.Errorf("令牌格式无效：一个房间只能填写一个令牌")
			}
		default:
			return nil, fmt.Errorf("新房间的令牌为空")
		}
		if seen[t] {
			// One account cannot keep two rooms alive; the second login would drop the first.
			return nil, fmt.Errorf("同一个令牌不能用于多个房间，请为每个房间指定不同网易账号的令牌")
		}
		seen[t] = true
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func sameTokens(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func saveTokens(path string, tokens []string) error {
	if len(tokens) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return secret.Save(path, strings.Join(tokens, "\n"))
}

func (m *Manager) start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return nil
	}
	if len(m.tokens) == 0 {
		return fmt.Errorf("请先保存机器人令牌")
	}
	profile, err := m.resourceProfile()
	if err != nil {
		return err
	}
	// Lock every token up front so a busy one cannot leave the rest half-started.
	leases := make([]*session.Session, 0, len(m.tokens))
	for i, token := range m.tokens {
		lease, err := session.Acquire(token)
		if err != nil {
			for _, l := range leases {
				l.Close()
			}
			if len(m.tokens) > 1 {
				return fmt.Errorf("%s%w", roomLabel(i, len(m.tokens)), err)
			}
			return err
		}
		leases = append(leases, lease)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.finished = make(chan struct{})
	m.status.Running = true
	m.status.LastError = ""
	m.status.Attempts = 0
	m.status.Rooms = make([]RoomStatus, len(m.tokens))
	for i := range m.status.Rooms {
		m.status.Rooms[i].Phase = "checking_upstream"
	}
	cfg, tokens, done := m.cfg, append([]string{}, m.tokens...), m.finished
	// Every room relays into the same world, so broadcasts are logged once overall.
	logs := newGameLogs(m.event)
	var rooms sync.WaitGroup
	for i, token := range tokens {
		rooms.Add(1)
		go func(i int, token string, lease *session.Session) {
			defer rooms.Done()
			defer lease.Close()
			m.run(ctx, i, len(tokens), cfg, token, logs, profile)
		}(i, token, leases[i])
	}
	go func() {
		rooms.Wait()
		m.update(func(s *Status) {
			s.Running = false
			s.Phase = "stopped"
			s.RoomID = 0
			s.Players = 0
			s.RetryAt = ""
			s.Rooms = nil
		})
		m.mu.Lock()
		m.cancel = nil
		close(done)
		m.mu.Unlock()
		m.event("房间已停止")
	}()
	return nil
}

func (m *Manager) resourceProfile() (*netease.ResourceProfile, error) {
	if m.cfg.NetEaseResourcePacks {
		// Explicit custom profiles keep existing deployments compatible. A
		// missing file falls back to Classic; corrupt/unreadable files report
		// an error instead of silently changing the requested component.
		path := filepath.Join(m.dir, netease.ResourceProfileFile)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return netease.LoadResourceProfile(path)
		}
	}
	if m.cfg.ForwardResourcePacks || m.cfg.NetEaseResourcePacks {
		return netease.BuiltinResourceProfile()
	}
	return nil, nil
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

// roomLog routes one room's messages through the shared log with its label.
type roomLog struct {
	m     *Manager
	label string
}

func (l roomLog) event(message string) { l.m.event(l.label + message) }
func (l roomLog) debug(message string) { l.m.debug(l.label + message) }

func (m *Manager) run(ctx context.Context, index, count int, cfg config.Config, token string, logs *gameLogs, profile *netease.ResourceProfile) {
	rl := roomLog{m: m, label: roomLabel(index, count)}
	rl.event("启动本地联机房主机器人")
	failures := 0
	for ctx.Err() == nil {
		m.updateRoom(index, func(s *RoomStatus) {
			s.Phase = "checking_upstream"
			s.RoomID = 0
			s.Players = 0
			s.RetryAt = ""
			s.Attempts++
		})
		version, err := bridge.PingUpstream(ctx, cfg.Upstream)
		if err == nil {
			m.update(func(s *Status) { s.UpstreamOnline = true; s.UpstreamVersion = version })
			m.updateRoom(index, func(s *RoomStatus) { s.Phase = "authenticating" })
			rl.debug("本地 Bedrock 服务器在线，版本 " + version + "；开始鉴权")
			var info auth.CreateInfo
			authCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
			info, err = auth.FetchCreate(authCtx, cfg, token)
			cancel()
			if err == nil {
				m.updateRoom(index, func(s *RoomStatus) {
					s.Account = info.UserPlayerName
					s.ProtocolID = info.ProtocolID
					s.Phase = "creating"
				})
				rl.debug(fmt.Sprintf("鉴权通过，房间协议 %d；连接开房节点", info.ProtocolID))
				var opened *room.Room
				createCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
				var itemIDs []uint64
				if profile != nil {
					itemIDs = profile.ItemIDs
				}
				opened, err = room.OpenWithResourcePacks(createCtx, cfg, info, rl.debug, itemIDs)
				cancel()
				if err == nil {
					failures = 0
					m.updateRoom(index, func(s *RoomStatus) {
						s.Phase = "online"
						s.RoomID = opened.ID
						s.ConnectedAt = time.Now().Format(time.RFC3339)
						s.LastError = ""
					})
					rl.event(fmt.Sprintf("房间 %d 已上线，房主机器人持续保活", opened.ID))
					err = m.serveRoom(ctx, index, cfg, opened, info, logs, rl, profile)
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
		m.updateRoom(index, func(s *RoomStatus) {
			s.Phase = "retrying"
			s.LastError = message
			s.RoomID = 0
			s.Players = 0
			s.RetryAt = time.Now().Add(delay).Format(time.RFC3339)
		})
		rl.event(fmt.Sprintf("%s；%.0f 秒后重试", message, delay.Seconds()))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *Manager) serveRoom(ctx context.Context, index int, cfg config.Config, r *room.Room, info auth.CreateInfo, gameLogs *gameLogs, rl roomLog, profile *netease.ResourceProfile) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	friendDone := make(chan struct{})
	go func() {
		defer close(friendDone)
		if cfg.AutoAcceptFriends {
			friends.Run(sessionCtx, info.UserUniqueID, info.EncryptKeyBytes, rl.event, rl.debug)
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
			// NetEase counts the host bot, so each room admits MaxPlayers-1 guests.
			full := true
			m.updateRoom(index, func(s *RoomStatus) {
				full = s.Players >= cfg.MaxPlayers-1
				if !full {
					s.Players++
				}
			})
			if full {
				conn.Close()
				continue
			}
			bridges.Add(1)
			go func(client *nethernet.Conn) {
				defer bridges.Done()
				defer client.Close()
				defer m.updateRoom(index, func(s *RoomStatus) {
					if s.Players > 0 {
						s.Players--
					}
				})
				rl.debug("玩家连接已建立，正在转发到本地服务器")
				dialCtx, stop := context.WithTimeout(sessionCtx, 8*time.Second)
				server, err := bdsraknet.DialContext(dialCtx, cfg.Upstream)
				stop()
				if err != nil {
					rl.event("玩家转发失败：本地服务器连接失败")
					return
				}
				defer server.Close()
				rl.debug("本地服务器 RakNet 连接成功")
				backend, err := bridge.NewWithOptions(server, rl.debug, bridge.Options{ForwardResourcePacks: cfg.ForwardResourcePacks, ResourceProfile: profile})
				if err != nil {
					rl.event("无法生成后端连接密钥")
					return
				}
				defer backend.ForgetSkin()
				gameSource := gameLogs.attach()
				defer gameLogs.detach(gameSource)
				backend.OnGameEvent = func(e bridge.GameEvent) { gameLogs.observe(gameSource, e) }
				clientStream := &relay.Observed{Stream: client, Label: "玩家 → 本地服务器", Trace: rl.debug}
				serverStream := &relay.Observed{Stream: backend, Label: "本地服务器 → 玩家", Trace: rl.debug}
				err = relay.Relay(sessionCtx, clientStream, serverStream)
				rl.debug(fmt.Sprintf("玩家转发连接已断开：%v；上行 %d 包，下行 %d 包", err, clientStream.Count, serverStream.Count))
				if err != nil && sessionCtx.Err() == nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
					rl.event(fmt.Sprintf("玩家连接异常：%v", err))
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
				}
			})
			m.updateRoom(index, func(s *RoomStatus) {
				if err == nil {
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
