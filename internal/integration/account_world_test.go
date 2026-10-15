package integration

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Run with scripts/test_bds_permissions.py --scenario identities. The runner
// grants only these isolated, freshly generated test accounts command access.
func TestBDSAccountSaveIsolation(t *testing.T) {
	prefix := os.Getenv("NEMCBRIDGE_BDS_IDENTITY_UID_PREFIX")
	if prefix == "" {
		t.Skip("requires isolated identity BDS scenario")
	}
	common := uuid.NewString()
	a := map[string]any{"identity": common, "displayName": "AccountTestA", "netease_uid": prefix + "1"}
	b := map[string]any{"identity": common, "displayName": "AccountTestB", "netease_uid": prefix + "2"}
	first := testBDSWorldScenarioCommands(t, false, false, true, true, a, []string{"/tp @s 123.25 90 321.25", "/give @s diamond 7", "/help"}, nil)
	time.Sleep(3 * time.Second) // BDS processes disconnect and flushes the player save.
	second := testBDSWorldScenarioCommands(t, false, false, true, true, b, []string{"/tp @s -123.25 90 -321.25", "/give @s emerald 3", "/help"}, nil)
	if first.EntityID == second.EntityID {
		t.Fatal("different accounts share a backend player")
	}
	if itemCount(first) != 0 || itemCount(second) != 0 {
		t.Fatal("new account inherited an existing inventory")
	}
	time.Sleep(3 * time.Second)
	// A rename and a different per-login UUID must still load account A's save.
	a["identity"], a["displayName"] = uuid.NewString(), "AccountRenamedA"
	again := testBDSWorldScenarioCommands(t, false, false, true, true, a, []string{"/help"}, nil)
	if again.EntityID != first.EntityID {
		t.Fatal("same account lost its persistent identity")
	}
	if again.Position[0] != 123.25 || again.Position[2] != 321.25 || itemCount(again) != 7 {
		t.Fatalf("account A save not retained: position=%v count=%d", again.Position, itemCount(again))
	}
	time.Sleep(3 * time.Second)
	b["identity"] = uuid.NewString()
	other := testBDSWorldScenarioCommands(t, false, false, true, true, b, []string{"/help"}, nil)
	if other.EntityID != second.EntityID || other.Position[0] != -123.25 || other.Position[2] != -321.25 || itemCount(other) != 3 {
		t.Fatalf("account B save not isolated: position=%v count=%d", other.Position, itemCount(other))
	}
	t.Log("PASS: shared client UUID -> separate inventories and positions; reconnect/rename retains each account's save")
}

func itemCount(s bdsPlayerSnapshot) int {
	total := 0
	for _, item := range s.Inventory {
		total += int(item.Stack.Count)
	}
	return total
}
