package state

import (
	"fmt"
	"sync"
)

// Ordering represents the ordering between two vector clocks
type Ordering int

const (
	// Before means this clock happened before the other
	Before Ordering = iota
	// After means this clock happened after the other
	After
	// Equal means the clocks are equal
	Equal
	// Concurrent means the clocks are concurrent (no causal relationship)
	Concurrent
)

// VectorClock represents a vector clock for causality tracking
type VectorClock struct {
	nodeID string
	clocks map[string]uint64
	mu     sync.RWMutex
}

// NewVectorClock creates a new vector clock
func NewVectorClock(nodeID string) *VectorClock {
	return &VectorClock{
		nodeID: nodeID,
		clocks: map[string]uint64{
			nodeID: 0,
		},
	}
}

// Increment increments the clock for this node
func (vc *VectorClock) Increment() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.clocks[vc.nodeID]++
}

// Update updates the clock based on a received clock
func (vc *VectorClock) Update(nodeID string, value uint64) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if current, exists := vc.clocks[nodeID]; !exists || value > current {
		vc.clocks[nodeID] = value
	}
}

// Merge merges this clock with another clock (for anti-entropy)
func (vc *VectorClock) Merge(other *VectorClock) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	for nodeID, otherClock := range other.clocks {
		if current, exists := vc.clocks[nodeID]; !exists || otherClock > current {
			vc.clocks[nodeID] = otherClock
		}
	}
}

// Compare compares this clock with another clock
func (vc *VectorClock) Compare(other *VectorClock) Ordering {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	// Get all node IDs
	allNodes := make(map[string]bool)
	for nodeID := range vc.clocks {
		allNodes[nodeID] = true
	}
	for nodeID := range other.clocks {
		allNodes[nodeID] = true
	}

	lessOrEqual := true
	greaterOrEqual := true

	for nodeID := range allNodes {
		thisClock := vc.clocks[nodeID]
		otherClock := other.clocks[nodeID]

		if thisClock < otherClock {
			greaterOrEqual = false
		}
		if thisClock > otherClock {
			lessOrEqual = false
		}
	}

	if lessOrEqual && greaterOrEqual {
		return Equal
	}
	if lessOrEqual {
		return Before
	}
	if greaterOrEqual {
		return After
	}
	return Concurrent
}

// Get returns the clock value for a specific node
func (vc *VectorClock) Get(nodeID string) uint64 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.clocks[nodeID]
}

// GetAll returns a copy of all clocks
func (vc *VectorClock) GetAll() map[string]uint64 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	clocks := make(map[string]uint64)
	for k, v := range vc.clocks {
		clocks[k] = v
	}
	return clocks
}

// Copy creates a deep copy of this vector clock
func (vc *VectorClock) Copy() *VectorClock {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	newVC := &VectorClock{
		nodeID: vc.nodeID,
		clocks: make(map[string]uint64),
	}

	for k, v := range vc.clocks {
		newVC.clocks[k] = v
	}

	return newVC
}

// String returns a string representation of the vector clock
func (vc *VectorClock) String() string {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	return fmt.Sprintf("VectorClock{node=%s, clocks=%v}", vc.nodeID, vc.clocks)
}

// HappenedBefore checks if this clock happened before another
func (vc *VectorClock) HappenedBefore(other *VectorClock) bool {
	return vc.Compare(other) == Before
}

// HappenedAfter checks if this clock happened after another
func (vc *VectorClock) HappenedAfter(other *VectorClock) bool {
	return vc.Compare(other) == After
}

// IsConcurrent checks if this clock is concurrent with another
func (vc *VectorClock) IsConcurrent(other *VectorClock) bool {
	return vc.Compare(other) == Concurrent
}

// IsEqual checks if this clock is equal to another
func (vc *VectorClock) IsEqual(other *VectorClock) bool {
	return vc.Compare(other) == Equal
}

// Size returns the number of nodes in the vector clock
func (vc *VectorClock) Size() int {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return len(vc.clocks)
}
