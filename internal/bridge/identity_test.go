package bridge

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestBackendXUIDStableAccountMapping(t *testing.T) {
	want := BackendXUID(map[string]any{"netease_uid": "1261597877"})
	n, err := strconv.ParseInt(want, 10, 64)
	if err != nil || n <= 0 || uint64(n)>>56 != 0x4e {
		t.Fatalf("invalid compatibility ID %q", want)
	}
	for _, uid := range []any{"001261597877", float64(1261597877), json.Number("1261597877")} {
		got := BackendXUID(map[string]any{"netease_uid": uid, "displayName": "Renamed", "identity": "changed-player-uuid", "netease_sid": "new-session"})
		if got != want {
			t.Fatalf("account identity changed: %v -> %q", uid, got)
		}
	}
	if other := BackendXUID(map[string]any{"netease_uid": "1261597878"}); other == want {
		t.Fatal("different accounts have the same ID")
	}
	if got := BackendXUID(map[string]any{"XUID": "2535421234567890", "netease_uid": "1261597877"}); got != "2535421234567890" {
		t.Fatal("existing XUID not preserved")
	}
	for _, fields := range []map[string]any{
		{"displayName": "Steve", "identity": "a-player-uuid"},
		{"netease_uid": float64(1.5)},
		{"netease_uid": "not-an-account"},
	} {
		if xuid := BackendXUID(fields); xuid != "" {
			t.Fatalf("invented account for invalid identity: %q", xuid)
		}
	}
	a := BackendXUID(map[string]any{"netease_uid": json.Number("9007199254740992")})
	b := BackendXUID(map[string]any{"netease_uid": json.Number("9007199254740993")})
	if a == b {
		t.Fatal("large account IDs lost precision")
	}
}
