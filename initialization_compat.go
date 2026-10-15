package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func (b *bedrockBridge) patchInitialization860(compressed []byte) ([]byte, error) {
	batch, err := decodeGameCompression(compressed)
	if err != nil {
		return nil, err
	}
	if len(batch) > 2<<20 && !b.largeBatchLogged && b.trace != nil {
		b.largeBatchLogged = true
		b.trace(fmt.Sprintf("大游戏批次已接收：传输 %d 字节，解压后 %d 字节，保留原压缩方式", len(compressed), len(batch)))
	}
	packets, err := splitGameBatch(batch)
	if err != nil {
		return nil, err
	}
	changed := false
	// NetEase servers follow a player's skin with ConfirmSkin (228) once it
	// has been checked; clients render other players' skins only after it.
	confirms := map[int][]byte{}
	for i, pk := range packets {
		id, n := binary.Uvarint(pk)
		if n <= 0 {
			return nil, errors.New("初始化数据包头无效")
		}
		switch id & 0x3ff {
		case 9:
			packets[i], err = patchText860(pk, true)
			if err != nil {
				return nil, err
			}
			changed = true
			if b.trace != nil {
				b.trace("聊天协议转换：已补齐网易消息扩展，不记录消息内容")
			}
		case 93:
			packets[i], err = patchPlayerSkin860(pk, true)
			if err != nil {
				return nil, err
			}
			if len(pk) >= n+16 {
				var entry protocol.PlayerListEntry
				protocol.NewReader(bytes.NewReader(pk[n:n+16]), 0, true).UUID(&entry.UUID)
				confirms[i] = confirmSkinPacket([]protocol.PlayerListEntry{entry})
			}
			changed = true
		case 148:
			packets[i], err = patchStackResponseContainers860(pk)
			if err != nil {
				return nil, err
			}
			changed = true
		case 49, 50:
			packets[i], err = patchInventoryContainer860(pk)
			if err != nil {
				return nil, err
			}
			changed = true
		case 307:
			// NetEase SetPlayerInventoryOptions appends a signed Varint32.
			// Eulogist's standard-to-NetEase converter initializes it to zero.
			packets[i] = append(append([]byte(nil), pk...), 0)
			changed = true
			if b.trace != nil {
				b.trace("SetPlayerInventoryOptions 网易转换：补齐界面选项扩展")
			}
		case 187:
			// Eulogist 4.8.0: common AbilityData, then Uint32 and Int64.
			packets[i], err = patchAbilityPacket860(pk)
			if err != nil {
				return nil, err
			}
			changed = true
			if b.trace != nil {
				b.trace("UpdateAbilities 网易转换：补齐 12 字节能力扩展")
			}
		case 52:
			// Three empty arrays (signed/signed/unsigned varint lengths),
			// an empty string and a false flag follow standard ClearRecipes.
			packets[i] = append(append([]byte(nil), pk...), 0, 0, 0, 0, 0)
			changed = true
			if b.trace != nil {
				b.trace("CraftingData 网易转换：补齐配方扩展数组和设置")
			}
		case 12:
			// Eulogist 4.8.0 NetEase AddPlayer (0x140e3a900) writes String,
			// String, Bool, Bool, Uint32, Int64 after BuildPlatform. Without
			// them the client reads past the end of every other player's spawn.
			packets[i] = append(append([]byte(nil), pk...), make([]byte, neteaseAddPlayerExtensionSize)...)
			changed = true
			if b.trace != nil {
				b.trace("AddPlayer 网易转换：补齐 16 字节玩家扩展")
			}
		case 6:
			var removed int
			packets[i], removed, err = neteaseResourcePacksInfo(pk)
			if err != nil {
				return nil, err
			}
			if removed > 0 {
				changed = true
				if b.trace != nil {
					b.trace(fmt.Sprintf("ResourcePacksInfo 网易转换：网易客户端不支持游戏内下载，跳过 %d 个服务器资源包", removed))
				}
			}
		case 7:
			var removed int
			packets[i], removed, err = neteaseResourcePackStack(pk)
			if err != nil {
				return nil, err
			}
			changed = true
			if b.trace != nil {
				b.trace(fmt.Sprintf("ResourcePackStack 网易转换：移除 %d 个服务器包，补齐两个空资源包数组", removed))
			}
		case 11:
			packets[i], err = patchWorldPacket860(pk, b.trace)
			if err != nil {
				return nil, err
			}
			b.worldPatched, changed = true, true
		case 122:
			packets[i], err = patchBiomeDefinitions860(pk)
			if err != nil {
				return nil, fmt.Errorf("生物群系网易转换失败：%w", err)
			}
			b.biomePatched, changed = true, true
			if b.trace != nil {
				b.trace("BiomeDefinitionList 网易转换：逐条补齐两个字段，关闭客户端地形生成数据")
			}
		case 63:
			var added []protocol.PlayerListEntry
			packets[i], added, err = convertPlayerList860(pk, b.trace)
			if err != nil {
				return nil, err
			}
			if len(added) > 0 {
				changed = true
				confirms[i] = confirmSkinPacket(added)
				if b.trace != nil {
					b.trace(fmt.Sprintf("PlayerList 网易转换：补齐 %d 名玩家的列表扩展，并发送皮肤确认", len(added)))
				}
			}
		}
	}
	if !changed {
		return compressed, nil
	}
	if len(confirms) > 0 {
		withConfirms := make([][]byte, 0, len(packets)+len(confirms))
		for i, pk := range packets {
			withConfirms = append(withConfirms, pk)
			if confirm, ok := confirms[i]; ok {
				withConfirms = append(withConfirms, confirm)
			}
		}
		packets = withConfirms
	}
	converted := joinGameBatch(packets)
	return encodeConvertedGameBatch(compressed, converted)
}

