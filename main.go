package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
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
)

//go:embed web/*
var webFiles embed.FS

// Set by the release workflow. Local builds retain the dev defaults.
var version = "dev"
var commit = "unknown"

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func runMain() error {
	showVersion := flag.Bool("version", false, "显示版本和源代码提交")
	base := flag.String("data-dir", "", "配置和日志目录（默认程序所在目录）")
	headless := flag.Bool("headless", false, "不自动打开浏览器")
	webPort := flag.Int("web-port", 0, "临时覆盖控制台端口")
	run := flag.Bool("run", false, "启动后自动开房")
	check := flag.Bool("check", false, "检查本地服务器和真实鉴权，然后退出")
	importToken := flag.Bool("import-token", false, "从标准输入读取令牌并保存")
	duration := flag.Int("run-seconds", 0, "运行指定秒数后关房退出（测试用）")
	flag.Parse()
	if *showVersion {
		fmt.Printf("LobbyKeeper %s (%s/%s, commit %s)\n", version, runtime.GOOS, runtime.GOARCH, commit)
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
	cfg, err := loadConfig(filepath.Join(dir, "config.json"))
	if err != nil {
		return fmt.Errorf("配置读取失败: %w", err)
	}
	if *webPort != 0 {
		cfg.WebPort = *webPort
		if err = cfg.validate(); err != nil {
			return err
		}
	}
	if *importToken {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 8192))
		if err != nil {
			return err
		}
		token := strings.TrimSpace(string(b))
		if token == "" {
			return fmt.Errorf("令牌为空")
		}
		if err = saveSecret(filepath.Join(dir, "secrets.dat"), token); err != nil {
			return err
		}
		fmt.Println("令牌已保存：" + secretStorageDescription())
		return nil
	}
	token := os.Getenv("LOBBYKEEPER_TOKEN")
	if token == "" {
		token, err = loadSecret(filepath.Join(dir, "secrets.dat"))
		if err != nil {
			return fmt.Errorf("无法读取令牌，请检查当前用户权限或重新设置: %w", err)
		}
	}
	logger, logFile, err := openLogger(dir)
	if err != nil {
		return err
	}
	defer logFile.Close()
	if *check {
		session, err := acquireAccountSession(token)
		if err != nil {
			return err
		}
		defer session.Close()
		if err := rejectRunningRoom(context.Background(), cfg.WebPort); err != nil {
			return err
		}
		version, err := pingUpstream(context.Background(), cfg.Upstream)
		if err != nil {
			return fmt.Errorf("本地 Bedrock 服务器不可达")
		}
		fmt.Println("本地服务器在线，版本:", version)
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		info, err := fetchCreate(ctx, cfg, token)
		if err != nil {
			return err
		}
		fmt.Printf("FGAuth 鉴权通过；账号 %s；房间协议 %d；转发凭据校验通过\n", info.UserPlayerName, info.ProtocolID)
		return nil
	}
	manager := newManager(dir, cfg, token, logger)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.WebPort))
	if err != nil {
		return fmt.Errorf("控制台端口被占用: %w", err)
	}
	server := &http.Server{Handler: makeHandler(manager), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 45 * time.Second}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(listener) }()
	address := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebPort)
	fmt.Println("本地联机挂服机控制台:", address)
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
func makeHandler(m *Manager) http.Handler {
	mux := http.NewServeMux()
	respond := func(w http.ResponseWriter, data any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(data)
	}
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) { respond(w, m.snapshot()) })
	mux.HandleFunc("POST /api/start", func(w http.ResponseWriter, r *http.Request) {
		if err := m.start(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, r *http.Request) { m.stop(); respond(w, map[string]bool{"ok": true}) })
	mux.HandleFunc("POST /api/exit", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"ok": true})
		m.exitOnce.Do(func() { close(m.exit) })
	})
	mux.HandleFunc("POST /api/config", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Config Config  `json:"config"`
			Token  *string `json:"token"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			http.Error(w, "配置 JSON 无效", 400)
			return
		}
		if err := m.configure(body.Config, body.Token); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, map[string]bool{"ok": true})
	})
	assets, _ := fs.Sub(webFiles, "web")
	mux.Handle("GET /", http.FileServer(http.FS(assets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := m.snapshot()
		host := fmt.Sprintf("127.0.0.1:%d", s.Config.WebPort)
		localHost := fmt.Sprintf("localhost:%d", s.Config.WebPort)
		if r.Host != host && r.Host != localHost {
			http.Error(w, "只允许本机访问", 403)
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
		mux.ServeHTTP(w, r)
	})
}
