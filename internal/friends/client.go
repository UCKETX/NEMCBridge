// Package friends handles the robot account's pending NetEase friend requests.
package friends

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const releaseURL = "https://g79.update.netease.com/serverlist/adr_release.0.17.json"

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

type client struct {
	http           *http.Client
	metadata       string
	gateway        string
	uid            string
	key            []byte
	coreSigning    bool
	signingChecked bool
}

type application struct {
	UID       json.RawMessage `json:"uid"`
	Nickname  string          `json:"nickname"`
	ApplyTime json.RawMessage `json:"apply_time"`
}

// dynamicToken signs the exact HTTP body using MD5(UserToken), which is already
// present in the room's encryption key. The raw login token is not needed.
func dynamicToken(key []byte, path string, body []byte) string {
	return signToken(key, path, body, false)
}

// The social API gateway XORs the rotated bytes directly. The game core uses
// a different variant that reverses each rotated byte before the XOR.
func signToken(key []byte, path string, body []byte, core bool) string {
	sum := md5.Sum([]byte(hex.EncodeToString(key) + string(body) + "0eGsBkhl" + strings.TrimSuffix(path, "?")))
	text := []byte(hex.EncodeToString(sum[:]))
	encoded := make([]byte, len(text))
	for i, b := range text {
		rotated := b<<6 | text[(i+1)%len(text)]>>2
		if core {
			rotated = bits.Reverse8(rotated)
		}
		encoded[i] = b ^ rotated
	}
	return strings.NewReplacer("+", "m", "/", "o").Replace(base64.StdEncoding.EncodeToString(encoded)[:16]) + "1"
}

func accountID(raw json.RawMessage) (uint64, bool) {
	s := string(raw)
	if len(s) > 0 && s[0] == '"' {
		if json.Unmarshal(raw, &s) != nil {
			return 0, false
		}
	}
	id, err := strconv.ParseUint(s, 10, 64)
	return id, err == nil && id != 0
}

// request deliberately returns only status/code diagnostics, never response
// bodies, headers or HTTP errors that might contain account credentials.
func (c *client) request(ctx context.Context, path string, body []byte, result any) error {
	method, address := http.MethodPost, c.gateway+path
	if path == "" {
		method, address = http.MethodGet, c.metadata
	}
	req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("好友接口地址无效")
	}
	if path != "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
		req.Header.Set("user-id", c.uid)
		req.Header.Set("user-token", signToken(c.key, path, body, c.coreSigning))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("好友接口连接失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("好友接口返回 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 || json.Unmarshal(b, result) != nil {
		return fmt.Errorf("好友接口响应格式无效")
	}
	return nil
}

func (c *client) discover(ctx context.Context) error {
	var release struct {
		Gateway string `json:"ApiGatewayUrl"`
	}
	if err := c.request(ctx, "", nil, &release); err != nil {
		return err
	}
	u, err := url.Parse(release.Gateway)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("网易好友接口配置无效")
	}
	c.gateway = strings.TrimRight(release.Gateway, "/")
	return nil
}

func (c *client) pending(ctx context.Context) ([]application, error) {
	applications, err := c.fetchPending(ctx)
	if err == nil {
		c.signingChecked = true
		return applications, nil
	}
	// Negotiate only on a read-only request. Never retry an acceptance with a
	// different signature: the first write may already have taken effect.
	if code, ok := err.(responseError); ok && code.code == 22 && !c.signingChecked && ctx.Err() == nil {
		c.signingChecked = true
		c.coreSigning = true
		applications, fallbackErr := c.fetchPending(ctx)
		if fallbackErr == nil {
			return applications, nil
		}
		c.coreSigning = false
		return nil, fallbackErr
	}
	return nil, err
}

type responseError struct {
	action string
	code   int
}

func (e responseError) Error() string {
	return fmt.Sprintf("%s失败，错误码 %d", e.action, e.code)
}

