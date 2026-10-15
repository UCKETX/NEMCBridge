package bridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/raknet"
)

func PingUpstream(ctx context.Context, addr string) (string, error) {
	child, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pong, err := raknet.PingContext(child, addr)
	if err != nil {
		return "", err
	}
	fields := strings.Split(string(pong), ";")
	if len(fields) < 4 || fields[0] != "MCPE" {
		return "", fmt.Errorf("目标不是 Bedrock 服务器")
	}
	return fields[3], nil
}
