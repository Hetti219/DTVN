# Docker Guide for Distributed Ticket Validation Network (DTVN)

## Table of Contents
1. [What is Docker?](#what-is-docker)
2. [Prerequisites](#prerequisites)
3. [Quick Start](#quick-start)
4. [Understanding the Project Structure](#understanding-the-project-structure)
5. [Managing the Application](#managing-the-application)
6. [Monitoring and Troubleshooting](#monitoring-and-troubleshooting)
7. [Common Commands Reference](#common-commands-reference)
8. [FAQ and Troubleshooting](#faq-and-troubleshooting)

---

## What is Docker?

Docker is a platform that packages applications and their dependencies into **containers** - lightweight, portable units that run consistently across different environments.

### Key Concepts

- **Container**: A running instance of your application with everything it needs (code, libraries, dependencies)
- **Image**: A blueprint/template used to create containers
- **Docker Compose**: A tool for managing multi-container applications (like this project with 7 validators + monitoring)
- **Volume**: Persistent storage for container data that survives container restarts

Think of it like this:
- **Image** = Recipe
- **Container** = Dish made from the recipe
- **Docker Compose** = Making a complete meal with multiple dishes

---

## Prerequisites

### Installing Docker on Pop!_OS (Ubuntu-based)

```bash
# Update package list
sudo apt update

# Install required packages
sudo apt install -y apt-transport-https ca-certificates curl software-properties-common

# Add Docker's official GPG key
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg

# Add Docker repository
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install Docker
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Add your user to the docker group (so you don't need sudo)
sudo usermod -aG docker $USER

# Log out and log back in, then verify installation
docker --version
docker compose version
```

### Verify Installation

```bash
# Test Docker
docker run hello-world

# If successful, you'll see a welcome message
```

---

## Quick Start

### Starting the Network (All Nodes)

```bash
# Navigate to project directory
cd "/home/sathikashervinhettiarachchi/Programming/Go Projects/DTVN"

# Start all services (7 validators + Prometheus + Grafana)
docker compose up -d
```

**What happens:**
- `-d` means "detached mode" - runs in the background
- Docker builds images (first time takes 5-10 minutes)
- Starts 7 validator nodes, Prometheus, and Grafana
- Creates a network for all containers to communicate

### Checking Status

```bash
# See all running containers
docker compose ps
```

You should see 9 services running:
- `dtvn-validator-0` through `dtvn-validator-6`
- `dtvn-prometheus`
- `dtvn-grafana`

### Stopping the Network

```bash
# Stop all containers
docker compose down
```

**Note:** This stops containers but keeps your data (stored in volumes).

---

## Understanding the Project Structure

```
DTVN/
├── docker-compose.yml      # Defines all services and how they connect
├── Dockerfile             # Instructions to build validator container image
├── docker/
│   └── entrypoint.sh      # Startup script for each validator
├── cmd/
│   └── validator/
│       └── main.go        # Main application code
└── config/                # Configuration files
    ├── prometheus.yml     # Prometheus metrics configuration
    └── grafana/           # Grafana dashboards and datasources
```

### Service Ports (on your machine)

| Service | P2P Port | API Port | Metrics Port |
|---------|----------|----------|--------------|
| validator-0 (Primary) | 4001 | 8080 | 9090 |
| validator-1 | 4002 | 8081 | 9091 |
| validator-2 | 4003 | 8082 | 9092 |
| validator-3 | 4004 | 8083 | 9093 |
| validator-4 | 4005 | 8084 | 9094 |
| validator-5 | 4006 | 8085 | 9095 |
| validator-6 | 4007 | 8086 | 9096 |
| Prometheus | - | - | 9999 |
| Grafana | - | 3000 | - |

**Access Points:**
- Validator 0 API: http://localhost:8080/health
- Grafana Dashboard: http://localhost:3000 (user: admin, password: admin)
- Prometheus: http://localhost:9999

---

## Managing the Application

### Starting Services

```bash
# Start all services
docker compose up -d

# Start only specific services
docker compose up -d validator-0 validator-1

# Start and view logs in real-time (no -d flag)
docker compose up
# Press Ctrl+C to stop
```

### Viewing Logs

```bash
# View logs from all services
docker compose logs

# View logs from a specific service
docker compose logs validator-0

# Follow logs in real-time (like tail -f)
docker compose logs -f validator-0

# View last 50 lines
docker compose logs --tail=50 validator-0

# View logs from multiple services
docker compose logs validator-0 validator-1
```

### Restarting Services

```bash
# Restart all services
docker compose restart

# Restart a specific service
docker compose restart validator-0

# Restart multiple services
docker compose restart validator-0 validator-1
```

### Stopping Services

```bash
# Stop all services (keeps containers and data)
docker compose stop

# Stop specific service
docker compose stop validator-0

# Stop and remove containers (keeps data in volumes)
docker compose down

# Stop, remove containers AND volumes (deletes all data!)
docker compose down -v
# ⚠️ WARNING: This deletes all validator data!
```

### Rebuilding After Code Changes

```bash
# Rebuild images and restart
docker compose up -d --build

# Force rebuild without cache (if having issues)
docker compose build --no-cache
docker compose up -d
```

### Scaling (Adding More Validators)

You can start a subset of validators:

```bash
# Start only the primary and 2 validators
docker compose up -d validator-0 validator-1 validator-2

# Add more validators later
docker compose up -d validator-3 validator-4
```

---

## Monitoring and Troubleshooting

### Checking Container Health

```bash
# Check status of all containers
docker compose ps

# Check detailed information about a container
docker inspect dtvn-validator-0

# Check resource usage (CPU, Memory)
docker stats

# Check resource usage for specific containers
docker stats dtvn-validator-0 dtvn-validator-1
```

### Entering a Container (Debugging)

```bash
# Open a shell inside a running container
docker exec -it dtvn-validator-0 sh

# Inside the container, you can:
# - Check files: ls -la
# - View processes: ps aux
# - Check environment: env
# - View data directory: ls -la /home/validator/data
# - Exit: type 'exit' or press Ctrl+D
```

### Checking Network Connectivity

```bash
# List Docker networks
docker network ls

# Inspect the DTVN network
docker network inspect dtvn_dtvn-network

# Test connectivity between containers
docker exec dtvn-validator-1 ping validator-0
docker exec dtvn-validator-1 wget -O- http://validator-0:8080/health
```

### Viewing Metrics

1. **Prometheus** (Raw Metrics):
   - Open: http://localhost:9999
   - Query examples:
     - `up` - See which services are up
     - `process_cpu_seconds_total` - CPU usage

2. **Grafana** (Visual Dashboards):
   - Open: http://localhost:3000
   - Login: admin / admin
   - Import or create dashboards to visualize metrics

### Container Logs Location

```bash
# Docker stores logs at:
# /var/lib/docker/containers/<container-id>/<container-id>-json.log

# Easier way - use docker logs:
docker logs dtvn-validator-0

# View logs with timestamps
docker logs -t dtvn-validator-0
```

---

## Common Commands Reference

### Essential Commands

```bash
# START
docker compose up -d                    # Start all services in background
docker compose up -d validator-0        # Start only validator-0

# STOP
docker compose down                     # Stop and remove containers
docker compose stop                     # Stop containers (don't remove)

# STATUS
docker compose ps                       # List running containers
docker compose logs -f <service>        # Follow logs for a service

# RESTART
docker compose restart                  # Restart all services
docker compose restart <service>        # Restart specific service

# REBUILD
docker compose up -d --build           # Rebuild and restart

# CLEAN UP
docker compose down -v                  # Remove containers and volumes (⚠️ deletes data)
docker system prune                     # Clean up unused images/containers
docker system prune -a                  # Clean up everything unused
```

### Advanced Commands

```bash
# Execute commands inside a container
docker exec -it <container-name> <command>
docker exec -it dtvn-validator-0 sh

# Copy files between host and container
docker cp <container>:/path/to/file ./local/path
docker cp ./local/file <container>:/path/to/destination

# Export/Import volumes (backup data)
docker run --rm -v validator-0-data:/data -v $(pwd):/backup alpine tar czf /backup/validator-0-backup.tar.gz -C /data .
docker run --rm -v validator-0-data:/data -v $(pwd):/backup alpine tar xzf /backup/validator-0-backup.tar.gz -C /data

# View Docker disk usage
docker system df

# Follow resource usage in real-time
docker stats --all
```

---

## FAQ and Troubleshooting

### Q: Containers keep restarting

**Check the logs:**
```bash
docker compose logs <service-name>
```

**Common causes:**
- Port already in use (kill the other process)
- Configuration error (check environment variables)
- Application crash (check application logs)

### Q: "Port already allocated" error

**Find what's using the port:**
```bash
# Check what's using port 8080
sudo lsof -i :8080
# or
sudo netstat -tulpn | grep 8080

# Kill the process
sudo kill -9 <PID>
```

**Or change the port in docker-compose.yml:**
```yaml
ports:
  - "8081:8080"  # Change host port from 8080 to 8081
```

### Q: Can't connect to Docker daemon

**Error:** `Cannot connect to the Docker daemon at unix:///var/run/docker.sock`

**Solution:**
```bash
# Start Docker service
sudo systemctl start docker

# Enable Docker to start on boot
sudo systemctl enable docker

# Check Docker status
sudo systemctl status docker
```

### Q: Permission denied errors

**Add yourself to docker group:**
```bash
sudo usermod -aG docker $USER

# Log out and log back in, then test:
docker ps
```

### Q: Container is healthy but can't access from browser

**Check:**
1. Container is running: `docker compose ps`
2. Port mapping is correct in docker-compose.yml
3. Firewall isn't blocking the port: `sudo ufw status`
4. Access from localhost: `curl http://localhost:8080/health`

### Q: Out of disk space

**Clean up Docker:**
```bash
# See disk usage
docker system df

# Remove unused data
docker system prune

# Remove everything (⚠️ careful!)
docker system prune -a --volumes
```

### Q: Changes to code not reflecting

**Rebuild the images:**
```bash
# Rebuild and restart
docker compose up -d --build

# Force full rebuild
docker compose build --no-cache
docker compose up -d
```

### Q: How to reset everything?

**Complete reset:**
```bash
# Stop and remove everything
docker compose down -v

# Remove images
docker compose down --rmi all -v

# Rebuild and start fresh
docker compose up -d --build
```

### Q: Container exits immediately

**Check the logs:**
```bash
docker compose logs <service>
```

**Run without detached mode to see errors:**
```bash
docker compose up <service>
```

### Q: How to update to latest Docker version?

```bash
# Update package list
sudo apt update

# Upgrade Docker
sudo apt upgrade docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Verify version
docker --version
docker compose version
```

---

## Best Practices

### Development Workflow

1. **Make code changes** in your editor
2. **Rebuild** the affected service:
   ```bash
   docker compose up -d --build validator-0
   ```
3. **Check logs** for errors:
   ```bash
   docker compose logs -f validator-0
   ```
4. **Test** the changes via API or Grafana

### Production Deployment

1. **Set environment variables** for version tracking:
   ```bash
   export VERSION=1.0.0
   export COMMIT=$(git rev-parse HEAD)
   export BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
   docker compose up -d --build
   ```

2. **Backup data regularly**:
   ```bash
   docker run --rm \
     -v dtvn_validator-0-data:/data \
     -v $(pwd)/backups:/backup \
     alpine tar czf /backup/validator-0-$(date +%Y%m%d).tar.gz -C /data .
   ```

3. **Monitor logs and metrics** via Grafana

4. **Update images periodically**:
   ```bash
   docker compose pull  # Pull latest base images
   docker compose up -d --build
   ```

### Security Tips

1. **Change default Grafana password** in docker-compose.yml
2. **Don't expose all ports** to the internet - use a reverse proxy (nginx/traefik)
3. **Keep Docker updated**: `sudo apt update && sudo apt upgrade`
4. **Scan images for vulnerabilities**:
   ```bash
   docker scout quickview
   docker scout cves <image-name>
   ```

---

## Need Help?

- **Docker Documentation**: https://docs.docker.com
- **Docker Compose Documentation**: https://docs.docker.com/compose
- **Project Issues**: Check the GitHub repository

---

## Quick Reference Card

```bash
# DAILY USE
docker compose up -d              # Start everything
docker compose ps                 # Check status
docker compose logs -f <service>  # View logs
docker compose restart <service>  # Restart a service
docker compose down              # Stop everything

# AFTER CODE CHANGES
docker compose up -d --build     # Rebuild and restart

# TROUBLESHOOTING
docker compose logs <service>     # Check what went wrong
docker exec -it <container> sh    # Enter container to debug
docker system prune              # Clean up disk space

# MONITORING
http://localhost:3000            # Grafana (admin/admin)
http://localhost:9999            # Prometheus
http://localhost:8080/health     # Validator 0 API
```

---

**Happy containerizing! 🐳**
