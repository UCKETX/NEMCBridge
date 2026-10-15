package bridge

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// BackendXUID preserves an existing numeric XUID, or supplies a stable local
// compatibility ID for a NetEase account. These self-signed identities are not
// Microsoft/Xbox authentication. Names and per-login session IDs are not used.
func BackendXUID(identity map[string]any) string {
	if xuid, ok := identity["XUID"].(string); ok {
		if n, err := strconv.ParseUint(xuid, 10, 64); err == nil && n != 0 {
			return strconv.FormatUint(n, 10)
		}
	}
	var account string
	switch uid := identity["netease_uid"].(type) {
	case string:
		account = uid
	case json.Number:
		account = string(uid)
	case float64:
		if uid > 0 && uid <= 1<<53 && math.Trunc(uid) == uid {
			account = strconv.FormatFloat(uid, 'f', 0, 64)
		}
	}
	if uid, err := strconv.ParseUint(strings.TrimSpace(account), 10, 64); err == nil && uid != 0 {
		account = "uid:" + strconv.FormatUint(uid, 10)
	} else {
		// Some clients expose only the NetEase account UUID. Never fall back
		// to displayName: a rename must not create a new backend identity.
		text, _ := identity["netease_uuid"].(string)
		id, err := uuid.Parse(text)
		if err != nil || id == uuid.Nil {
			return ""
		}
		account = "uuid:" + id.String()
	}
	sum := sha256.Sum256([]byte("NEMCBridge/netease/xuid/v1\x00" + account))
	// 0x4e ('N') marks this project's compatibility namespace. Keep the value
	// positive even in plugins that parse XUIDs as signed 64-bit integers.
	value := binary.BigEndian.Uint64(sum[:8])&0x00ffffffffffffff | 0x4e00000000000000
	return strconv.FormatUint(value, 10)
}