func encodeConvertedGameBatch(compressed, converted []byte) ([]byte, error) {
	if compressed[0] == 0 {
		// Preserve negotiated DEFLATE for large recipe/command batches.
		// Escaping the complete batch made a ~1MB fragmented world message.
		var output bytes.Buffer
		output.WriteByte(0)
		writer, err := flate.NewWriter(&output, flate.DefaultCompression)
		if err != nil {
			return nil, err
		}
		if _, err = writer.Write(converted); err != nil {
			writer.Close()
			return nil, err
		}
		if err = writer.Close(); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}
	return append([]byte{0xff}, converted...), nil
}

func patchAbilityPacket860(pk []byte) ([]byte, error) {
	_, n := binary.Uvarint(pk)
	// AbilityData: int64, two uint8 permissions, uint8 layer count;
	// every standard/NetEase layer is uint16+uint32+uint32+3*float32.
	if n <= 0 || len(pk) < n+11 {
		return nil, errors.New("能力包公共字段不完整")
	}
	count := pk[n+10]
	if len(pk) != n+11+int(count)*22 {
		return nil, errors.New("能力包层数与长度不一致")
	}
	result := binary.AppendUvarint(append([]byte(nil), pk[:n+10]...), uint64(count))
	result = append(result, pk[n+11:]...)
	return append(result, make([]byte, 12)...), nil
}

// The inventory-options packet is bidirectional. Strip only its known NetEase
// final field; forwarding it to BDS would leave unread bytes in the packet.
func patchClientInitialization860(compressed []byte) ([]byte, error) {
	batch, err := decodeGameCompression(compressed)
	if err != nil {
		return nil, err
	}
	packets, err := splitGameBatch(batch)
	if err != nil {
		return nil, err
	}
	changed := false
	for i, pk := range packets {
		id, n := binary.Uvarint(pk)
		if n > 0 && id&0x3ff == 9 {
			packets[i], err = patchText860(pk, false)
			if err != nil {
				return nil, err
			}
			changed = true
			continue
		}
		if n > 0 && id&0x3ff == 93 {
			packets[i], err = patchPlayerSkin860(pk, false)
			if err != nil {
				return nil, err
			}
			changed = true
			continue
		}
		if n > 0 && (id&0x3ff == 147 || id&0x3ff == 30) {
			packets[i], err = patchClientInventory860(pk)
			if err != nil {
				return nil, err
			}
			changed = true
			continue
		}
		if n > 0 && id&0x3ff == 144 {
			packets[i], err = patchPlayerAuthInput860(pk)
			if err != nil {
				return nil, err
			}
			changed = true
			continue
		}
		if n <= 0 || id&0x3ff != 307 {
			continue
		}
		cursor := n
		for field := 0; field < 5; field++ {
			if field == 2 {
				if cursor >= len(pk) || pk[cursor] > 1 {
					return nil, errors.New("客户端背包选项布尔字段无效")
				}
				cursor++
				continue
			}
			v, size := binary.Varint(pk[cursor:])
			if size <= 0 || size > 5 || v < -1<<31 || v > 1<<31-1 {
				return nil, errors.New("客户端背包选项数值字段无效")
			}
			cursor += size
		}
		if cursor == len(pk) {
			continue // Also accept a standard client in the integration path.
		}
		v, size := binary.Varint(pk[cursor:])
		if size <= 0 || size > 5 || cursor+size != len(pk) || v < -1<<31 || v > 1<<31-1 {
			return nil, errors.New("客户端网易背包选项扩展无效")
		}
		packets[i] = pk[:cursor]
		changed = true
	}
	if !changed {
		return compressed, nil
	}
	return encodeConvertedGameBatch(compressed, joinGameBatch(packets))
}

// Two empty strings, two false flags, a zero uint32 and a zero int64.
const neteaseAddPlayerExtensionSize = 1 + 1 + 1 + 1 + 4 + 8

// NetEase's add-player list includes five extra groups after Skin.Trusted:
// optional-entry bool[], optional-entry bool[], string[], uint32[], bool[].
// neomega-core names the uint32 group GrowthLevels (NetEase launcher level).
// Each group has exactly the player count, without another length prefix.
// Eulogist 4.8.0 uses absent entries, empty strings and zero levels/flags.
func patchPlayerList860(pk []byte, traces ...func(string)) (result []byte, entries int, err error) {
	result, added, err := convertPlayerList860(pk, traces...)
	return result, len(added), err
}

