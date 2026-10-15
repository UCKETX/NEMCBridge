package bridge

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"lobbykeeper/internal/gamebatch"
	"lobbykeeper/internal/netease"
	"lobbykeeper/internal/relay"
)

// NetherNet encrypts at DTLS; RakNet BDS also requires the game AES handshake.
// Terminate the latter using a fresh offline backend identity. Gameplay batches
// are otherwise opaque and keep their original compression and packet layout.
type Bridge struct {
	relay.Stream
	writeMu         sync.Mutex
	key             *ecdsa.PrivateKey
	send, recv      *gameCipher
	trace           func(string)
	reads, writes   int
	clientStages    uint32
	serverPacketIDs map[uint64]bool
	server          netease.ServerConverter
	skinPlayer      uuid.UUID
}

func New(server relay.Stream, trace func(string)) (*Bridge, error) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	return &Bridge{Stream: server, key: key, trace: trace, server: netease.ServerConverter{Trace: trace}}, err
}

func (b *Bridge) Write(p []byte) (int, error) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	original := len(p)
	b.writes++
	if b.writes == 2 {
		var err error
		p, err = b.rewriteLogin(p)
		if err != nil {
			return 0, fmt.Errorf("后端登录转换失败：%w", err)
		}
		if b.trace != nil {
			b.trace("已为本地服务器转换登录密钥，玩家身份与皮肤保留")
		}
	}
	if b.writes > 2 && b.writes <= 64 && len(p) > 1 {
		b.observeWorldStage(p[1:], true)
	}
	if b.writes > 2 && b.writes <= 64 && len(p) > 1 {
		netease.TraceResourcePacks(p[1:], true, b.trace)
	}
	if b.writes > 2 && len(p) > 1 {
		converted, err := netease.ConvertClient(p[1:])
		if err != nil {
			return 0, err
		}
		p = append([]byte{0xfe}, converted...)
	}
	if b.send != nil {
		if len(p) == 0 || p[0] != 0xfe {
			return 0, errors.New("缺少 RakNet 游戏包头")
		}
		p = append([]byte{0xfe}, b.send.encrypt(p[1:])...)
	}
	n, err := b.Stream.Write(p)
	if err != nil {
		return 0, err
	}
	if n != len(p) {
		return 0, io.ErrShortWrite
	}
	return original, nil
}

func (b *Bridge) ReadPacket() ([]byte, error) {
	for {
		p, err := b.Stream.ReadPacket()
		if err != nil {
			return nil, err
		}
		if len(p) == 0 || p[0] != 0xfe {
			continue
		}
		b.reads++
		if b.recv != nil {
			plain, err := b.recv.decrypt(p[1:])
			if err != nil {
				return nil, err
			}
			if b.reads <= 8 {
				netease.TraceResourcePacks(plain, false, b.trace)
			}
			plain, err = b.server.Convert(plain)
			if err != nil {
				return nil, err
			}
			if b.reads <= 512 {
				b.observeWorldStage(plain, false)
			}
			return append([]byte{0xfe}, plain...), nil
		}
		if b.reads != 2 {
			return p, nil
		}
		data, err := gamebatch.Decode(p[1:])
		if err != nil {
			return nil, err
		}
		packets, err := gamebatch.Split(data)
		if err != nil {
			return nil, err
		}
		remaining := [][]byte{}
		handled := false
		for _, pk := range packets {
			id, n := binary.Uvarint(pk)
			if n > 0 && id&0x3ff == 3 {
				size, m := binary.Uvarint(pk[n:])
				if m <= 0 || size > uint64(len(pk)-n-m) {
					return nil, errors.New("服务器加密握手格式错误")
				}
				if err := b.enableGameEncryption(string(pk[n+m : n+m+int(size)])); err != nil {
					return nil, err
				}
				handled = true
			} else {
				remaining = append(remaining, pk)
			}
		}
		if !handled {
			return p, nil
		}
		if len(remaining) > 0 {
			return append([]byte{0xfe, 0xff}, gamebatch.Join(remaining)...), nil
		}
	}
}

// ForgetSkin drops the player's original NetEase skin once the session ends.
func (b *Bridge) ForgetSkin() {
	if b.skinPlayer != uuid.Nil {
		netease.ForgetSkin(b.skinPlayer)
	}
}

