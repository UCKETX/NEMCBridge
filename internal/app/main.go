package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"nemcbridge/internal/auth"
	"nemcbridge/internal/bridge"
	"nemcbridge/internal/config"
	"nemcbridge/internal/netease"
	"nemcbridge/internal/packimport"
	"nemcbridge/internal/secret"
	"nemcbridge/internal/session"
	"nemcbridge/internal/webui"
)

// Supplied by the command entry point. Local builds retain the dev defaults.
var version = "dev"
var commit = "unknown"

// Run starts the console and room manager with the executable's build metadata.
func Run(buildVersion, buildCommit string) error {
	version, commit = buildVersion, buildCommit
	return runMain()
}
func runMain() error {
	showVersion := flag.Bool("version", false, "显示版本和源代码提交")
	base := flag.String("data-dir", "", "配置和日志目录（默认程序所在目录）")
	headless := flag.Bool("headless", false, "不自动打开浏览器")
	webPort := flag.Int("web-port", 0, "临时覆盖控制台端口")
	run := flag.Bool("run", false, "启动后自动开房")
	debug := flag.Bool("debug", false, "开启详细诊断日志（也可在控制台即时切换）")
	check := flag.Bool("check", false, "检查本地服务器和真实鉴权，然后退出")
	importToken := flag.Bool("import-token", false, "从标准输入读取令牌并保存")
	resourceRoom := flag.String("import-resource-room", "", "从网易原生房间导入组件资源配置后退出（先停止机器人房间）")
	resourcePassword := flag.String("resource-room-password", "", "待导入的原生房间密码")
	resetAdmin := flag.Bool("reset-admin-password", false, "重置管理密码并在终端显示新密码")
	duration := flag.Int("run-seconds", 0, "运行指定秒数后关房退出（测试用）")
	flag.Parse()
	if *showVersion {
		fmt.Printf("NEMCBridge %s (%s/%s, commit %s)\n", version, runtime.GOOS, runtime.GOARCH, commit)
		return nil
	}
	if *base == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		*base = filepath.Dir(exe)
	}
	dir, err := filepath.Abs(*base)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	cfg, err := config.Load(filepath.Join(dir, "config.json"))
	if err != nil {
		return fmt.Errorf("配置读取失败: %w", err)
	}
	if *webPort != 0 {
		cfg.WebPort = *webPort
		if err = cfg.Validate(); err != nil {
			return err
		}
	}
	if *debug {
		cfg.Debug = true
	}
	if *importToken {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 8192))
		if err != nil {
			return err
		}
		tokens := parseTokens(string(b))
		if len(tokens) == 0 {
			return fmt.Errorf("令牌为空")
		}
		if len(tokens) > maxRooms {
			return fmt.Errorf("最多同时开 %d 个房间", maxRooms)
		}
		if err = saveTokens(filepath.Join(dir, "secrets.dat"), tokens); err != nil {
			return err
		}
		fmt.Printf("已保存 %d 个房间的令牌：%s\n", len(tokens), secret.StorageDescription())
		return nil
	}
	// One token per room; whitespace separates several.
	tokens := parseTokens(os.Getenv("NEMCBRIDGE_TOKEN"))
	if len(tokens) == 0 {
		tokens = parseTokens(os.Getenv("LOBBYKEEPER_TOKEN")) // Compatibility with existing deployments.
	}
	if len(tokens) == 0 {
		stored, err := secret.Load(filepath.Join(dir, "secrets.dat"))
		if err != nil {
			return fmt.Errorf("无法读取令牌，请检查当前用户权限或重新设置: %w", err)
		}
		tokens = parseTokens(stored)
	}
	if len(tokens) > maxRooms {
		return fmt.Errorf("最多同时开 %d 个房间，当前提供了 %d 个令牌", maxRooms, len(tokens))
	}
	if *resourceRoom != "" {
		if len(tokens) == 0 {
			return fmt.Errorf("请先保存机器人令牌")
		}
		lease, err := session.Acquire(tokens[0])
		if err != nil {
			return err
		}
		defer lease.Close()
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		ctx, timeout := context.WithTimeout(ctx, 90*time.Second)
		defer timeout()
		if err = session.RejectRunningRoom(ctx, cfg.WebPort); err != nil {
			return err
		}
		fmt.Println("正在读取原生房间资源配置；同一机器人账号应只在本处登录")
		profile, err := packimport.Capture(ctx, cfg, tokens[0], *resourceRoom, *resourcePassword)
		if err != nil {
			return err
		}
		if err = netease.SaveResourceProfile(filepath.Join(dir, netease.ResourceProfileFile), profile); err != nil {
			return err
		}
		cfg.NetEaseResourcePacks = true
		if err = config.Save(filepath.Join(dir, "config.json"), cfg); err != nil {
			return err
		}
		fmt.Printf("已导入 %d 个网易组件并启用组件资源。请重新启动程序；资源配置含包密钥，请勿公开。\n", len(profile.ItemIDs))
		return nil
	}
	logger, logFile, err := openLogger(dir)
	if err != nil {
		return err
	}
	defer logFile.Close()
	if *check {
		if len(tokens) == 0 {
			return fmt.Errorf("请先设置机器人令牌")
		}
		for _, token := range tokens {
			lease, err := session.Acquire(token)
			if err != nil {
				return err
			}
			defer lease.Close()
		}
		if err := session.RejectRunningRoom(context.Background(), cfg.WebPort); err != nil {
			return err
		}
		version, err := bridge.PingUpstream(context.Background(), cfg.Upstream)
		if err != nil {
			return fmt.Errorf("本地 Bedrock 服务器不可达")
		}
		fmt.Println("本地服务器在线，版本:", version)
		accounts := map[string]int{}
		for i, token := range tokens {
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			info, err := auth.FetchCreate(ctx, cfg, token)
			cancel()
			if err != nil {
				return fmt.Errorf("%s%w", roomLabel(i, len(tokens)), err)
			}
			fmt.Printf("%sFGAuth 鉴权通过；账号 %s；房间协议 %d；转发凭据校验通过\n", roomLabel(i, len(tokens)), info.UserPlayerName, info.ProtocolID)
			// Two rooms on one account would knock each other offline.
			if prev, dup := accounts[info.UserPlayerName]; dup {
				return fmt.Errorf("房间 %d 与房间 %d 使用了同一个网易账号，请在 FGAuth 为每个房间创建指定不同账号的令牌", prev+1, i+1)
			}
			accounts[info.UserPlayerName] = i
		}
		return nil
	}
	manager := newManager(dir, cfg, tokens, logger)
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", cfg.WebPort))
	if err != nil {
		return fmt.Errorf("控制台端口被占用: %w", err)
	}
	defer listener.Close()
	admin, initialPassword, err := loadConsoleAuth(dir, os.Getenv("NEMCBRIDGE_ADMIN_PASSWORD"), *resetAdmin)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: makeHandler(manager, admin), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 45 * time.Second}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(listener) }()
	address := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebPort)
	printStartupBanner(os.Stdout, terminalColorEnabled())
	printConsoleURLs(os.Stdout, cfg.WebPort, consoleIPv4Addresses())
	fmt.Println("管理员账号：admin")
	if initialPassword != "" {
		fmt.Println("管理员初始密码：", initialPassword)
		fmt.Println("请保存此密码；后续启动不再显示，可用 -reset-admin-password 重置。")
	} else if os.Getenv("NEMCBRIDGE_ADMIN_PASSWORD") != "" {
		fmt.Println("管理密码：由 NEMCBRIDGE_ADMIN_PASSWORD 设置。")
	} else {
		fmt.Println("管理密码：已设置，忘记密码可用 -reset-admin-password 重置。")
	}
	if !*headless {
		openBrowser(address)
	}
	if *run || cfg.AutoStart {
		if err = manager.start(); err != nil {
			manager.event(err.Error())
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var timed <-chan time.Time
	var timer *time.Timer
	if *duration > 0 {
		timer = time.NewTimer(time.Duration(*duration) * time.Second)
		timed = timer.C
		defer timer.Stop()
	}
	select {
	case <-ctx.Done():
	case <-timed:
	case <-manager.exit:
	case err = <-serverErr:
		if err != http.ErrServerClosed {
			return err
		}
	}
	manager.stop()
	manager.wait()
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}
func openBrowser(address string) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address).Start()
	} else if runtime.GOOS == "darwin" {
		_ = exec.Command("open", address).Start()
	} else {
		_ = exec.Command("xdg-open", address).Start()
	}
}
func makeHandler(m *Manager, admin *consoleAuth) http.Handler {
	if admin == nil {
		panic("console authentication is required")
	}
	mux := http.NewServeMux()
	respond := func(w http.ResponseWriter, data any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(data)
	}
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		page, err := webui.Files.ReadFile("login.html")
		if err != nil {
			http.Error(w, "登录页面不可用", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("POST /api/login", admin.login)
	mux.HandleFunc("POST /api/logout", admin.logout)
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) { respond(w, m.snapshot()) })
	mux.HandleFunc("POST /api/start", func(w http.ResponseWriter, r *http.Request) {
		if err := m.start(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, r *http.Request) { m.stop(); respond(w, map[string]bool{"ok": true}) })
	mux.HandleFunc("POST /api/debug", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.Enabled == nil {
			http.Error(w, "请指定 enabled 为 true 或 false", http.StatusBadRequest)
			return
		}
		if err := m.setDebug(*body.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respond(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/exit", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"ok": true})
		m.exitOnce.Do(func() { close(m.exit) })
	})
	mux.HandleFunc("POST /api/config", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Config config.Config `json:"config"`
			Token  *string       `json:"token"`
			Rooms  *[]roomToken  `json:"rooms"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			http.Error(w, "配置 JSON 无效", 400)
			return
		}
		if err := m.configure(body.Config, body.Token, body.Rooms); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, map[string]bool{"ok": true})
	})
	mux.Handle("GET /", http.FileServer(http.FS(webui.Files)))
	allowedHosts := consoleHosts(m.snapshot().Config.WebPort, consoleIPv4Addresses())
	protected := admin.protect(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHosts[r.Host] {
			http.Error(w, "控制台访问地址无效，请使用本机或终端显示的 IP 地址", 403)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("Origin") != "http://"+r.Host || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "请求来源校验失败", 403)
				return
			}
		}
		protected.ServeHTTP(w, r)
	})
}
