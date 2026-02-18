package consensus

import (
	"fmt"

	pb "github.com/Hetti219/DTVN/proto"
	"google.golang.org/protobuf/proto"
)

// SerializePBFTMessage wraps a PBFT message in a ValidatorMessage and serializes it
func SerializePBFTMessage(msgType pb.ValidatorMessage_Type, payload []byte, senderID string) ([]byte, error) {
	msg := &pb.ValidatorMessage{
		Type:      msgType,
		Payload:   payload,
		SenderId:  senderID,
		Timestamp: 0, // Will be set by caller if needed
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ValidatorMessage: %w", err)
	}

	return data, nil
}

// DeserializeValidatorMessage deserializes a ValidatorMessage from bytes
func DeserializeValidatorMessage(data []byte) (*pb.ValidatorMessage, error) {
	msg := &pb.ValidatorMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ValidatorMessage: %w", err)
	}
	return msg, nil
}

// operationToProto converts a string operation name to the protobuf TicketOperation enum.
// Using a switch avoids a map lookup into pb.TicketOperation_value on every serialization.
func operationToProto(op string) pb.TicketOperation {
	switch op {
	case "CONSUME":
		return pb.TicketOperation_CONSUME
	case "DISPUTE":
		return pb.TicketOperation_DISPUTE
	case "QUERY":
		return pb.TicketOperation_QUERY
	default:
		return pb.TicketOperation_VALIDATE
	}
}

// SerializePrePrepare serializes a PrePrepareMsg to protobuf format
func SerializePrePrepare(msg *PrePrepareMsg) ([]byte, error) {
	// Convert Request to protobuf Request
	reqProto := &pb.Request{
		RequestId:  msg.Request.RequestID,
		TicketId:   msg.Request.TicketID,
		Operation:  operationToProto(msg.Request.Operation),
		TicketData: msg.Request.Data,
		Timestamp:  msg.Request.Timestamp,
	}

	prePrepare := &pb.PrePrepare{
		View:     msg.View,
		Sequence: msg.Sequence,
		Digest:   []byte(msg.Digest),
		Request:  reqProto,
	}

	data, err := proto.Marshal(prePrepare)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PrePrepare: %w", err)
	}

	return data, nil
}

// DeserializePrePrepare deserializes a PrePrepareMsg from protobuf format
func DeserializePrePrepare(data []byte) (*PrePrepareMsg, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot deserialize PrePrepare: empty payload")
	}

	prePrepare := &pb.PrePrepare{}
	if err := proto.Unmarshal(data, prePrepare); err != nil {
		return nil, fmt.Errorf("failed to unmarshal PrePrepare: %w", err)
	}

	// Check that Request is not nil (required field for PrePrepare)
	if prePrepare.Request == nil {
		return nil, fmt.Errorf("invalid PrePrepare message: Request field is nil")
	}

	// Convert back to internal format
	req := &Request{
		RequestID: prePrepare.Request.RequestId,
		TicketID:  prePrepare.Request.TicketId,
		Operation: prePrepare.Request.Operation.String(),
		Data:      prePrepare.Request.TicketData,
		Timestamp: prePrepare.Request.Timestamp,
		NodeID:    string(prePrepare.Request.ClientSignature),
	}

	return &PrePrepareMsg{
		View:     prePrepare.View,
		Sequence: prePrepare.Sequence,
		Digest:   string(prePrepare.Digest),
		Request:  req,
	}, nil
}

// SerializePrepare serializes a PrepareMsg to protobuf format
func SerializePrepare(msg *PrepareMsg) ([]byte, error) {
	prepare := &pb.Prepare{
		View:     msg.View,
		Sequence: msg.Sequence,
		Digest:   []byte(msg.Digest),
		NodeId:   msg.NodeID,
	}

	data, err := proto.Marshal(prepare)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Prepare: %w", err)
	}

	return data, nil
}

// DeserializePrepare deserializes a PrepareMsg from protobuf format
func DeserializePrepare(data []byte) (*PrepareMsg, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot deserialize Prepare: empty payload")
	}

	prepare := &pb.Prepare{}
	if err := proto.Unmarshal(data, prepare); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Prepare: %w", err)
	}

	return &PrepareMsg{
		View:     prepare.View,
		Sequence: prepare.Sequence,
		Digest:   string(prepare.Digest),
		NodeID:   prepare.NodeId,
	}, nil
}