func (b *Bridge) enableGameEncryption(token string) error {
	header, claims, err := jwtParts(token)
	if err != nil {
		return err
	}
	var publicText string
	if json.Unmarshal(header["x5u"], &publicText) != nil {
		return errors.New("服务器未提供握手公钥")
	}
	public, err := parseGamePublicKey(publicText)
	if err != nil {
		return err
	}
	if err = verifyGameJWT(token, public); err != nil {
		return errors.New("服务器握手签名无效")
	}
	var saltText string
	if json.Unmarshal(claims["salt"], &saltText) != nil {
		return errors.New("服务器握手缺少 salt")
	}
	salt, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(saltText, "="))
	if err != nil {
		return errors.New("服务器 salt 格式错误")
	}
	x, _ := public.Curve.ScalarMult(public.X, public.Y, b.key.D.Bytes())
	shared := x.FillBytes(make([]byte, 48))
	secret := sha256.Sum256(append(salt, shared...))
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	b.recv = newGameCipher(secret)
	b.send = newGameCipher(secret)
	handshake := append([]byte{0xfe}, b.send.encrypt([]byte{0xff, 1, 4})...)
	if _, err = b.Stream.Write(handshake); err != nil {
		return err
	}
	if b.trace != nil {
		b.trace("本地服务器游戏加密已建立；NetherNet 使用 WebRTC 加密")
	}
	return nil
}

func (b *Bridge) rewriteLogin(wire []byte) ([]byte, error) {
	if len(wire) < 2 || wire[0] != 0xfe {
		return nil, errors.New("登录包缺少 RakNet 包头")
	}
	batch, err := gamebatch.Decode(wire[1:])
	if err != nil {
		return nil, err
	}
	packets, err := gamebatch.Split(batch)
	if err != nil {
		return nil, err
	}
	found := false
	for index, p := range packets {
		id, n := binary.Uvarint(p)
		if n <= 0 || id&0x3ff != 1 {
			continue
		}
		if len(p) < n+4 {
			return nil, errors.New("登录协议字段不完整")
		}
		prefix := p[:n+4]
		length, m := binary.Uvarint(p[n+4:])
		if m <= 0 || length > uint64(len(p)-n-4-m) {
			return nil, errors.New("登录请求长度无效")
		}
		request := p[n+4+m : n+4+m+int(length)]
		identity, skin, err := ParseIdentity(request)
		if err != nil {
			return nil, err
		}
		if b.trace != nil {
			b.trace("客户端皮肤元数据（不含内容）：" + netease.SkinClaimsSummary(skin))
		}
		if b.trace != nil {
			for _, line := range netease.LoginShapeSummary(identity, skin) {
				b.trace(line)
			}
		}
		if id, record, ok := netease.OriginalLoginSkin(identity, skin); ok {
			netease.RememberSkin(id, record)
			b.skinPlayer = id
		}
		skinChange, err := netease.NormalizeLoginSkin(skin)
		if err != nil {
			return nil, err
		}
		if skinChange != "" && b.trace != nil {
			b.trace(skinChange)
		}
		publicRaw, _ := x509.MarshalPKIXPublicKey(&b.key.PublicKey)
		publicText := base64.StdEncoding.EncodeToString(publicRaw)
		identity["XUID"] = "" // Local offline identity does not claim Xbox authentication.
		now := time.Now().Unix()
		certificate, err := SignJWT(b.key, map[string]any{"exp": now + 3600, "nbf": now - 60, "extraData": identity, "identityPublicKey": publicText})
		if err != nil {
			return nil, err
		}
		skinJWT, err := SignJWT(b.key, skin)
		if err != nil {
			return nil, err
		}
		certJSON, _ := json.Marshal(map[string]any{"chain": []string{certificate}})
		envelope, _ := json.Marshal(map[string]any{"Certificate": string(certJSON), "AuthenticationType": 2, "Token": ""})
		translated := binary.LittleEndian.AppendUint32(nil, uint32(len(envelope)))
		translated = append(translated, envelope...)
		translated = binary.LittleEndian.AppendUint32(translated, uint32(len(skinJWT)))
		translated = append(translated, skinJWT...)
		pk := append([]byte(nil), prefix...)
		pk = binary.AppendUvarint(pk, uint64(len(translated)))
		pk = append(pk, translated...)
		packets[index] = pk
		found = true
	}
	if !found {
		return nil, errors.New("第二个客户端批次没有 Login")
	}
	// Uncompressed escape is supported after NetworkSettings, independent of threshold.
	return append([]byte{0xfe, 0xff}, gamebatch.Join(packets)...), nil
}

