package network

import (
	"errors"
	"sync"
	"testing"

	pb "github.com/Hetti219/distributed-ticket-validation/proto"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// TestNewMessageRouter tests router creation
func TestNewMessageRouter(t *testing.T) {
	t.Run("CreateNewRouter", func(t *testing.T) {
		router := NewMessageRouter()
		assert.NotNil(t, router)
		assert.Nil(t, router.pbftHandler)
		assert.Nil(t, router.stateHandler)
		assert.Nil(t, router.stateSyncHandler)
	})
}

// TestRegisterHandlers tests handler registration
func TestRegisterHandlers(t *testing.T) {
	t.Run("RegisterPBFTHandler", func(t *testing.T) {
		router := NewMessageRouter()

		called := false
		handler := func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			called = true
			return nil
		}

		router.RegisterPBFTHandler(handler)
		assert.NotNil(t, router.pbftHandler)

		// Test handler
		router.pbftHandler(pb.ValidatorMessage_PREPARE, []byte("test"))
		assert.True(t, called)
	})

	t.Run("RegisterStateHandler", func(t *testing.T) {
		router := NewMessageRouter()

		called := false
		handler := func(update *pb.StateUpdate) error {
			called = true
			return nil
		}

		router.RegisterStateHandler(handler)
		assert.NotNil(t, router.stateHandler)

		// Test handler
		router.stateHandler(&pb.StateUpdate{})
		assert.True(t, called)
	})

	t.Run("RegisterStateSyncHandler", func(t *testing.T) {
		router := NewMessageRouter()

		called := false
		handler := func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			called = true
			return nil
		}

		router.RegisterStateSyncHandler(handler)
		assert.NotNil(t, router.stateSyncHandler)

		// Test handler
		router.stateSyncHandler(pb.ValidatorMessage_STATE_SYNC_REQUEST, []byte("test"))
		assert.True(t, called)
	})

	t.Run("ReplaceHandlers", func(t *testing.T) {
		router := NewMessageRouter()

		handler1 := func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			return nil
		}
		handler2 := func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			return errors.New("handler2")
		}

		router.RegisterPBFTHandler(handler1)
		router.RegisterPBFTHandler(handler2) // Replace

		// Test that handler2 is active
		err := router.pbftHandler(pb.ValidatorMessage_PREPARE, []byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "handler2")
	})
}

// TestRouteMessage tests message routing
func TestRouteMessage(t *testing.T) {
	// Create a dummy peer ID for testing
	_, pub, _ := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	senderID, _ := peer.IDFromPublicKey(pub)

	t.Run("RoutePrePrepareMessage", func(t *testing.T) {
		router := NewMessageRouter()

		receivedType := pb.ValidatorMessage_Type(-1)
		var receivedPayload []byte

		router.RegisterPBFTHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			receivedType = msgType
			receivedPayload = payload
			return nil
		})

		// Create message
		request := &pb.Request{
			RequestId: "test-request",
			Operation: pb.TicketOperation_VALIDATE,
		}
		payload, _ := proto.Marshal(request)

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_PRE_PREPARE,
			Payload: payload,
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		require.NoError(t, err)
		assert.Equal(t, pb.ValidatorMessage_PRE_PREPARE, receivedType)
		assert.Equal(t, payload, receivedPayload)
	})

	t.Run("RoutePrepareMessage", func(t *testing.T) {
		router := NewMessageRouter()

		receivedType := pb.ValidatorMessage_Type(-1)

		router.RegisterPBFTHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			receivedType = msgType
			return nil
		})

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_PREPARE,
			Payload: []byte("prepare-payload"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		require.NoError(t, err)
		assert.Equal(t, pb.ValidatorMessage_PREPARE, receivedType)
	})

	t.Run("RouteCommitMessage", func(t *testing.T) {
		router := NewMessageRouter()

		receivedType := pb.ValidatorMessage_Type(-1)

		router.RegisterPBFTHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			receivedType = msgType
			return nil
		})

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_COMMIT,
			Payload: []byte("commit-payload"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		require.NoError(t, err)
		assert.Equal(t, pb.ValidatorMessage_COMMIT, receivedType)
	})

	t.Run("RouteStateUpdateMessage", func(t *testing.T) {
		router := NewMessageRouter()

		var receivedUpdate *pb.StateUpdate

		router.RegisterStateHandler(func(update *pb.StateUpdate) error {
			receivedUpdate = update
			return nil
		})

		stateUpdate := &pb.StateUpdate{
			NodeId:    "test-node",
			Timestamp: 12345,
		}
		payload, _ := proto.Marshal(stateUpdate)

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_STATE_UPDATE,
			Payload: payload,
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		require.NoError(t, err)
		require.NotNil(t, receivedUpdate)
		assert.Equal(t, "test-node", receivedUpdate.NodeId)
		assert.Equal(t, int64(12345), receivedUpdate.Timestamp)
	})

	t.Run("RouteClientRequest", func(t *testing.T) {
		router := NewMessageRouter()

		receivedType := pb.ValidatorMessage_Type(-1)

		router.RegisterPBFTHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			receivedType = msgType
			return nil
		})

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_CLIENT_REQUEST,
			Payload: []byte("client-request"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		require.NoError(t, err)
		assert.Equal(t, pb.ValidatorMessage_CLIENT_REQUEST, receivedType)
	})

	t.Run("RouteStateSyncRequest", func(t *testing.T) {
		router := NewMessageRouter()

		receivedType := pb.ValidatorMessage_Type(-1)

		router.RegisterStateSyncHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			receivedType = msgType
			return nil
		})

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_STATE_SYNC_REQUEST,
			Payload: []byte("sync-request"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		require.NoError(t, err)
		assert.Equal(t, pb.ValidatorMessage_STATE_SYNC_REQUEST, receivedType)
	})

	t.Run("RouteStateSyncResponse", func(t *testing.T) {
		router := NewMessageRouter()

		receivedType := pb.ValidatorMessage_Type(-1)

		router.RegisterStateSyncHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			receivedType = msgType
			return nil
		})

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_STATE_SYNC_RESPONSE,
			Payload: []byte("sync-response"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		require.NoError(t, err)
		assert.Equal(t, pb.ValidatorMessage_STATE_SYNC_RESPONSE, receivedType)
	})
}

