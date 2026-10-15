package main

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Eulogist 4.8.0: NetEase Text appends []string for chat and one string
// for popup after the three common trailing strings. Keep the common bytes
// (including UTF-8, filtering and subclient header bits) exactly as received.
func patchText860(pk []byte, toNetEase bool) (result []byte, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("聊天包字段不完整或格式无效")
		}
	}()
	_, n := binary.Uvarint(pk)
	if n <= 0 || len(pk) < n+2 || pk[n] > packet.TextTypeObjectAnnouncement {
		return nil, errors.New("聊天包类型或头部无效")
	}
	buffer := bytes.NewBuffer(pk[n:])
	var text packet.Text
	reader := protocol.NewReader(buffer, 0, true)
	text.Marshal(reader)
	commonEnd := len(pk) - buffer.Len()
	if toNetEase {
		if buffer.Len() != 0 {
			return nil, errors.New("后端聊天包包含未知字段")
		}
		if text.TextType == packet.TextTypeChat || text.TextType == packet.TextTypePopup {
			return append(append([]byte(nil), pk...), 0), nil
		}
		return pk, nil
	}
	if buffer.Len() == 0 {
		return pk, nil
	} // Standard test frontends are also accepted.
	switch text.TextType {
	case packet.TextTypeChat:
		var count uint32
		reader.Varuint32(&count)
		if count > 1024 {
			return nil, errors.New("网易聊天扩展数量超出限制")
		}
		for range count {
			var value string
			reader.String(&value)
		}
	case packet.TextTypePopup:
		var value string
		reader.String(&value)
	default:
		return nil, errors.New("聊天包类型不支持该网易扩展")
	}
	if buffer.Len() != 0 {
		return nil, errors.New("网易聊天包包含未知尾部字段")
	}
	return append([]byte(nil), pk[:commonEnd]...), nil
}
