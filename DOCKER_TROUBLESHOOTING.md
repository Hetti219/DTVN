# Docker Troubleshooting Guide

## Common Issues and Fixes

### Issue 1: ❌ Environment Variables Not Expanding in CMD

**Problem:**
```dockerfile
CMD ["-id", "${NODE_ID}", "-port", "4001"]  # Won't work!
```

**Why:** JSON array format (exec form) doesn't support shell variable expansion.

**Solution:** Use shell form OR entrypoint script
```dockerfile
CMD ./validator -id ${NODE_ID:-validator-1}  # Shell form
# OR
ENTRYPOINT ["/root/entrypoint.sh"]  # Script handles variables
```

### Issue 2: ❌ Bootstrap Peer Address Invalid

**Problem:**
```yaml
- "-bootstrap"
- "/ip4/validator-0/tcp/4001"  # Invalid!
```

**Why:**
- Container hostnames don't work directly in libp2p multiaddr `/ip4/` component
- Missing peer ID in format: `/ip4/IP/tcp/PORT/p2p/PEER_ID`

**Solutions:**

**A. Use DHT Discovery (Recommended for Docker)**
Remove bootstrap flag entirely and let DHT handle peer discovery automatically within the Docker network.

**B. Use Entrypoint Script**
Resolve container DNS to IP at runtime:
```sh
BOOTSTRAP_IP=$(getent hosts validator-0 | awk '{print $1}')
```

**C. Use Static IP Addresses**
Configure static IPs in docker-compose:
```yaml
networks:
  dtvn-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.28.0.0/16
services:
  validator-0:
    networks:
      dtvn-network:
        ipv4_address: 172.28.0.10
```

### Issue 3: ❌ Healthcheck Fails - wget Not Found

**Problem:**
```
wget: not found
```

**Why:** Alpine Linux doesn't include `wget` by default.

**Solution:** Add wget to Dockerfile
```dockerfile
RUN apk --no-cache add ca-certificates wget
```

### Issue 4: ❌ Containers Can't Connect to Each Other

**Problem:** Validators start but don't discover peers.

**Possible Causes & Solutions:**

**A. Port Binding Issues**
```yaml
ports:
  - "4001:4001"  # Host:Container
  - "8080:8080"
```
Ensure no port conflicts on the host.

**B. Network Mode**
Use bridge network (default in docker-compose):
```yaml
networks:
  - dtvn-network
```

**C. Firewall Rules**
Docker might block inter-container communication. Check with:
```bash
docker network inspect dtvn-network
```

**D. DHT Bootstrap Time**
Give nodes time to discover each other:
```yaml
healthcheck:
  start_period: 15s  # Wait before first health check
```

### Issue 5: ❌ Data Persistence Issues

**Problem:** Data lost when container restarts.

**Solution:** Use named volumes
```yaml
volumes:
  - validator-0-data:/root/data

volumes:
  validator-0-data:
```

### Issue 6: ❌ P2P Port Already in Use

**Problem:**
```
bind: address already in use
```

**Solution:** Map to different host ports
```yaml
validator-1:
  ports:
    - "4002:4001"  # Map host 4002 -> container 4001
    - "8081:8080"
```

### Issue 7: ❌ Nodes Start But Don't Form Consensus

**Problem:** Nodes run but don't reach consensus.

**Debugging Steps:**

1. **Check logs:**
```bash
docker-compose logs -f validator-0
```

2. **Verify peer count:**
```bash
curl http://localhost:8080/api/v1/stats
```

3. **Check network connectivity:**
```bash
docker exec dtvn-validator-0 ping validator-1
```

4. **Verify Byzantine tolerance:**
- Need minimum 4 nodes for f=1
- Need 7 nodes for f=2
- Formula: f = (n-1)/3

### Issue 8: ❌ Build Fails with CGO Errors

**Problem:**
```
cgo: C compiler not found
```

**Solution:** Install build dependencies
```dockerfile
RUN apk add --no-cache git make gcc musl-dev
```

## Best Practices for Docker Deployment

### 1. **Use Multi-Stage Builds**
```dockerfile
FROM golang:1.21-alpine AS builder
# ... build here ...

FROM alpine:latest
COPY --from=builder /validator .
```
Reduces final image size.

### 2. **Health Checks with Start Period**
```yaml
healthcheck:
  test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
  interval: 10s
  timeout: 5s
  retries: 3
  start_period: 15s  # Important for slow-starting apps
```

### 3. **Proper Dependency Order**
```yaml
depends_on:
  validator-0:
    condition: service_healthy  # Wait for health check
```

### 4. **Resource Limits**
```yaml
deploy:
  resources:
    limits:
      cpus: '0.5'
      memory: 512M
    reservations:
      cpus: '0.25'
      memory: 256M
```

### 5. **Logging**
```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```

## Testing Docker Setup

### 1. Build the Image
```bash
docker-compose build
```

### 2. Start the Network
```bash
docker-compose up -d
```

### 3. Check Logs
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f validator-0
```

### 4. Verify Services Are Running
```bash
docker-compose ps
```

Expected output:
```
NAME                 STATUS        PORTS
dtvn-validator-0     Up (healthy)  0.0.0.0:4001->4001/tcp, ...
dtvn-validator-1     Up (healthy)  0.0.0.0:4002->4001/tcp, ...
```

### 5. Test API
```bash
# Health check
curl http://localhost:8080/health

# Node stats
curl http://localhost:8080/api/v1/stats

# Validate ticket
curl -X POST http://localhost:8080/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id":"TICKET-001","data":"dGVzdA=="}'
```

### 6. Check Peer Connectivity
```bash
# Enter container
docker exec -it dtvn-validator-0 sh

# Check if other nodes are reachable
ping validator-1
ping validator-2
```

### 7. Monitor Metrics
```bash
# Prometheus
curl http://localhost:9999

# Grafana
open http://localhost:3000
```

## Cleanup

```bash
# Stop and remove containers
docker-compose down

# Remove volumes (WARNING: deletes data)
docker-compose down -v

# Remove images
docker-compose down --rmi all
```

## Quick Fixes Summary

| Issue | Quick Fix |
|-------|-----------|
| ENV vars not working | Use shell form CMD or entrypoint script |
| wget not found | Add `wget` to apk install |
| Bootstrap address invalid | Use DHT discovery (remove bootstrap) |
| Peers not connecting | Check network, increase start_period |
| Data lost on restart | Use named volumes |
| Port conflicts | Map to different host ports |
| Build failures | Install gcc, musl-dev in builder stage |
| Slow startup | Increase healthcheck start_period |

## Current Configuration

The current setup:
- ✅ Uses entrypoint script for flexible configuration
- ✅ Includes wget for health checks
- ✅ Proper health check with start_period
- ✅ Named volumes for data persistence
- ✅ Separate network namespace
- ✅ Service dependencies with health conditions
- ✅ DHT-based peer discovery (no hardcoded bootstrap)

You can now deploy with:
```bash
docker-compose up -d
```

And monitor with:
```bash
docker-compose logs -f
```
