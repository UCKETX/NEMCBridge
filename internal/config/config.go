package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"nemcbridge/internal/fileutil"
)

type Config struct {
	AuthServer        string `json:"auth_server"`
	Upstream          string `json:"upstream"`
	RoomName          string `json:"room_name"`
	RoomGameVersion   string `json:"room_game_version"`
	RoomPassword      string `json:"room_password"`
	MaxPlayers        int    `json:"max_players"`
	FriendsOnly       bool   `json:"friends_only"`
	AutoAcceptFriends bool   `json:"auto_accept_friends"`
	AllowPvP          bool   `json:"allow_pvp"`
	AutoStart         bool   `json:"auto_start"`
	WebPort           int    `json:"web_port"`
	RetrySeconds      int    `json:"retry_seconds"`
	Debug             bool   `json:"debug"`
}

func Default() Config {
	return Config{AuthServer: "https://fgauth.ucket.top", Upstream: "127.0.0.1:19132", RoomName: "来和我一起玩吧！", RoomGameVersion: "1.21.120.0", MaxPlayers: 10, AllowPvP: true, AutoAcceptFriends: true, WebPort: 18321, RetrySeconds: 10}
}

func (c Config) Validate() error {
	u, err := url.Parse(c.AuthServer)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("验证服务器地址无效")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopback(u.Hostname())) {
		return fmt.Errorf("验证服务器必须使用 HTTPS（本机测试可使用 HTTP）")
	}
	host, port, err := net.SplitHostPort(c.Upstream)
	if err != nil || strings.TrimSpace(host) == "" {
		return fmt.Errorf("服务器地址格式应为 IP:端口")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("服务器端口无效")
	}
	if c.MaxPlayers < 2 || c.MaxPlayers > 40 {
		return fmt.Errorf("房间人数必须为 2–40")
	}
	if len([]rune(c.RoomName)) < 1 || len([]rune(c.RoomName)) > 40 {
		return fmt.Errorf("房间名称必须为 1–40 字")
	}
	if len(c.RoomPassword) > 64 {
		return fmt.Errorf("房间密码过长")
	}
	parts := strings.Split(c.RoomGameVersion, ".")
	if len(parts) != 4 {
		return fmt.Errorf("网易房间内核版本应为四段数字，例如 1.21.120.0")
	}
	for _, part := range parts {
		if _, err := strconv.ParseUint(part, 10, 16); err != nil {
			return fmt.Errorf("网易房间内核版本格式无效")
		}
	}
	if c.WebPort < 1024 || c.WebPort > 65535 {
		return fmt.Errorf("控制台端口必须为 1024–65535")
	}
	if c.RetrySeconds < 5 || c.RetrySeconds > 300 {
		return fmt.Errorf("重连间隔必须为 5–300 秒")
	}
	return nil
}

func isLoopback(h string) bool {
	ip := net.ParseIP(h)
	return h == "localhost" || (ip != nil && ip.IsLoopback())
}

func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, Save(path, c)
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWrite(path, append(b, '\n'), 0600)
}