// SerializeCommit serializes a CommitMsg to protobuf format
func SerializeCommit(msg *CommitMsg) ([]byte, error) {
	commit := &pb.Commit{
		View:     msg.View,
		Sequence: msg.Sequence,
		Digest:   []byte(msg.Digest),
		NodeId:   msg.NodeID,
	}

	data, err := proto.Marshal(commit)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Commit: %w", err)
	}

	return data, nil
}

// DeserializeCommit deserializes a CommitMsg from protobuf format
func DeserializeCommit(data []byte) (*CommitMsg, error) {
	commit := &pb.Commit{}
	if err := proto.Unmarshal(data, commit); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Commit: %w", err)
	}

	return &CommitMsg{
		View:     commit.View,
		Sequence: commit.Sequence,
		Digest:   string(commit.Digest),
		NodeID:   commit.NodeId,
	}, nil
}

// SerializeRequest serializes a Request to protobuf format
func SerializeRequest(req *Request) ([]byte, error) {
	reqProto := &pb.Request{
		RequestId: req.RequestID,
		TicketId:  req.TicketID,
		Operation: operationToProto(req.Operation),
		TicketData: req.Data,
		Timestamp:  req.Timestamp,
		// NodeID is stored in ClientSignature ([]byte) because the proto Request
		// message has no dedicated node_id field. This is intentional — changing
		// the proto schema would break the wire protocol for existing deployments.
		ClientSignature: []byte(req.NodeID),
	}

	data, err := proto.Marshal(reqProto)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Request: %w", err)
	}

	return data, nil
}

// DeserializeRequest deserializes a Request from protobuf format
func DeserializeRequest(data []byte) (*Request, error) {
	reqProto := &pb.Request{}
	if err := proto.Unmarshal(data, reqProto); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Request: %w", err)
	}

	return &Request{
		RequestID: reqProto.RequestId,
		TicketID:  reqProto.TicketId,
		Operation: reqProto.Operation.String(),
		Data:      reqProto.TicketData,
		Timestamp: reqProto.Timestamp,
		NodeID:    string(reqProto.ClientSignature),
	}, nil
}

// SerializeViewChange serializes a ViewChangeMsg to protobuf format
func SerializeViewChange(msg *ViewChangeMsg) ([]byte, error) {
	viewChange := &pb.ViewChange{
		NewView: msg.NewView,
		NodeId:  msg.NodeID,
	}

	data, err := proto.Marshal(viewChange)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ViewChange: %w", err)
	}

	return data, nil
}

// DeserializeViewChange deserializes a ViewChangeMsg from protobuf format
func DeserializeViewChange(data []byte) (*ViewChangeMsg, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot deserialize ViewChange: empty payload")
	}

	viewChange := &pb.ViewChange{}
	if err := proto.Unmarshal(data, viewChange); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ViewChange: %w", err)
	}

	return &ViewChangeMsg{
		NewView: viewChange.NewView,
		NodeID:  viewChange.NodeId,
	}, nil
}

// SerializeCheckpoint serializes a Checkpoint to protobuf format
func SerializeCheckpoint(ckpt *Checkpoint) ([]byte, error) {
	checkpoint := &pb.Checkpoint{
		Sequence:    ckpt.Sequence,
		StateDigest: []byte(ckpt.StateDigest),
		NodeId:      ckpt.NodeID,
		Timestamp:   ckpt.Timestamp,
	}

	data, err := proto.Marshal(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Checkpoint: %w", err)
	}

	return data, nil
}

// DeserializeCheckpoint deserializes a Checkpoint from protobuf format
func DeserializeCheckpoint(data []byte) (*Checkpoint, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot deserialize Checkpoint: empty payload")
	}

	checkpoint := &pb.Checkpoint{}
	if err := proto.Unmarshal(data, checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Checkpoint: %w", err)
	}

	return &Checkpoint{
		Sequence:    checkpoint.Sequence,
		StateDigest: string(checkpoint.StateDigest),
		NodeID:      checkpoint.NodeId,
		Timestamp:   checkpoint.Timestamp,
	}, nil
}
