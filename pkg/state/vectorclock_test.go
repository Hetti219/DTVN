package state

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewVectorClock tests vector clock creation
func TestNewVectorClock(t *testing.T) {
	t.Run("CreateVectorClock", func(t *testing.T) {
		vc := NewVectorClock("node1")
		require.NotNil(t, vc)
		assert.Equal(t, "node1", vc.nodeID)
		assert.Equal(t, uint64(0), vc.Get("node1"))
	})

	t.Run("CreateMultipleVectorClocks", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc2 := NewVectorClock("node2")

		assert.Equal(t, "node1", vc1.nodeID)
		assert.Equal(t, "node2", vc2.nodeID)
		assert.NotEqual(t, vc1.nodeID, vc2.nodeID)
	})
}

// TestIncrement tests clock incrementing
func TestIncrement(t *testing.T) {
	t.Run("IncrementOnce", func(t *testing.T) {
		vc := NewVectorClock("node1")
		assert.Equal(t, uint64(0), vc.Get("node1"))

		vc.Increment()
		assert.Equal(t, uint64(1), vc.Get("node1"))
	})

	t.Run("IncrementMultipleTimes", func(t *testing.T) {
		vc := NewVectorClock("node1")

		for i := 1; i <= 10; i++ {
			vc.Increment()
			assert.Equal(t, uint64(i), vc.Get("node1"))
		}
	})

	t.Run("IncrementOnlyIncrementsSelfNode", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Update("node2", 5)

		vc.Increment()

		assert.Equal(t, uint64(1), vc.Get("node1"))
		assert.Equal(t, uint64(5), vc.Get("node2"))
	})
}

// TestUpdate tests clock updating
func TestUpdate(t *testing.T) {
	t.Run("UpdateNewNode", func(t *testing.T) {
		vc := NewVectorClock("node1")

		vc.Update("node2", 5)
		assert.Equal(t, uint64(5), vc.Get("node2"))
	})

	t.Run("UpdateExistingNodeWithHigherValue", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Update("node2", 3)

		vc.Update("node2", 7)
		assert.Equal(t, uint64(7), vc.Get("node2"))
	})

	t.Run("UpdateExistingNodeWithLowerValue", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Update("node2", 10)

		vc.Update("node2", 5)
		assert.Equal(t, uint64(10), vc.Get("node2")) // Should not decrease
	})

	t.Run("UpdateMultipleNodes", func(t *testing.T) {
		vc := NewVectorClock("node1")

		vc.Update("node2", 3)
		vc.Update("node3", 5)
		vc.Update("node4", 7)

		assert.Equal(t, uint64(3), vc.Get("node2"))
		assert.Equal(t, uint64(5), vc.Get("node3"))
		assert.Equal(t, uint64(7), vc.Get("node4"))
	})
}

// TestMerge tests clock merging
func TestMerge(t *testing.T) {
	t.Run("MergeTwoClocks", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment() // node1: 1
		vc1.Update("node2", 3)

		vc2 := NewVectorClock("node2")
		vc2.Increment() // node2: 1
		vc2.Increment() // node2: 2
		vc2.Update("node3", 5)

		vc1.Merge(vc2)

		assert.Equal(t, uint64(1), vc1.Get("node1"))
		assert.Equal(t, uint64(3), vc1.Get("node2")) // Keeps higher value
		assert.Equal(t, uint64(5), vc1.Get("node3")) // Adopts new node
	})

	t.Run("MergeWithHigherValues", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Update("node2", 5)

		vc2 := NewVectorClock("node2")
		vc2.Update("node2", 10)

		vc1.Merge(vc2)

		assert.Equal(t, uint64(10), vc1.Get("node2"))
	})

	t.Run("MergeWithLowerValues", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Update("node2", 10)

		vc2 := NewVectorClock("node2")
		vc2.Update("node2", 3)

		vc1.Merge(vc2)

		assert.Equal(t, uint64(10), vc1.Get("node2")) // Keeps higher value
	})

	t.Run("MergeDisjointClocks", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Update("node2", 5)

		vc2 := NewVectorClock("node3")
		vc2.Update("node4", 7)

		vc1.Merge(vc2)

		assert.Equal(t, uint64(5), vc1.Get("node2"))
		assert.Equal(t, uint64(0), vc1.Get("node3"))
		assert.Equal(t, uint64(7), vc1.Get("node4"))
	})
}