func ParseIdentity(request []byte) (map[string]any, map[string]json.RawMessage, error) {
	readString := func(p []byte) (string, []byte, error) {
		if len(p) < 4 {
			return "", nil, io.ErrUnexpectedEOF
		}
		size := uint64(binary.LittleEndian.Uint32(p))
		if size > uint64(len(p)-4) {
			return "", nil, io.ErrUnexpectedEOF
		}
		return string(p[4 : 4+int(size)]), p[4+int(size):], nil
	}
	identityText, rest, err := readString(request)
	if err != nil {
		return nil, nil, err
	}
	skinToken, _, err := readString(rest)
	if err != nil {
		return nil, nil, err
	}
	var envelope struct {
		Certificate string
		Chain       []string `json:"chain"`
	}
	if json.Unmarshal([]byte(identityText), &envelope) != nil {
		return nil, nil, errors.New("身份封装不是 JSON")
	}
	if envelope.Certificate != "" {
		var cert struct {
			Chain []string `json:"chain"`
		}
		if json.Unmarshal([]byte(envelope.Certificate), &cert) != nil {
			return nil, nil, errors.New("Certificate 格式无效")
		}
		envelope.Chain = cert.Chain
	}
	if len(envelope.Chain) == 0 {
		return nil, nil, errors.New("缺少玩家证书链")
	}
	var last map[string]json.RawMessage
	var key *ecdsa.PublicKey
	for i, token := range envelope.Chain {
		header, claims, err := jwtParts(token)
		if err != nil {
			return nil, nil, err
		}
		if i == 0 {
			var pub string
			json.Unmarshal(header["x5u"], &pub)
			key, err = parseGamePublicKey(pub)
			if err != nil {
				return nil, nil, err
			}
		}
		if err = verifyGameJWT(token, key); err != nil {
			return nil, nil, errors.New("玩家证书链签名无效")
		}
		var next string
		if json.Unmarshal(claims["identityPublicKey"], &next) == nil && next != "" {
			key, err = parseGamePublicKey(next)
			if err != nil {
				return nil, nil, err
			}
		}
		last = claims
	}
	if err = verifyGameJWT(skinToken, key); err != nil {
		return nil, nil, errors.New("玩家皮肤签名无效")
	}
	_, skin, err := jwtParts(skinToken)
	if err != nil {
		return nil, nil, err
	}
	var identity map[string]any
	if json.Unmarshal(last["extraData"], &identity) != nil || identity["identity"] == nil || identity["displayName"] == nil {
		return nil, nil, errors.New("玩家证书缺少名称或 UUID")
	}
	return identity, skin, nil
}

func jwtParts(token string) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, errors.New("JWT 格式无效")
	}
	var objects [2]map[string]json.RawMessage
	for i := 0; i < 2; i++ {
		raw, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil || json.Unmarshal(raw, &objects[i]) != nil {
			return nil, nil, errors.New("JWT 内容格式无效")
		}
	}
	return objects[0], objects[1], nil
}

func parseGamePublicKey(encoded string) (*ecdsa.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("公钥编码无效")
	}
	parsed, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		return nil, errors.New("公钥格式无效")
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P384() {
		return nil, errors.New("需要 P384 公钥")
	}
	return key, nil
}

func verifyGameJWT(token string, key *ecdsa.PublicKey) error {
	header, _, err := jwtParts(token)
	if err != nil {
		return err
	}
	var alg string
	json.Unmarshal(header["alg"], &alg)
	if alg != "ES384" {
		return errors.New("签名算法不是 ES384")
	}
	parts := strings.Split(token, ".")
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 96 {
		return errors.New("签名长度无效")
	}
	hash := sha512.Sum384([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, hash[:], new(big.Int).SetBytes(sig[:48]), new(big.Int).SetBytes(sig[48:])) {
		return errors.New("签名校验失败")
	}
	return nil
}

func SignJWT(key *ecdsa.PrivateKey, claims any) (string, error) {
	rawKey, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	header, _ := json.Marshal(map[string]any{"alg": "ES384", "x5u": base64.StdEncoding.EncodeToString(rawKey)})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	content := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	hash := sha512.Sum384([]byte(content))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", err
	}
	signature := append(r.FillBytes(make([]byte, 48)), s.FillBytes(make([]byte, 48))...)
	return content + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

type gameCipher struct {
	stream  cipher.Stream
	key     [32]byte
	counter uint64
}

func newGameCipher(key [32]byte) *gameCipher {
	block, _ := aes.NewCipher(key[:])
	iv := append(append([]byte(nil), key[:12]...), 0, 0, 0, 2)
	return &gameCipher{stream: cipher.NewCTR(block, iv), key: key}
}

func (c *gameCipher) checksum(data []byte) []byte {
	h := sha256.New()
	h.Write(binary.LittleEndian.AppendUint64(nil, c.counter))
	h.Write(data)
	h.Write(c.key[:])
	c.counter++
	return h.Sum(nil)[:8]
}

func (c *gameCipher) encrypt(data []byte) []byte {
	out := append([]byte(nil), data...)
	out = append(out, c.checksum(data)...)
	c.stream.XORKeyStream(out, out)
	return out
}

func (c *gameCipher) decrypt(data []byte) ([]byte, error) {
	if len(data) < 8 {
		return nil, errors.New("加密批次太短")
	}
	out := append([]byte(nil), data...)
	c.stream.XORKeyStream(out, out)
	plain := out[:len(out)-8]
	if !bytes.Equal(c.checksum(plain), out[len(out)-8:]) {
		return nil, errors.New("本地服务器游戏包校验失败")
	}
	return plain, nil
}
