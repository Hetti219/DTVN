package network

import (
	"fmt"

	"github.com/Hetti219/distributed-ticket-validation/pkg/consensus"
	pb "github.com/Hetti219/distributed-ticket-validation/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

// MessageRouter routes incoming network messages to appropriate handlers
type MessageRouter struct {
	pbftHandler  PBFTHandler
	stateHandler StateHandler
}

// PBFTHandler processes PBFT consensus messages
type PBFTHandler func(msgType pb.ValidatorMessage_Type, payload []byte) error

// StateHandler processes state synchronization messages
type StateHandler func(update *pb.StateUpdate) error

// NewMessageRouter creates a new message router
func NewMessageRouter() *MessageRouter {
	return &MessageRouter{}
}

// RegisterPBFTHandler registers the PBFT message handler
func (r *MessageRouter) RegisterPBFTHandler(handler PBFTHandler) {
	r.pbftHandler = handler
}

// RegisterStateHandler registers the state synchronization handler
func (r *MessageRouter) RegisterStateHandler(handler StateHandler) {
	r.stateHandler = handler
}

// RouteMessage routes a message to the appropriate handler based on its type
func (r *MessageRouter) RouteMessage(data []byte, senderID peer.ID) error {
	// Deserialize the outer ValidatorMessage
	msg, err := consensus.DeserializeValidatorMessage(data)
	if err != nil {
		return fmt.Errorf("failed to deserialize message from peer %s: %w", senderID, err)
	}

	// Route based on message type
	switch msg.Type {
	case pb.ValidatorMessage_PRE_PREPARE,
		pb.ValidatorMessage_PREPARE,
		pb.ValidatorMessage_COMMIT:
		// Route to PBFT handler
		if r.pbftHandler == nil {
			return fmt.Errorf("no PBFT handler registered")
		}
		return r.pbftHandler(msg.Type, msg.Payload)

	case pb.ValidatorMessage_STATE_UPDATE:
		// Route to state handler
		if r.stateHandler == nil {
			return fmt.Errorf("no state handler registered")
		}

		// Deserialize state update
		stateUpdate := &pb.StateUpdate{}
		if err := proto.Unmarshal(msg.Payload, stateUpdate); err != nil {
			return fmt.Errorf("failed to unmarshal state update: %w", err)
		}

		return r.stateHandler(stateUpdate)

	case pb.ValidatorMessage_VIEW_CHANGE:
		// TODO: Implement view change handling
		fmt.Printf("Received VIEW_CHANGE message from %s (not yet implemented)\n", senderID)
		return nil

	case pb.ValidatorMessage_CHECKPOINT:
		// TODO: Implement checkpoint handling
		fmt.Printf("Received CHECKPOINT message from %s (not yet implemented)\n", senderID)
		return nil

	case pb.ValidatorMessage_HEARTBEAT:
		// TODO: Implement heartbeat handling
		fmt.Printf("Received HEARTBEAT message from %s (not yet implemented)\n", senderID)
		return nil

	default:
		return fmt.Errorf("unknown message type: %v from peer %s", msg.Type, senderID)
	}
}