// TestCompare tests clock comparison
func TestCompare(t *testing.T) {
	t.Run("CompareEqual", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment()
		vc1.Update("node2", 3)

		vc2 := NewVectorClock("node2")
		vc2.Update("node1", 1)
		vc2.Update("node2", 3)

		assert.Equal(t, Equal, vc1.Compare(vc2))
		assert.Equal(t, Equal, vc2.Compare(vc1))
	})

	t.Run("CompareBefore", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment() // node1: 1

		vc2 := NewVectorClock("node1")
		vc2.Update("node1", 2) // node1: 2

		assert.Equal(t, Before, vc1.Compare(vc2))
		assert.Equal(t, After, vc2.Compare(vc1))
	})

	t.Run("CompareAfter", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Update("node1", 5)

		vc2 := NewVectorClock("node1")
		vc2.Update("node1", 2)

		assert.Equal(t, After, vc1.Compare(vc2))
		assert.Equal(t, Before, vc2.Compare(vc1))
	})

	t.Run("CompareConcurrent", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment() // node1: 1
		vc1.Update("node2", 0)

		vc2 := NewVectorClock("node2")
		vc2.Increment() // node2: 1
		vc2.Update("node1", 0)

		assert.Equal(t, Concurrent, vc1.Compare(vc2))
		assert.Equal(t, Concurrent, vc2.Compare(vc1))
	})

	t.Run("CompareComplexConcurrent", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment()        // node1: 1
		vc1.Update("node2", 5) // node2: 5
		vc1.Update("node3", 2) // node3: 2

		vc2 := NewVectorClock("node2")
		vc2.Update("node1", 0) // node1: 0
		vc2.Update("node2", 7) // node2: 7
		vc2.Update("node3", 1) // node3: 1

		// vc1 has node1:1 > 0, node3:2 > 1
		// vc2 has node2:7 > 5
		// They are concurrent
		assert.Equal(t, Concurrent, vc1.Compare(vc2))
	})
}

// TestHelperMethods tests helper methods
func TestHelperMethods(t *testing.T) {
	t.Run("HappenedBefore", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment()

		vc2 := NewVectorClock("node1")
		vc2.Update("node1", 2)

		assert.True(t, vc1.HappenedBefore(vc2))
		assert.False(t, vc2.HappenedBefore(vc1))
	})

	t.Run("HappenedAfter", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Update("node1", 5)

		vc2 := NewVectorClock("node1")
		vc2.Update("node1", 2)

		assert.True(t, vc1.HappenedAfter(vc2))
		assert.False(t, vc2.HappenedAfter(vc1))
	})

	t.Run("IsConcurrent", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment()

		vc2 := NewVectorClock("node2")
		vc2.Increment()

		assert.True(t, vc1.IsConcurrent(vc2))
		assert.True(t, vc2.IsConcurrent(vc1))
	})

	t.Run("IsEqual", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment()
		vc1.Update("node2", 3)

		vc2 := NewVectorClock("node2")
		vc2.Update("node1", 1)
		vc2.Update("node2", 3)

		assert.True(t, vc1.IsEqual(vc2))
		assert.True(t, vc2.IsEqual(vc1))
	})
}

// TestGet tests getting clock values
func TestGet(t *testing.T) {
	t.Run("GetExistingNode", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Increment()
		vc.Update("node2", 5)

		assert.Equal(t, uint64(1), vc.Get("node1"))
		assert.Equal(t, uint64(5), vc.Get("node2"))
	})

	t.Run("GetNonExistentNode", func(t *testing.T) {
		vc := NewVectorClock("node1")

		assert.Equal(t, uint64(0), vc.Get("non-existent"))
	})
}

// TestGetAll tests getting all clock values
func TestGetAll(t *testing.T) {
	t.Run("GetAllClocks", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Increment()
		vc.Update("node2", 3)
		vc.Update("node3", 5)

		clocks := vc.GetAll()
		assert.Len(t, clocks, 3)
		assert.Equal(t, uint64(1), clocks["node1"])
		assert.Equal(t, uint64(3), clocks["node2"])
		assert.Equal(t, uint64(5), clocks["node3"])
	})

	t.Run("GetAllIsACopy", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Increment()

		clocks := vc.GetAll()
		clocks["node1"] = 100 // Modify the copy

		// Original should not be affected
		assert.Equal(t, uint64(1), vc.Get("node1"))
	})
}

// TestCopy tests copying vector clocks
func TestCopy(t *testing.T) {
	t.Run("CopyVectorClock", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment()
		vc1.Update("node2", 5)

		vc2 := vc1.Copy()

		assert.Equal(t, "node1", vc2.nodeID)
		assert.Equal(t, uint64(1), vc2.Get("node1"))
		assert.Equal(t, uint64(5), vc2.Get("node2"))
	})

	t.Run("CopyIsIndependent", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment()

		vc2 := vc1.Copy()
		vc2.Increment() // Modify the copy

		// Original should not be affected
		assert.Equal(t, uint64(1), vc1.Get("node1"))
		assert.Equal(t, uint64(2), vc2.Get("node1"))
	})

	t.Run("CopyPreservesAllNodes", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Update("node2", 3)
		vc1.Update("node3", 5)
		vc1.Update("node4", 7)

		vc2 := vc1.Copy()

		assert.Equal(t, uint64(3), vc2.Get("node2"))
		assert.Equal(t, uint64(5), vc2.Get("node3"))
		assert.Equal(t, uint64(7), vc2.Get("node4"))
	})
}

