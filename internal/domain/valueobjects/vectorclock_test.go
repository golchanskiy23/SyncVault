package valueobjects

import (
	"testing"
)

func TestVectorClock_Increment(t *testing.T) {
	vc := NewVectorClock()
	vc = vc.Increment("node1")
	vc = vc.Increment("node1")
	vc = vc.Increment("node2")

	if vc["node1"] != 2 {
		t.Errorf("expected node1=2, got %d", vc["node1"])
	}
	if vc["node2"] != 1 {
		t.Errorf("expected node2=1, got %d", vc["node2"])
	}
}

func TestVectorClock_Merge(t *testing.T) {
	a := VectorClock{"node1": 3, "node2": 1}
	b := VectorClock{"node1": 1, "node2": 5, "node3": 2}

	merged := a.Merge(b)

	if merged["node1"] != 3 {
		t.Errorf("expected node1=3, got %d", merged["node1"])
	}
	if merged["node2"] != 5 {
		t.Errorf("expected node2=5, got %d", merged["node2"])
	}
	if merged["node3"] != 2 {
		t.Errorf("expected node3=2, got %d", merged["node3"])
	}
}

func TestVectorClock_Compare_Before(t *testing.T) {
	a := VectorClock{"node1": 1, "node2": 1}
	b := VectorClock{"node1": 2, "node2": 2}

	if a.Compare(b) != VectorClockBefore {
		t.Errorf("expected a < b (Before)")
	}
}

func TestVectorClock_Compare_After(t *testing.T) {
	a := VectorClock{"node1": 3, "node2": 3}
	b := VectorClock{"node1": 1, "node2": 2}

	if a.Compare(b) != VectorClockAfter {
		t.Errorf("expected a > b (After)")
	}
}

func TestVectorClock_Compare_Equal(t *testing.T) {
	a := VectorClock{"node1": 2, "node2": 2}
	b := VectorClock{"node1": 2, "node2": 2}

	if a.Compare(b) != VectorClockEqual {
		t.Errorf("expected a == b (Equal)")
	}
}

func TestVectorClock_Compare_Concurrent(t *testing.T) {
	// node1 advanced on a, node2 advanced on b → concurrent
	a := VectorClock{"node1": 2, "node2": 1}
	b := VectorClock{"node1": 1, "node2": 2}

	if a.Compare(b) != VectorClockConcurrent {
		t.Errorf("expected concurrent conflict")
	}
	if !a.IsConcurrentWith(b) {
		t.Errorf("IsConcurrentWith should return true")
	}
}

func TestVectorClock_ImmutableIncrement(t *testing.T) {
	original := VectorClock{"node1": 1}
	incremented := original.Increment("node1")

	if original["node1"] != 1 {
		t.Errorf("Increment should not mutate original clock")
	}
	if incremented["node1"] != 2 {
		t.Errorf("expected incremented node1=2")
	}
}
