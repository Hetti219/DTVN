# Testing Guide - Synchronization & Duplicate Prevention Fixes

## What Was Fixed:

1. ✅ **View Change Logic** - Nodes now correctly identify the primary in each view
2. ✅ **Single-Node Consensus** - PBFT now works with just 1 node by checking quorum immediately
3. ✅ **Duplicate Validation** - Tickets cannot be validated twice
4. ✅ **State Synchronization** - Validated tickets sync across all nodes via gossip

---

## Test 1: Single Node - Duplicate Prevention

### Start Single Node:
```bash
# Clean up any old data
rm -rf ./data/node0

# Start validator
./bin/validator -id node0 -port 4001 -api-port 8080 -data-dir ./data/node0 -primary -total-nodes 1
```

### Test Duplicate Validation:
```bash
# Terminal 2 - First validation (should succeed)
curl -X POST http://localhost:8080/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TEST-001", "data": "dGVzdA=="}'

# Expected output:
# {"success":true,"message":"Ticket validated successfully"}

# Second validation (should fail)
curl -X POST http://localhost:8080/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TEST-001", "data": "dGVzdA=="}'

# Expected output:
# {"success":false,"error":"Failed to validate ticket: ticket TEST-001 already validated"}
```

### Verify Ticket Status:
```bash
curl http://localhost:8080/api/v1/tickets/TEST-001
# Should show: {"success":true,"data":{"id":"TEST-001","state":"VALIDATED",...}}
```

---

## Test 2: Multi-Node - Synchronization

### Start 4-Node Network:

**Note:** Peer IDs are now deterministic based on node ID. The commands below use the
pre-computed peer ID for node0: `12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C`

**Terminal 1 - Primary (node0):**
```bash
rm -rf ./data/node0
./bin/validator -id node0 -port 4001 -api-port 8081 -data-dir ./data/node0 -primary -total-nodes 4
```

**Terminal 2 - Replica (node1):**
```bash
rm -rf ./data/node1
./bin/validator -id node1 -port 4002 -api-port 8082 -data-dir ./data/node1 -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"
```

**Terminal 3 - Replica (node2):**
```bash
rm -rf ./data/node2
./bin/validator -id node2 -port 4003 -api-port 8083 -data-dir ./data/node2 -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"
```

**Terminal 4 - Replica (node3):**
```bash
rm -rf ./data/node3
./bin/validator -id node3 -port 4004 -api-port 8084 -data-dir ./data/node3 -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"
```

### Verify Peer Connections:
You should see `Discovery: ✅ Connected to bootstrap peer` in each replica's terminal.
Then look for: `Discovered new peer: <peer_id>` messages.

### Test Validation:

**Terminal 5 - Submit to Primary (node0):**
```bash
# Validate a ticket
curl -X POST http://localhost:8081/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "SYNC-TEST-001", "data": "dGVzdA=="}'

# Expected output:
# {"success":true,"message":"Ticket validated successfully"}
```

**Check Validator Logs:**
You should see in node0's terminal:
```
PBFT: Node node0 proposing request for ticket SYNC-TEST-001 (op: VALIDATE)
PBFT: Created PRE-PREPARE for seq 1, view 0
PBFT: Prepare quorum reached for seq 1 (1/1 messages)
PBFT: Commit quorum reached for seq 1 (1/1 messages)
PBFT: ✅ Consensus reached for seq 1, executing request for ticket SYNC-TEST-001
PBFT: ✅ Request executed successfully for ticket SYNC-TEST-001
```

### Verify Synchronization:

**Wait 2-3 seconds for gossip to propagate, then check all nodes:**
```bash
# Check node0 (primary)
curl http://localhost:8081/api/v1/tickets/SYNC-TEST-001

# Check node1
curl http://localhost:8082/api/v1/tickets/SYNC-TEST-001

# Check node2
curl http://localhost:8083/api/v1/tickets/SYNC-TEST-001

# Check node3
curl http://localhost:8084/api/v1/tickets/SYNC-TEST-001
```

**Expected:** All nodes should return the same ticket with `"state":"VALIDATED"`

---

## Test 3: Non-Primary Rejection

Try to validate directly on a non-primary node:

```bash
# This should fail
curl -X POST http://localhost:8082/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "SHOULD-FAIL", "data": "dGVzdA=="}'

# Expected output:
# {"success":false,"error":"Failed to validate ticket: this node is not the primary, please send request to node0"}
```

---

## Troubleshooting

### Issue: "this node is not the primary, please send request to nodeX"

**Cause:** The node's view has changed (view timeout triggered).

**Solution:**
1. Send validation request within 5 seconds of node startup
2. Or identify the current primary from the error message
3. Or increase view timeout in code (currently 5 seconds)

### Issue: Curl hangs with no response

**Checks:**
1. Verify node is running: `curl http://localhost:8080/health`
2. Check validator logs for errors
3. Verify PBFT logs show consensus progression
4. Kill and restart validator

### Issue: Nodes can't discover each other / "no peers connected"

**Most Common Cause:** Bootstrap address is missing the peer ID!

If you see warnings like:
```
⚠️  Bootstrap address /ip4/127.0.0.1/tcp/4001 is missing peer ID component
Discovery: ⚠️  Skipping bootstrap peer without peer ID.
```

**Solution:** Use the **full multiaddr** with node0's deterministic peer ID:
```bash
# WRONG (missing peer ID):
-bootstrap "/ip4/127.0.0.1/tcp/4001"

# CORRECT (includes peer ID):
-bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"
```

**Other Checks:**
1. Ensure node0 is running before starting other nodes
2. Check firewall isn't blocking ports 4001-4004
3. Look for "Discovery: ✅ Connected to bootstrap peer" in logs
4. Verify the peer ID matches node0's actual peer ID

---

## Expected Behavior Summary

### ✅ Single Node:
- First validation succeeds immediately
- Duplicate validation returns error
- PBFT consensus completes instantly (1 node = instant quorum)

### ✅ Multi-Node:
- Only primary (node0) accepts validation requests
- Other nodes reject with "not the primary" error
- After validation on primary, state syncs to all nodes via gossip
- All nodes show same ticket state after sync

### ✅ View Changes:
- If no activity for 5 seconds, view changes
- Primary rotates: view 0 = node0, view 1 = node1, view 2 = node2, view 3 = node3, view 4 = node0...
- Nodes correctly identify who should be primary in each view
