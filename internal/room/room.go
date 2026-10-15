package room

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/raknet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/core/webrtc"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/encoding"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/packet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/service/signaling"

	"nemcbridge/internal/auth"
	"nemcbridge/internal/config"
)

type Room struct {
	ID        uint32
	control   *raknet.Conn
	encoder   *packet.Encoder
	decoder   *packet.Decoder
	listener  *nethernet.Listener
	signal    *signaling.Conn
	networkID uint64
	done      chan struct{}
	errMu     sync.Mutex
	err       error
	writeMu   sync.Mutex
	once      sync.Once
	trace     func(string)
}

func marshalControl(p packet.Packet) ([]byte, error) {
	b := bytes.NewBuffer(nil)
	h := packet.Header{PacketID: p.ID()}
	if err := h.Write(b); err != nil {
		return nil, err
	}
	p.Marshal(encoding.NewWriter(b, 0))
	return b.Bytes(), nil
}

func (r *Room) write(p packet.Packet) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	b, err := marshalControl(p)
	if err != nil {
		return err
	}
	return r.encoder.Encode(b)
}

func (r *Room) read() (p packet.Packet, err error) {
	defer func() {
		if recover() != nil {
			p = nil
			err = fmt.Errorf("房间协议数据格式错误")
		}
	}()
	data, err := r.decoder.Decode()
	if err != nil {
		return nil, err
	}
	b := bytes.NewBuffer(data)
	h := packet.Header{}
	if err = h.Read(b); err != nil {
		return nil, err
	}
	p = packet.NewServerPool()[h.PacketID]
	if p == nil {
		if r.trace != nil {
			r.trace(fmt.Sprintf("收到未实现的房间控制包：ID=%d，长度=%d", h.PacketID, len(data)))
		}
		return nil, nil
	}
	p.Marshal(encoding.NewReader(b))
	return p, nil
}