// TestString tests string representation
func TestString(t *testing.T) {
	t.Run("StringRepresentation", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Increment()

		str := vc.String()
		assert.Contains(t, str, "node1")
		assert.Contains(t, str, "VectorClock")
	})
}

// TestSize tests size tracking
func TestSize(t *testing.T) {
	t.Run("SizeWithOneNode", func(t *testing.T) {
		vc := NewVectorClock("node1")
		assert.Equal(t, 1, vc.Size())
	})

	t.Run("SizeWithMultipleNodes", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Update("node2", 1)
		vc.Update("node3", 1)
		vc.Update("node4", 1)

		assert.Equal(t, 4, vc.Size())
	})

	t.Run("SizeDoesNotIncreaseOnUpdate", func(t *testing.T) {
		vc := NewVectorClock("node1")
		vc.Update("node2", 1)

		initialSize := vc.Size()
		vc.Update("node2", 5) // Update existing node

		assert.Equal(t, initialSize, vc.Size())
	})
}

// TestConcurrentOperations tests thread safety
func TestConcurrentOperations(t *testing.T) {
	t.Run("ConcurrentIncrement", func(t *testing.T) {
		vc := NewVectorClock("node1")

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func() {
				for j := 0; j < 100; j++ {
					vc.Increment()
				}
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		assert.Equal(t, uint64(1000), vc.Get("node1"))
	})

	t.Run("ConcurrentUpdate", func(t *testing.T) {
		vc := NewVectorClock("node1")

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 10; j++ {
					nodeID := "node-" + string(rune('0'+id))
					vc.Update(nodeID, uint64(j))
				}
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		// Should have node1 + 10 other nodes
		assert.GreaterOrEqual(t, vc.Size(), 11)
	})

	t.Run("ConcurrentMerge", func(t *testing.T) {
		vc1 := NewVectorClock("node1")

		done := make(chan bool)
		for i := 0; i < 5; i++ {
			go func(id int) {
				vc2 := NewVectorClock("node2")
				vc2.Update("merge-node-"+string(rune('0'+id)), uint64(id))
				vc1.Merge(vc2)
				done <- true
			}(i)
		}

		for i := 0; i < 5; i++ {
			<-done
		}
	})

	t.Run("ConcurrentCompare", func(t *testing.T) {
		vc1 := NewVectorClock("node1")
		vc1.Increment()

		vc2 := NewVectorClock("node2")
		vc2.Increment()

		done := make(chan bool)
		for i := 0; i < 20; i++ {
			go func() {
				_ = vc1.Compare(vc2)
				_ = vc1.HappenedBefore(vc2)
				_ = vc1.IsConcurrent(vc2)
				done <- true
			}()
		}

		for i := 0; i < 20; i++ {
			<-done
		}
	})

	t.Run("ConcurrentGetAndUpdate", func(t *testing.T) {
		vc := NewVectorClock("node1")

		var wg sync.WaitGroup
		wg.Add(20)

		// Concurrent gets
		for i := 0; i < 10; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					_ = vc.Get("node1")
					_ = vc.GetAll()
				}
			}()
		}

		// Concurrent updates
		for i := 0; i < 10; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					vc.Update("node-"+string(rune('0'+id)), uint64(j))
				}
			}(i)
		}

		wg.Wait()
	})
}

// TestCausalityScenarios tests real-world causality scenarios
func TestCausalityScenarios(t *testing.T) {
	t.Run("SimpleMessageSequence", func(t *testing.T) {
		// Node1 sends message
		vc1 := NewVectorClock("node1")
		vc1.Increment() // {node1: 1}

		// Node2 receives and processes
		vc2 := NewVectorClock("node2")
		vc2.Merge(vc1)
		vc2.Increment() // {node1: 1, node2: 1}

		assert.True(t, vc1.HappenedBefore(vc2))
	})

	t.Run("ConcurrentUpdates", func(t *testing.T) {
		// Two nodes update concurrently
		vc1 := NewVectorClock("node1")
		vc1.Increment() // {node1: 1}

		vc2 := NewVectorClock("node2")
		vc2.Increment() // {node2: 1}

		assert.True(t, vc1.IsConcurrent(vc2))
	})

	t.Run("ThreeNodeCausality", func(t *testing.T) {
		// Node1 -> Node2 -> Node3
		vc1 := NewVectorClock("node1")
		vc1.Increment() // {node1: 1}

		vc2 := NewVectorClock("node2")
		vc2.Merge(vc1)
		vc2.Increment() // {node1: 1, node2: 1}

		vc3 := NewVectorClock("node3")
		vc3.Merge(vc2)
		vc3.Increment() // {node1: 1, node2: 1, node3: 1}

		assert.True(t, vc1.HappenedBefore(vc2))
		assert.True(t, vc2.HappenedBefore(vc3))
		assert.True(t, vc1.HappenedBefore(vc3))
	})
}
