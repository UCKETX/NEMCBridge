// Package packimport collects resource metadata from a user-selected native
// room, then leaves before world initialization. It never copies pack contents.
package packimport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"

	"github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	nepacket "github.com/Happy2018new/nemc-tan-lobby-solver/minecraft/protocol/packet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/service"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"nemcbridge/internal/auth"
	"nemcbridge/internal/config"
	"nemcbridge/internal/gamebatch"
	"nemcbridge/internal/netease"
	"nemcbridge/internal/relay"
)

func Capture(ctx context.Context, cfg config.Config, token, room, password string) (profile *netease.ResourceProfile, err error) {
	defer func() {
		if recover() != nil {
			profile = nil
			err = errors.New("原生房间资源协议格式无效")
		}
	}()
	roomID, parseErr := strconv.ParseUint(room, 10, 32)
	if parseErr != nil || roomID == 0 {
		return nil, errors.New("原生房间号无效")
	}
	raw, info, err := service.DialContext(ctx, room, password, auth.NewRoomAccess(ctx, cfg, token))
	if err != nil {
		return nil, errors.New("无法加入原生房间，请检查房间号、密码、好友权限和机器人鉴权")
	}
	defer raw.Close()
	conn, ok := raw.(*nethernet.Conn)
	if !ok {
		return nil, errors.New("原生房间连接类型无效")
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	return captureStream(conn, info)
}

func captureStream(conn relay.Stream, info bunker.TanLobbyLoginResponse) (profile *netease.ResourceProfile, err error) {
	defer func() {
		if recover() != nil {
			profile = nil
			err = errors.New("原生房间资源协议格式无效")
		}
	}()
	profile = &netease.ResourceProfile{ProtocolID: 860}
	for _, value := range info.RoomModDisplayName {
		id, e := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if e != nil || id == 0 {
			return nil, errors.New("验证站未提供数字网易组件编号，无法导入此房间")
		}
		profile.ItemIDs = append(profile.ItemIDs, id)
	}
	if len(profile.ItemIDs) == 0 {
		return nil, errors.New("原生房间没有声明网易组件，请先在原生世界启用材质")
	}
	if _, err = conn.Write([]byte{6, 0xc1, 1, 0, 0, 3, 0x5c}); err != nil {
		return nil, err
	}
	first, err := conn.ReadPacket()
	if err != nil {
		return nil, err
	}
	packets, err := gamebatch.Split(first)
	if err != nil || len(packets) != 1 {
		return nil, errors.New("原生房间网络设置无效")
	}
	id, n := binary.Uvarint(packets[0])
	if n <= 0 || id&1023 != uint64(packet.IDNetworkSettings) {
		return nil, errors.New("原生房间未返回网络设置")
	}
	var settings packet.NetworkSettings
	settings.Marshal(protocol.NewReader(bytes.NewBuffer(packets[0][n:]), 0, true))
	if settings.CompressionAlgorithm != 2 {
		return nil, errors.New("资源导入目前仅支持网易游戏协议 860 的字典压缩")
	}
	compress, decompress := nepacket.NeteaseCompression(), nepacket.NeteaseCompression()
	send := func(pk []byte) error {
		wire, e := compress.Compress(gamebatch.Join([][]byte{pk}))
		if e != nil {
			return e
		}
		_, e = conn.Write(wire)
		return e
	}
	login, err := makeLogin(info)
	if err != nil {
		return nil, err
	}
	if err = send(login); err != nil {
		return nil, err
	}
	for count := 0; count < 64; count++ {
		wire, e := conn.ReadPacket()
		if e != nil {
			return nil, errors.New("原生房间在资源导入完成前断开连接")
		}
		plain, e := decompress.Decompress(wire, 2<<20)
		if e != nil {
			return nil, errors.New("原生房间资源批次解压失败")
		}
		packets, e = gamebatch.Split(plain)
		if e != nil {
			return nil, e
		}
		for _, pk := range packets {
			id, n = binary.Uvarint(pk)
			if n <= 0 {
				return nil, errors.New("原生资源包头无效")
			}
			switch id & 1023 {
			case uint64(packet.IDServerToClientHandshake):
				return nil, errors.New("该原生房间要求额外游戏加密，目前无法导入")
			case uint64(packet.IDDisconnect):
				return nil, errors.New("原生房主拒绝了机器人连接")
			case uint64(packet.IDPlayStatus):
				if len(pk) >= n+4 && binary.BigEndian.Uint32(pk[n:]) == 0 {
					if err = send([]byte{0x81, 1, 0}); err != nil {
						return nil, err
					}
				}
			case uint64(packet.IDResourcePacksInfo):
				if len(pk) > 256<<10 {
					return nil, errors.New("原生资源信息过大")
				}
				profile.Info = append([]byte(nil), pk...)
				if err = send([]byte{8, 3, 0, 0}); err != nil {
					return nil, err
				}
			case uint64(packet.IDResourcePackStack):
				if len(pk) > 256<<10 {
					return nil, errors.New("原生资源顺序过大")
				}
				profile.Stack = append([]byte(nil), pk...)
				if err = profile.Validate(); err != nil {
					return nil, err
				}
				return profile, nil
			}
		}
	}
	return nil, errors.New("原生房间未提供完整资源配置")
}
