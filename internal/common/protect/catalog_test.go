package protect

import "testing"

func TestCriticalSecretInventoryHasRotationAndBoundaries(t *testing.T) {
	items := CriticalSecretInventory()
	if len(items) == 0 {
		t.Fatal("expected critical secret inventory entries")
	}
	for _, item := range items {
		if item.Name == "" || item.AccessBoundary == "" || item.ManagedBy == "" {
			t.Fatalf("incomplete secret inventory entry: %#v", item)
		}
		if item.RotationPeriod <= 0 {
			t.Fatalf("expected positive rotation period for %s", item.Name)
		}
	}
}
