package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAutoAcceptFriendsUpgradeAndOptOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// Existing configuration files have no auto_accept_friends field.
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil || !c.AutoAcceptFriends {
		t.Fatalf("upgrade default not applied: %v", err)
	}
	c.AutoAcceptFriends = false
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	c, err = Load(path)
	if err != nil || c.AutoAcceptFriends {
		t.Fatalf("explicit opt-out not preserved: %v", err)
	}
}

func TestResourcePacksDefaultAndLegacyUpgrade(t *testing.T) {
	if !Default().ForwardResourcePacks || Default().NetEaseResourcePacks {
		t.Fatal("default must forward server packs using built-in Classic preload")
	}
	for _, raw := range []string{`{}`, `{"forward_resource_packs":false,"netease_resource_packs":false}`, `{"forward_resource_packs":true,"netease_resource_packs":true}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			c, err := Load(path)
			if err != nil || !c.ForwardResourcePacks {
				t.Fatal("legacy resource setting was not upgraded", err)
			}
			var original map[string]bool
			json.Unmarshal([]byte(raw), &original)
			if c.NetEaseResourcePacks != original["netease_resource_packs"] {
				t.Fatal("custom profile preference changed")
			}
		}
	}
}

func TestRoomCapacityAndOldConfiguration(t *testing.T) {
	for _, limit := range []int{1, 11, 40} {
		c := Default()
		c.MaxPlayers = limit
		if c.Validate() == nil {
			t.Fatalf("accepted invalid room capacity %d", limit)
		}
	}
	for _, limit := range []int{2, 10, 11, 40} {
		path := filepath.Join(t.TempDir(), "config.json")
		old := Default()
		old.MaxPlayers = limit
		old.RoomPassword = "fixture-password"
		old.AutoAcceptFriends = false
		data, _ := json.Marshal(old)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		want := old
		if limit > 10 {
			want.MaxPlayers = 10
		}
		for range 2 {
			got, err := Load(path)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("capacity %d migration changed other settings or was not persisted: %v", limit, err)
			}
		}
	}
}
