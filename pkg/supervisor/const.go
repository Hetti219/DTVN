package supervisor

// JSON keys used in the supervisor's HTTP responses and WebSocket events.
const (
	jsonKeyType            = "type"
	jsonKeyTicketID        = "ticket_id"
	jsonKeyTimestamp       = "timestamp"
	jsonKeyNodeID          = "node_id"
	jsonKeyNodes           = "nodes"
	jsonKeyNodesCount      = "nodes_count"
	jsonKeySuccess         = "success"
	jsonKeyMessage         = "message"
	jsonKeyData            = "data"
	jsonKeyStatus          = "status"
	jsonKeySimulatorStatus = "simulator_status"
	jsonKeySupervisor      = "supervisor"
)

// supervisorIdentifier is the node id reported for messages that originate
// from the supervisor itself rather than from a managed validator node.
const supervisorIdentifier = "supervisor"
