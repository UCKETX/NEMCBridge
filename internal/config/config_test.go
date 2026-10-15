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
