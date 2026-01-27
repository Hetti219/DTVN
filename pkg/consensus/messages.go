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

// SerializePrePrepare serializes a PrePrepareMsg to protobuf format
func SerializePrePrepare(msg *PrePrepareMsg) ([]byte, error) {
	// Convert Request to protobuf Request
	reqProto := &pb.Request{
		RequestId:  msg.Request.RequestID,
		TicketId:   msg.Request.TicketID,
		Operation:  pb.TicketOperation(pb.TicketOperation_value[msg.Request.Operation]),
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
	prePrepare := &pb.PrePrepare{}
	if err := proto.Unmarshal(data, prePrepare); err != nil {
		return nil, fmt.Errorf("failed to unmarshal PrePrepare: %w", err)
	}

	// Convert back to internal format
	req := &Request{
		RequestID: prePrepare.Request.RequestId,
		TicketID:  prePrepare.Request.TicketId,
		Operation: prePrepare.Request.Operation.String(),
		Data:      prePrepare.Request.TicketData,
		Timestamp: prePrepare.Request.Timestamp,
		ClientSig: prePrepare.Request.ClientSignature,
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
		RequestId:       req.RequestID,
		TicketId:        req.TicketID,
		Operation:       pb.TicketOperation(pb.TicketOperation_value[req.Operation]),
		TicketData:      req.Data,
		Timestamp:       req.Timestamp,
		ClientSignature: req.ClientSig,
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
		ClientSig: reqProto.ClientSignature,
	}, nil
}