// convertPlayerList860 also returns the added players for ConfirmSkin.
func convertPlayerList860(pk []byte, traces ...func(string)) (result []byte, added []protocol.PlayerListEntry, err error) {
	defer func() {
		if recover() != nil {
			result, added, err = nil, nil, errors.New("玩家列表皮肤或字段格式无效")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 || len(pk) < n+2 {
		return nil, nil, errors.New("玩家列表包不完整")
	}
	if pk[n] == 1 {
		return pk, nil, nil // Removal has no NetEase extensions.
	}
	if pk[n] != 0 {
		return nil, nil, errors.New("玩家列表操作类型无效")
	}
	count, m := binary.Uvarint(pk[n+1:])
	if m <= 0 || count > 1024 || count > uint64(len(pk)-n-1-m)/17 {
		return nil, nil, errors.New("玩家列表数量或长度无效")
	}
	buffer := bytes.NewBuffer(pk[n:])
	var list packet.PlayerList
	list.Marshal(protocol.NewReader(buffer, 0, true))
	if buffer.Len() != 0 || len(list.Entries) != int(count) {
		return nil, nil, errors.New("玩家列表包含未知字段或数量不一致")
	}
	levels := make([]uint32, len(list.Entries))
	for i := range list.Entries {
		entry := &list.Entries[i]
		skin := &entry.Skin
		if len(traces) > 0 && traces[0] != nil {
			traces[0](fmt.Sprintf("后端玩家皮肤：pixels=%dx%d RGBA=%d trusted=%v premium=%v override=%v", skin.SkinImageWidth, skin.SkinImageHeight, len(skin.SkinData), skin.Trusted, skin.PremiumSkin, skin.OverrideAppearance))
		}
		var xuid string
		levels[i], xuid, err = neteaseRecipientEntry(entry.UUID, entry.Username, skin)
		if err != nil {
			return nil, nil, err
		}
		if xuid != "" {
			entry.XUID = xuid
		}
		if len(traces) > 0 && traces[0] != nil {
			traces[0](fmt.Sprintf("发往网易的玩家皮肤：pixels=%dx%d RGBA=%d geometry=%d patch=%d trusted=%v persona=%v premium=%v override=%v", skin.SkinImageWidth, skin.SkinImageHeight, len(skin.SkinData), len(skin.SkinGeometry), len(skin.SkinResourcePatch), skin.Trusted, skin.PersonaSkin, skin.PremiumSkin, skin.OverrideAppearance))
		}
	}
	var body bytes.Buffer
	list.Marshal(protocol.NewWriter(&body, 0))
	result = append(append([]byte(nil), pk[:n]...), body.Bytes()...)
	// Two absent optional profiles and empty strings, one byte per player each.
	result = append(result, make([]byte, int(count)*3)...)
	for _, level := range levels {
		result = binary.LittleEndian.AppendUint32(result, level)
	}
	result = append(result, make([]byte, int(count))...)
	return result, list.Entries, nil
}

// The protocol 860 standard decoder consumes all optional generation data.
// NetEase inserts int32 + string after Rain (byte offset 29) in every entry.
// Terrain is provided by BDS; omit client-side generation consistently with
// StartGame instead of advertising international generation rules to NetEase.
func patchBiomeDefinitions860(pk []byte) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("协议 860 生物群系定义不完整或格式无效")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 {
		return nil, errors.New("生物群系包头无效")
	}
	input := bytes.NewBuffer(pk[n:])
	reader := protocol.NewReader(input, 0, true)
	var count uint32
	reader.Varuint32(&count)
	if count > 4096 {
		return nil, errors.New("生物群系数量超出限制")
	}
	result = binary.AppendUvarint(append([]byte(nil), pk[:n]...), uint64(count))
	for range count {
		var definition protocol.BiomeDefinition
		definition.Marshal(reader)
		definition.ChunkGeneration = protocol.Optional[protocol.BiomeChunkGeneration]{}
		var core bytes.Buffer
		definition.Marshal(protocol.NewWriter(&core, 0))
		if core.Len() < 31 {
			return nil, errors.New("生物群系公共字段不完整")
		}
		encoded := core.Bytes()
		result = append(result, encoded[:29]...)
		result = append(result, 0, 0, 0, 0, 0)
		result = append(result, encoded[29:]...)
	}
	// Validate the string dictionary while retaining its exact original bytes.
	dictionary := append([]byte(nil), input.Bytes()...)
	remaining := dictionary
	dictionaryCount, size := binary.Uvarint(remaining)
	if size <= 0 || dictionaryCount > 65536 {
		return nil, errors.New("生物群系字典数量超出限制")
	}
	remaining = remaining[size:]
	for range dictionaryCount {
		length, size := binary.Uvarint(remaining)
		if size <= 0 || length > uint64(len(remaining)-size) {
			return nil, errors.New("生物群系字典字符串不完整")
		}
		remaining = remaining[size+int(length):]
	}
	if len(remaining) != 0 {
		return nil, errors.New("生物群系包含未知尾部字段")
	}
	return append(result, dictionary...), nil
}
