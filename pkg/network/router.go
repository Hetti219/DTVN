package network

import (
	"fmt"

	"github.com/Hetti219/DTVN/pkg/consensus"
	pb "github.com/Hetti219/DTVN/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

// MessageRouter routes incoming network messages to appropriate handlers
type MessageRouter struct {
	pbftHandler      PBFTHandler
	stateHandler     StateHandler
	stateSyncHandler StateSyncHandler
}

// PBFTHandler processes PBFT consensus messages
type PBFTHandler func(msgType pb.ValidatorMessage_Type, payload []byte) error

// StateHandler processes state synchronization messages
type StateHandler func(update *pb.StateUpdate) error

// StateSyncHandler processes state sync messages (request/response)
type StateSyncHandler func(msgType pb.ValidatorMessage_Type, payload []byte) error

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

// RegisterStateSyncHandler registers the state sync request/response handler
func (r *MessageRouter) RegisterStateSyncHandler(handler StateSyncHandler) {
	r.stateSyncHandler = handler
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

	case pb.ValidatorMessage_CLIENT_REQUEST:
		// Route to PBFT handler for client request forwarding
		if r.pbftHandler == nil {
			return fmt.Errorf("no PBFT handler registered")
		}
		return r.pbftHandler(msg.Type, msg.Payload)

	case pb.ValidatorMessage_VIEW_CHANGE,
		pb.ValidatorMessage_CHECKPOINT,
		pb.ValidatorMessage_HEARTBEAT:
		// Route to PBFT handler for view change, checkpoint, and heartbeat processing
		if r.pbftHandler == nil {
			return fmt.Errorf("no PBFT handler registered")
		}
		return r.pbftHandler(msg.Type, msg.Payload)

	case pb.ValidatorMessage_STATE_SYNC_REQUEST,
		pb.ValidatorMessage_STATE_SYNC_RESPONSE:
		// Route to state sync handler
		if r.stateSyncHandler == nil {
			return fmt.Errorf("no state sync handler registered")
		}
		return r.stateSyncHandler(msg.Type, msg.Payload)

	default:
		return fmt.Errorf("unknown message type: %v from peer %s", msg.Type, senderID)
	}
}
