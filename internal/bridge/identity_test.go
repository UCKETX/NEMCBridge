package bridge

import (
	"encoding/json"
	"github.com/google/uuid"
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

func TestBackendUUIDAccountIsolation(t *testing.T) {
	common := uuid.NewString()
	a, err := BackendUUID(map[string]any{"identity": common, "netease_uid": "1261597877"})
	if err != nil || a == uuid.Nil {
		t.Fatal(a, err)
	}
	b, err := BackendUUID(map[string]any{"identity": common, "netease_uid": "1261597878"})
	if err != nil || a == b {
		t.Fatal("different accounts share UUID", err)
	}
	for _, uid := range []any{"001261597877", json.Number("1261597877"), float64(1261597877)} {
		got, err := BackendUUID(map[string]any{"identity": uuid.NewString(), "netease_uid": uid, "displayName": "Renamed", "netease_sid": "changed"})
		if err != nil || got != a {
			t.Fatal("reconnect or rename changed UUID", got, err)
		}
	}
	for _, fields := range []map[string]any{{"netease_uid": "bad"}, {"netease_uid": 1.5}, {"netease_uuid": "bad"}} {
		fields["identity"] = common
		if _, err := BackendUUID(fields); err == nil {
			t.Fatal("invalid NetEase identity accepted")
		}
	}
	got, err := BackendUUID(map[string]any{"identity": common})
	if err != nil || got.String() != common {
		t.Fatal("ordinary Bedrock UUID changed")
	}
	accountID := uuid.NewString()
	one, _ := BackendUUID(map[string]any{"netease_uuid": accountID})
	two, _ := BackendUUID(map[string]any{"netease_uuid": accountID, "identity": common})
	if one == uuid.Nil || one != two {
		t.Fatal("NetEase account UUID fallback unstable")
	}
}
