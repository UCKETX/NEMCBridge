package config

import (
	"os"
	"path/filepath"
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
