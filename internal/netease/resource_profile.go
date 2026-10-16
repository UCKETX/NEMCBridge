package netease

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"nemcbridge/internal/fileutil"
)

const ResourceProfileFile = "netease-resource-profile.json"

// ResourceProfile contains server-supplied pack keys. Keep it out of status
// responses, logs, source control and release archives. It holds metadata only,
// not a copy of the third-party resource files.
type ResourceProfile struct {
	ProtocolID int      `json:"game_protocol"`
	ItemIDs    []uint64 `json:"component_ids"`
	Info       []byte   `json:"info_packet"`
	Stack      []byte   `json:"stack_packet"`
}

func (p *ResourceProfile) Validate() (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("网易资源配置包格式无效")
		}
	}()
	if p.ProtocolID != 860 || len(p.ItemIDs) == 0 || len(p.ItemIDs) > 255 || len(p.Info)+len(p.Stack) > 512<<10 {
		return errors.New("网易资源配置版本、组件数量或大小无效")
	}
	ids := map[uint64]bool{}
	for _, id := range p.ItemIDs {
		if id == 0 || ids[id] {
			return errors.New("网易组件编号为空或重复")
		}
		ids[id] = true
	}
	infoID, n := binary.Uvarint(p.Info)
	if n <= 0 || infoID != uint64(packet.IDResourcePacksInfo) {
		return errors.New("缺少网易资源包信息")
	}
	b := bytes.NewBuffer(p.Info[n:])
	var info packet.ResourcePacksInfo
	info.Marshal(protocol.NewReader(b, 0, true))
	if b.Len() != 0 || len(info.TexturePacks) == 0 || len(info.TexturePacks) > 64 {
		return errors.New("网易资源包信息数量或尾部无效")
	}
	offered := map[string]bool{}
	for _, v := range info.TexturePacks {
		offered[v.UUID.String()+"\x00"+v.Version] = true
	}
	stackID, n := binary.Uvarint(p.Stack)
	if n <= 0 || stackID != uint64(packet.IDResourcePackStack) {
		return errors.New("缺少网易资源包加载顺序")
	}
	b = bytes.NewBuffer(p.Stack[n:])
	r := protocol.NewReader(b, 0, true)
	var stack packet.ResourcePackStack
	stack.Marshal(r)
	var extra1, extra2 []protocol.StackResourcePack
	protocol.Slice(r, &extra1)
	protocol.Slice(r, &extra2)
	if b.Len() != 0 || len(stack.TexturePacks) == 0 || len(stack.TexturePacks)+len(stack.BehaviourPacks) > 64 {
		return errors.New("网易资源包加载顺序或尾部无效")
	}
	for _, v := range stack.TexturePacks {
		if !offered[v.UUID+"\x00"+v.Version] {
			return errors.New("网易资源包信息与加载顺序不匹配")
		}
	}
	return nil
}

func LoadResourceProfile(path string) (*ResourceProfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("无法读取网易资源配置，请先导入原生房间资源")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("网易资源配置读取失败或超过大小限制")
	}
	var p ResourceProfile
	if json.Unmarshal(b, &p) != nil {
		return nil, errors.New("网易资源配置不是有效 JSON")
	}
	if err = p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

func SaveResourceProfile(path string, p *ResourceProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWrite(path, append(b, '\n'), 0600)
}

// Keep the current subclient header rather than replaying a captured header.
func profilePacket(original, captured []byte) []byte {
	_, n := binary.Uvarint(original)
	_, m := binary.Uvarint(captured)
	return append(append([]byte(nil), original[:n]...), captured[m:]...)
}
