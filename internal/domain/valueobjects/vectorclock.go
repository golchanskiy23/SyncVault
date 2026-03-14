package valueobjects

// VectorClockOrder represents the causal relationship between two vector clocks.
type VectorClockOrder int

const (
	VectorClockBefore     VectorClockOrder = -1 // this happened before other
	VectorClockConcurrent VectorClockOrder = 0  // concurrent — conflict
	VectorClockAfter      VectorClockOrder = 1  // this happened after other
	VectorClockEqual      VectorClockOrder = 2  // identical
)

// VectorClock is a map of nodeID → logical timestamp used for causality tracking.
// Each node increments its own counter on every local event.
// Conflict detection: if neither clock dominates the other → Concurrent.
type VectorClock map[string]uint64

// NewVectorClock creates an empty vector clock.
func NewVectorClock() VectorClock {
	return make(VectorClock)
}

// Increment increments the counter for the given nodeID.
func (vc VectorClock) Increment(nodeID string) VectorClock {
	result := vc.copy()
	result[nodeID]++
	return result
}

// Merge returns a new clock that is the component-wise maximum of vc and other.
// Used when receiving an event from another node.
func (vc VectorClock) Merge(other VectorClock) VectorClock {
	result := vc.copy()
	for nodeID, ts := range other {
		if ts > result[nodeID] {
			result[nodeID] = ts
		}
	}
	return result
}

// Compare returns the causal relationship between vc and other.
func (vc VectorClock) Compare(other VectorClock) VectorClockOrder {
	vcDominates := false
	otherDominates := false

	// Collect all node IDs from both clocks
	allNodes := make(map[string]struct{})
	for k := range vc {
		allNodes[k] = struct{}{}
	}
	for k := range other {
		allNodes[k] = struct{}{}
	}

	for nodeID := range allNodes {
		a := vc[nodeID]
		b := other[nodeID]
		if a > b {
			vcDominates = true
		} else if b > a {
			otherDominates = true
		}
		if vcDominates && otherDominates {
			return VectorClockConcurrent
		}
	}

	switch {
	case !vcDominates && !otherDominates:
		return VectorClockEqual
	case vcDominates:
		return VectorClockAfter
	default:
		return VectorClockBefore
	}
}

// IsConcurrentWith returns true if the two clocks are concurrent (conflict).
func (vc VectorClock) IsConcurrentWith(other VectorClock) bool {
	return vc.Compare(other) == VectorClockConcurrent
}

// copy returns a shallow copy of the vector clock.
func (vc VectorClock) copy() VectorClock {
	result := make(VectorClock, len(vc))
	for k, v := range vc {
		result[k] = v
	}
	return result
}
