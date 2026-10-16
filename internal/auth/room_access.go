package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	"nemcbridge/internal/config"
	"nemcbridge/internal/redact"
)

type roomAccess struct {
	bunker.Authenticator
	ctx   context.Context
	cfg   config.Config
	token string
}

// Use the same redirect policy and bounded HTTP requests as room creation.
func NewRoomAccess(ctx context.Context, cfg config.Config, token string) bunker.Authenticator {
	return roomAccess{Authenticator: bunker.NewAccessWrapper(cfg.AuthServer, token, "", ""), ctx: ctx, cfg: cfg, token: token}
}

func (a roomAccess) GetAccess(roomID string) (bunker.TanLobbyLoginResponse, error) {
	var info bunker.TanLobbyLoginResponse
	body, _ := json.Marshal(map[string]string{"login_token": a.token, "room_id": roomID})
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, strings.TrimRight(a.cfg.AuthServer, "/")+"/api/phoenix/tan_lobby_login", bytes.NewReader(body))
	if err != nil {
		return info, fmt.Errorf("原生房间鉴权请求无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NEMCBridge/1.0")
	resp, err := newAuthHTTP(a.cfg.AuthServer).Do(req)
	if err != nil {
		return info, fmt.Errorf("原生房间鉴权连接失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("原生房间鉴权返回 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 || json.Unmarshal(b, &info) != nil {
		return info, fmt.Errorf("原生房间鉴权响应无效")
	}
	if !info.Success {
		return info, fmt.Errorf("原生房间鉴权失败：%s", redact.Sanitize(info.ErrorInfo, a.token))
	}
	if info.UserUniqueID == 0 || info.RoomOwnerID == 0 || len(info.EncryptKeyBytes) != 32 || len(info.DecryptKeyBytes) != 32 || len(info.RaknetRand) != 16 || len(info.RaknetAESRand) < 16 || len(info.SignalingSeed) != 16 || len(info.SignalingTicket) < 16 {
		return info, fmt.Errorf("原生房间鉴权信息不完整")
	}
	for _, addr := range []string{info.RaknetServerAddress, info.SignalingServerAddress} {
		if _, err = ParseEndpoint(addr); err != nil {
			return info, fmt.Errorf("原生房间转发地址无效")
		}
	}
	info.RaknetAESRand = info.RaknetAESRand[:16]
	info.SignalingTicket = info.SignalingTicket[:16]
	return info, nil
}
