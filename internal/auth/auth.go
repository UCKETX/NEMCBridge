package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker"

	"nemcbridge/internal/config"
	"nemcbridge/internal/redact"
)

type CreateInfo struct {
	bunker.TanLobbyCreateResponse
	ProtocolID uint8 `json:"lobby_protocol_id"`
}

func newAuthHTTP(base string) *http.Client {
	root, _ := url.Parse(base)
	return &http.Client{Timeout: 75 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return fmt.Errorf("验证站重定向过多")
		}
		if !allowedAuthRedirect(root, req.URL) {
			return fmt.Errorf("验证站重定向地址无效或使用了不安全的协议，未发送令牌")
		}
		return nil
	}}
}

func allowedAuthRedirect(root, target *url.URL) bool {
	if root == nil || target == nil || root.User != nil || target.User != nil || target.Hostname() == "" {
		return false
	}
	// The user selects the authentication service; that service may route to
	// other HTTPS domains or ports. No provider-specific allowlist is required.
	if target.Scheme == "https" {
		return true
	}
	loopback := func(host string) bool {
		ip := net.ParseIP(host)
		return strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
	}
	// Preserve local HTTP development without allowing a remote plaintext hop.
	return root.Scheme == "http" && target.Scheme == "http" && loopback(root.Hostname()) && loopback(target.Hostname())
}

func FetchCreate(ctx context.Context, cfg config.Config, token string) (CreateInfo, error) {
	var info CreateInfo
	if token == "" {
		return info, fmt.Errorf("请先设置机器人令牌")
	}
	body, _ := json.Marshal(map[string]string{"login_token": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.AuthServer, "/")+"/api/phoenix/tan_lobby_create", bytes.NewReader(body))
	if err != nil {
		return info, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NEMCBridge/1.0")
	resp, err := newAuthHTTP(cfg.AuthServer).Do(req)
	if err != nil {
		return info, fmt.Errorf("验证站连接失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return info, fmt.Errorf("验证站返回 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return info, err
	}
	if err = json.Unmarshal(b, &info); err != nil {
		return info, fmt.Errorf("验证站未返回有效 JSON")
	}
	if !info.Success {
		return info, fmt.Errorf("鉴权失败: %s", redact.Sanitize(info.ErrorInfo, token))
	}
	if info.ProtocolID == 0 {
		return info, fmt.Errorf("验证站未提供 lobby_protocol_id，请更新 FGAuth")
	}
	if info.UserUniqueID == 0 || info.UserPlayerName == "" {
		return info, fmt.Errorf("验证站返回的用户信息不完整")
	}
	for _, addr := range []string{info.RaknetServerAddress, info.SignalingServerAddress} {
		if _, err := ParseEndpoint(addr); err != nil {
			return info, fmt.Errorf("验证站返回的转发地址无效")
		}
	}
	if len(info.RaknetRand) != 16 || len(info.RaknetAESRand) < 16 || len(info.SignalingSeed) != 16 || len(info.SignalingTicket) < 16 || len(info.EncryptKeyBytes) != 32 || len(info.DecryptKeyBytes) != 32 {
		return info, fmt.Errorf("验证站返回的加密材料长度无效")
	}
	// Old auth implementations return PKCS#7 padding, but the handshake uses one AES block.
	info.RaknetAESRand = info.RaknetAESRand[:16]
	info.SignalingTicket = info.SignalingTicket[:16]
	return info, nil
}

// ParseEndpoint resolves a relay or signaling UDP address.
func ParseEndpoint(addr string) (*net.UDPAddr, error) { return net.ResolveUDPAddr("udp", addr) }