// TestRouteMessageErrors tests error conditions
func TestRouteMessageErrors(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	senderID, _ := peer.IDFromPublicKey(pub)

	t.Run("InvalidMessageData", func(t *testing.T) {
		router := NewMessageRouter()

		err := router.RouteMessage([]byte("invalid-protobuf-data"), senderID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to deserialize")
	})

	t.Run("PBFTHandlerNotRegistered", func(t *testing.T) {
		router := NewMessageRouter()

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_PREPARE,
			Payload: []byte("test"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no PBFT handler registered")
	})

	t.Run("StateHandlerNotRegistered", func(t *testing.T) {
		router := NewMessageRouter()

		stateUpdate := &pb.StateUpdate{
			NodeId: "test",
		}
		payload, _ := proto.Marshal(stateUpdate)

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_STATE_UPDATE,
			Payload: payload,
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no state handler registered")
	})

	t.Run("StateSyncHandlerNotRegistered", func(t *testing.T) {
		router := NewMessageRouter()

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_STATE_SYNC_REQUEST,
			Payload: []byte("test"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no state sync handler registered")
	})

	t.Run("HandlerReturnsError", func(t *testing.T) {
		router := NewMessageRouter()

		router.RegisterPBFTHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			return errors.New("handler error")
		})

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_PREPARE,
			Payload: []byte("test"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "handler error")
	})

	t.Run("InvalidStateUpdatePayload", func(t *testing.T) {
		router := NewMessageRouter()

		router.RegisterStateHandler(func(update *pb.StateUpdate) error {
			return nil
		})

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_STATE_UPDATE,
			Payload: []byte("invalid-state-update"),
		}
		data, _ := proto.Marshal(msg)

		err := router.RouteMessage(data, senderID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal state update")
	})
}

// TestUnimplementedMessageTypes tests messages not yet implemented
func TestUnimplementedMessageTypes(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	senderID, _ := peer.IDFromPublicKey(pub)

	t.Run("ViewChangeMessage", func(t *testing.T) {
		router := NewMessageRouter()

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_VIEW_CHANGE,
			Payload: []byte("view-change"),
		}
		data, _ := proto.Marshal(msg)

		// Should not error (just logs for now)
		err := router.RouteMessage(data, senderID)
		assert.NoError(t, err)
	})

	t.Run("CheckpointMessage", func(t *testing.T) {
		router := NewMessageRouter()

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_CHECKPOINT,
			Payload: []byte("checkpoint"),
		}
		data, _ := proto.Marshal(msg)

		// Should not error (just logs for now)
		err := router.RouteMessage(data, senderID)
		assert.NoError(t, err)
	})

	t.Run("HeartbeatMessage", func(t *testing.T) {
		router := NewMessageRouter()

		msg := &pb.ValidatorMessage{
			Type:    pb.ValidatorMessage_HEARTBEAT,
			Payload: []byte("heartbeat"),
		}
		data, _ := proto.Marshal(msg)

		// Should not error (just logs for now)
		err := router.RouteMessage(data, senderID)
		assert.NoError(t, err)
	})
}

// TestMultipleMessageRouting tests routing multiple messages
func TestMultipleMessageRouting(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	senderID, _ := peer.IDFromPublicKey(pub)

	t.Run("RouteMultipleMessages", func(t *testing.T) {
		router := NewMessageRouter()

		pbftCount := 0
		stateCount := 0

		router.RegisterPBFTHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			pbftCount++
			return nil
		})

		router.RegisterStateHandler(func(update *pb.StateUpdate) error {
			stateCount++
			return nil
		})

		// Send PBFT messages
		for i := 0; i < 5; i++ {
			msg := &pb.ValidatorMessage{
				Type:    pb.ValidatorMessage_PREPARE,
				Payload: []byte("test"),
			}
			data, _ := proto.Marshal(msg)
			router.RouteMessage(data, senderID)
		}

		// Send state messages
		for i := 0; i < 3; i++ {
			stateUpdate := &pb.StateUpdate{NodeId: "test"}
			payload, _ := proto.Marshal(stateUpdate)

			msg := &pb.ValidatorMessage{
				Type:    pb.ValidatorMessage_STATE_UPDATE,
				Payload: payload,
			}
			data, _ := proto.Marshal(msg)
			router.RouteMessage(data, senderID)
		}

		assert.Equal(t, 5, pbftCount)
		assert.Equal(t, 3, stateCount)
	})
}

// TestConcurrentRouting tests thread safety
func TestConcurrentRouting(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	senderID, _ := peer.IDFromPublicKey(pub)

	t.Run("ConcurrentMessageRouting", func(t *testing.T) {
		router := NewMessageRouter()

		count := 0
		var mu sync.Mutex

		router.RegisterPBFTHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
			mu.Lock()
			count++
			mu.Unlock()
			return nil
		})

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func() {
				msg := &pb.ValidatorMessage{
					Type:    pb.ValidatorMessage_PREPARE,
					Payload: []byte("test"),
				}
				data, _ := proto.Marshal(msg)
				router.RouteMessage(data, senderID)
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		mu.Lock()
		finalCount := count
		mu.Unlock()

		assert.Equal(t, 10, finalCount)
	})
}
