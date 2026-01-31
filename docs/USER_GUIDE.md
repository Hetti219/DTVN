# DTVN User Guide

## What is DTVN?

DTVN (Distributed Ticket Validation Network) is a system that validates event tickets across multiple computers. It prevents the same ticket from being used twice and ensures all validators agree on whether a ticket is valid.

## System Requirements

- A computer running Linux, macOS, or Windows
- Go programming language (version 1.21 or higher)
- At least 512MB of available RAM
- Internet connection for multi-node setups

## Installation

1. Download the project files from the repository
2. Open a terminal/command prompt
3. Navigate to the project folder
4. Run the build command:

```bash
make build
```

This creates the validator program in the `bin/` folder.

## Starting the System

### Single Validator (Simplest Setup)

To run one validator node:

```bash
make run-validator
```

The validator starts and listens for ticket validation requests on port 8080.

### Multiple Validators (Network Setup)

To run a network of 4 validators:

```bash
make run-network
```

This starts 4 validators that work together:
- Validator 0: port 8081
- Validator 1: port 8082
- Validator 2: port 8083
- Validator 3: port 8084

## Using the System

### Validating a Ticket

Send a validation request using curl or any HTTP client:

```bash
curl -X POST http://localhost:8080/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001", "data": "event-info"}'
```

**Response:**
```json
{
  "success": true,
  "message": "Ticket validated successfully",
  "ticket_id": "TICKET-001"
}
```

### Checking Ticket Status

To see if a ticket has been validated:

```bash
curl http://localhost:8080/api/v1/tickets/TICKET-001
```

**Response:**
```json
{
  "id": "TICKET-001",
  "state": "VALIDATED",
  "data": "event-info"
}
```

### Consuming a Ticket

Mark a ticket as used:

```bash
curl -X POST http://localhost:8080/api/v1/tickets/consume \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001"}'
```

### Viewing All Tickets

See all tickets in the system:

```bash
curl http://localhost:8080/api/v1/tickets
```

### Checking System Health

Verify the validator is running properly:

```bash
curl http://localhost:8080/health
```

**Response:** `{"status":"healthy"}`

## Stopping the System

To stop a running validator:
- Press `Ctrl+C` in the terminal where it's running
- The system will shut down gracefully

## Common Issues and Solutions

### Port Already in Use

**Problem:** Error message says port 4001 or 8080 is already in use

**Solution:** Either stop the other program using that port, or start the validator with a different port:

```bash
./bin/validator -port 4002 -api-port 8081
```

### Cannot Connect to Validator

**Problem:** API requests return "Connection refused"

**Solution:**
1. Check if the validator is running: `ps aux | grep validator`
2. Verify you're using the correct port (default: 8080)
3. Try the health check: `curl http://localhost:8080/health`

### Validators Not Finding Each Other

**Problem:** Multiple validators don't connect in a network

**Solution:**
1. Ensure all validators are running
2. Wait 30 seconds for automatic discovery
3. Check that validators use the same total node count
4. Verify firewall isn't blocking connections

### Ticket Validation Fails

**Problem:** Validation request returns an error

**Solution:**
1. Check ticket ID format (should be a string)
2. Verify the validator is healthy
3. Ensure enough validators are running (at least 3 for a 4-node network)
4. Check logs for specific error messages

## Understanding Ticket States

Tickets move through different states:

1. **ISSUED** - Ticket created but not yet validated
2. **PENDING** - Validation in progress
3. **VALIDATED** - Ticket successfully validated
4. **CONSUMED** - Ticket has been used
5. **DISPUTED** - Ticket has been contested

## Important Notes

1. **Ticket IDs Must Be Unique** - Each ticket needs a different ID
2. **Multiple Validators Are Better** - Running 4 or more validators provides fault tolerance
3. **Wait for Consensus** - Validation may take 1-2 seconds as validators agree
4. **Cannot Undo** - Once a ticket is consumed, it cannot be reused
5. **Data Persistence** - All data is saved in the `data/` folder

## Testing the System

A simulator is included to test the network:

```bash
make run-simulator
```

This runs automated tests with multiple validators and tickets to verify everything works correctly.

## Quick Reference

| Task | Command |
|------|---------|
| Build system | `make build` |
| Run single validator | `make run-validator` |
| Run 4-validator network | `make run-network` |
| Run tests | `make test` |
| Run simulator | `make run-simulator` |
| Clean up | `make clean` |

## API Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/tickets/validate` | POST | Validate a ticket |
| `/api/v1/tickets/consume` | POST | Mark ticket as used |
| `/api/v1/tickets/{id}` | GET | Get ticket status |
| `/api/v1/tickets` | GET | List all tickets |
| `/health` | GET | Check system health |
| `/api/v1/stats` | GET | View system statistics |

## Getting Help

If you encounter issues not covered here:

1. Check the logs in the terminal where the validator is running
2. Review the configuration in `config/validator.yaml`
3. Ensure all required ports are available
4. Verify Go is installed correctly: `go version`

---

**Document Version:** 2.0
**Last Updated:** 2026-01-31
**For:** Appendix - Non-Technical User Guide
