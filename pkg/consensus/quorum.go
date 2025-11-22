package consensus

import (
	"fmt"
)

// Vote represents a vote from a node
type Vote struct {
	NodeID    string
	Sequence  int64
	Digest    string
	Signature []byte
	Timestamp int64
}

// QuorumChecker handles quorum verification for Byzantine fault tolerance
type QuorumChecker struct {
	totalNodes int
	f          int // Maximum Byzantine nodes
}

// NewQuorumChecker creates a new quorum checker
func NewQuorumChecker(totalNodes int) *QuorumChecker {
	return &QuorumChecker{
		totalNodes: totalNodes,
		f:          (totalNodes - 1) / 3,
	}
}

// CheckQuorum verifies if we have enough votes for quorum
// For Byzantine fault tolerance, we need 2f+1 votes out of 3f+1 total nodes
func (qc *QuorumChecker) CheckQuorum(votes []Vote) bool {
	required := 2*qc.f + 1
	return len(votes) >= required
}

// CheckPrepareQuorum checks if prepare quorum is reached
func (qc *QuorumChecker) CheckPrepareQuorum(prepares map[string]*PrepareMsg) bool {
	required := 2*qc.f + 1
	return len(prepares) >= required
}

// CheckCommitQuorum checks if commit quorum is reached
func (qc *QuorumChecker) CheckCommitQuorum(commits map[string]*CommitMsg) bool {
	required := 2*qc.f + 1
	return len(commits) >= required
}

// ValidateQuorum validates votes and checks for quorum
func (qc *QuorumChecker) ValidateQuorum(votes []Vote, expectedSequence int64, expectedDigest string) error {
	if !qc.CheckQuorum(votes) {
		return fmt.Errorf("insufficient votes: got %d, need %d", len(votes), 2*qc.f+1)
	}

	// Validate all votes
	validVotes := 0
	for _, vote := range votes {
		if vote.Sequence != expectedSequence {
			continue
		}
		if vote.Digest != expectedDigest {
			continue
		}
		// In production, verify signature here
		validVotes++
	}

	if validVotes < 2*qc.f+1 {
		return fmt.Errorf("insufficient valid votes: got %d, need %d", validVotes, 2*qc.f+1)
	}

	return nil
}

// GetRequiredQuorum returns the number of votes required for quorum
func (qc *QuorumChecker) GetRequiredQuorum() int {
	return 2*qc.f + 1
}

// GetMaxByzantineNodes returns the maximum number of Byzantine nodes tolerated
func (qc *QuorumChecker) GetMaxByzantineNodes() int {
	return qc.f
}

// CanTolerateFailures checks if the system can tolerate current failures
func (qc *QuorumChecker) CanTolerateFailures(activeNodes int) bool {
	// We need at least 2f+1 active nodes
	return activeNodes >= 2*qc.f+1
}

// IsValidNetworkSize checks if network size is valid for Byzantine tolerance
func IsValidNetworkSize(totalNodes int) bool {
	// Network must have at least 4 nodes for f=1 (3f+1 = 4)
	return totalNodes >= 4 && (totalNodes-1)%3 == 0
}

// CalculateOptimalNetworkSize calculates optimal network size for given fault tolerance
func CalculateOptimalNetworkSize(byzantineNodes int) int {
	// For f Byzantine nodes, we need 3f+1 total nodes
	return 3*byzantineNodes + 1
}