func (c *client) fetchPending(ctx context.Context) ([]application, error) {
	var result struct {
		Code     *int          `json:"code"`
		Entities []application `json:"entities"`
	}
	if err := c.request(ctx, "/user-addme-friends/", []byte("{}"), &result); err != nil {
		return nil, err
	}
	if result.Code == nil {
		return nil, fmt.Errorf("好友申请列表缺少状态码")
	}
	if *result.Code != 0 {
		return nil, responseError{"查询好友申请", *result.Code}
	}
	return result.Entities, nil
}

func (c *client) accept(ctx context.Context, uid uint64) error {
	body, _ := json.Marshal(struct {
		FID      uint64 `json:"fid"`
		Accept   bool   `json:"accept"`
		AcceptUI string `json:"accept_ui"`
	}{uid, true, "联系人-好友申请"})
	var result struct {
		Code *int `json:"code"`
	}
	if err := c.request(ctx, "/user-reply-friend/", body, &result); err != nil {
		return err
	}
	if result.Code == nil {
		return fmt.Errorf("同意好友申请响应缺少状态码")
	}
	if *result.Code != 0 {
		return responseError{"同意好友申请", *result.Code}
	}
	return nil
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func displayName(s string) string {
	r := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	if len(r) > 40 {
		r = r[:40]
	}
	return string(r)
}

type worker struct {
	client   *client
	accepted map[uint64]string
	pace     time.Duration
	event    func(string)
}

func (w *worker) poll(ctx context.Context) error {
	if w.client.gateway == "" {
		if err := w.client.discover(ctx); err != nil {
			return err
		}
	}
	applications, err := w.client.pending(ctx)
	if err != nil {
		return err
	}
	// Drop IDs no longer in the pending list, so a future new application from
	// the same account can be accepted and the cache remains bounded.
	present := make(map[uint64]bool)
	for _, a := range applications {
		if id, ok := accountID(a.UID); ok {
			present[id] = true
		}
	}
	for id := range w.accepted {
		if !present[id] {
			delete(w.accepted, id)
		}
	}
	count := 0
	for _, a := range applications {
		id, ok := accountID(a.UID)
		if !ok || strconv.FormatUint(id, 10) == w.client.uid {
			continue
		}
		stamp := string(a.ApplyTime)
		if old, seen := w.accepted[id]; seen && old == stamp {
			continue
		}
		if ctx.Err() != nil || count == 20 {
			break
		}
		if count > 0 && !wait(ctx, w.pace) {
			return ctx.Err()
		}
		if err := w.client.accept(ctx, id); err != nil {
			return err
		}
		w.accepted[id] = stamp
		count++
		w.event(fmt.Sprintf("已自动同意好友申请：%s（%d）", displayName(a.Nickname), id))
	}
	return ctx.Err()
}

// Run polls only while this room is online. Failures back off independently of
// the game connection, and cancellation interrupts both HTTP requests and waits.
func Run(ctx context.Context, uid uint32, encryptionKey []byte, event, debug func(string)) {
	if uid == 0 || len(encryptionKey) != 32 {
		event("无法开启自动同意好友申请：鉴权材料不完整")
		return
	}
	c := &client{
		http:     newHTTPClient(),
		metadata: releaseURL,
		uid:      strconv.FormatUint(uint64(uid), 10),
		key:      bytes.Clone(encryptionKey[:16]),
	}
	w := &worker{client: c, accepted: make(map[uint64]string), pace: time.Second, event: event}
	run(ctx, w.poll, event, debug)
}

func run(ctx context.Context, poll func(context.Context) error, event, debug func(string)) {
	event("自动同意好友申请已开启（每 30 秒检查）")
	delay := 30 * time.Second
	failed := false
	for ctx.Err() == nil {
		err := poll(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if !failed {
				event("自动同意好友申请暂时失败，将自动重试：" + err.Error())
			}
			debug("好友申请检查失败：" + err.Error())
			failed = true
			delay = min(delay*2, 5*time.Minute)
		} else {
			if failed {
				event("自动同意好友申请已恢复")
			}
			failed = false
			delay = 30 * time.Second
		}
		if !wait(ctx, delay) {
			return
		}
	}
}