func Open(ctx context.Context, cfg config.Config, info auth.CreateInfo, trace func(string)) (r *Room, err error) {
	var id [8]byte
	if _, err = rand.Read(id[:]); err != nil {
		return nil, err
	}
	r = &Room{networkID: binary.LittleEndian.Uint64(id[:]), done: make(chan struct{}), trace: trace}
	if r.networkID == 0 {
		r.networkID = 1
	}
	ok := false
	room := r // Keep cleanup bound to the allocation even when a return sets r=nil.
	defer func() {
		if !ok {
			room.Close()
		}
	}()
	r.control, err = raknet.DialContext(ctx, info.RaknetServerAddress)
	if err != nil {
		return nil, fmt.Errorf("房间转发节点连接失败: %w", err)
	}
	// Cancellation must interrupt every registration read, not just the UDP handshake.
	stopCancel := context.AfterFunc(ctx, func() { room.control.Close() })
	defer stopCancel()
	r.encoder = packet.NewEncoder(r.control)
	r.decoder, err = packet.NewDecoder(r.control)
	if err != nil {
		return nil, err
	}
	if err = r.write(&packet.TanLoginRequest{PlayerID: info.UserUniqueID, Rand: info.RaknetRand, AESRand: info.RaknetAESRand, PlayerName: info.UserPlayerName}); err != nil {
		return nil, err
	}
	p, err := r.read()
	if err != nil {
		return nil, err
	}
	login, valid := p.(*packet.TanLoginResponse)
	if !valid {
		return nil, fmt.Errorf("房间节点返回了非登录响应")
	}
	if login.ErrorCode != 0 {
		return nil, fmt.Errorf("房间节点拒绝登录，错误码 %d", login.ErrorCode)
	}
	if err = r.encoder.EnableEncryption(info.EncryptKeyBytes, info.DecryptKeyBytes); err != nil {
		return nil, err
	}
	if err = r.decoder.EnableEncryption(info.EncryptKeyBytes, info.DecryptKeyBytes); err != nil {
		return nil, err
	}
	privacy := uint8(0)
	if cfg.FriendsOnly {
		privacy = 1
	}
	if err = r.write(&packet.TanCreateRoomRequest{
		Capacity: uint8(cfg.MaxPlayers), Privacy: privacy, Name: "", Slogan: cfg.RoomName, Password: cfg.RoomPassword,
		Tips: encoding.RoomTips{LevelID: "NEMCBridge", GameType: 0, ConstantTestString: "test", ProtocolID: info.ProtocolID, GameVersion: cfg.RoomGameVersion},
		PvP:  cfg.AllowPvP, PlayerAuth: 1, EnableWebRTC: true, OwnerPing: 3, PerfLv: 1,
	}); err != nil {
		return nil, err
	}
	p, err = r.read()
	if err != nil {
		return nil, err
	}
	created, valid := p.(*packet.TanCreateRoomResponse)
	if !valid {
		return nil, fmt.Errorf("房间节点返回了非开房响应")
	}
	if created.ErrorCode != 0 {
		return nil, fmt.Errorf("创建房间失败，错误码 %d（7 表示名称需要 VIP）", created.ErrorCode)
	}
	r.ID = created.RoomID
	if r.ID == 0 {
		return nil, fmt.Errorf("房间节点返回了空房间号")
	}
	// The signaling package keeps its own lifetime context, sends a ping every 50s,
	// and refreshes TURN credentials every 30min without recreating the room.
	r.signal, err = (signaling.Dialer{NetherNetID: r.networkID, RefreshTime: 30 * time.Minute}).DialContext(ctx, info.SignalingServerAddress, info.UserUniqueID, info.SignalingSeed, info.SignalingTicket)
	if err != nil {
		return nil, fmt.Errorf("WebRTC 信令连接失败（连接超时、节点拒绝或信令格式不兼容）")
	}
	settings := webrtc.SettingEngine{LoggerFactory: transportLogs{trace: trace}}
	r.listener, err = (nethernet.ListenConfig{API: webrtc.NewAPI(webrtc.WithSettingEngine(settings)), Log: slog.New(stageLog{trace: trace}), ConnContext: func(parent context.Context, c *nethernet.Conn) context.Context {
		child, cancel := context.WithTimeout(parent, 30*time.Second)
		context.AfterFunc(child, cancel)
		return child
	}}).Listen(tracedSignaling{Signaling: r.signal, trace: trace})
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ok = true
	go r.controlLoop()
	return r, nil
}

func (r *Room) controlLoop() {
	for {
		p, err := r.read()
		if err != nil {
			r.fail(fmt.Errorf("房间连接断开: %w", err))
			return
		}
		if _, ok := p.(*packet.TanNewGuestResponse); ok {
			if r.trace != nil {
				r.trace("收到玩家加入请求，发送服务器就绪信息")
			}
			if err = r.write(&packet.TanNotifyServerReady{ServerAddress: "127.0.0.1|19132", NetherNetID: strconv.FormatUint(r.networkID, 10), WebRTCCompressEnabled: true}); err != nil {
				r.fail(err)
				return
			}
		} else if p != nil && r.trace != nil {
			r.trace(fmt.Sprintf("收到房间控制包：%T", p))
		}
	}
}

func (r *Room) fail(err error) {
	r.errMu.Lock()
	if r.err == nil {
		r.err = err
	}
	r.errMu.Unlock()
	r.Close()
}

// Accept returns the next player's NetherNet connection.
func (r *Room) Accept() (net.Conn, error) { return r.listener.Accept() }

// Done is closed when the room's control connection ends.
func (r *Room) Done() <-chan struct{} { return r.done }

func (r *Room) Error() error { r.errMu.Lock(); defer r.errMu.Unlock(); return r.err }

func (r *Room) Close() {
	r.once.Do(func() {
		close(r.done)
		// Disconnecting the owner's RakNet session removes the room. Avoid inventing
		// a shutdown packet or blocking cancellation on an unacknowledged send.
		if r.control != nil {
			r.control.Close()
		}
		if r.listener != nil {
			r.listener.Close()
		} else if r.signal != nil {
			r.signal.Close(net.ErrClosed)
		}
	})
}
