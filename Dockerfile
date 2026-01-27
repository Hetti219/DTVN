# Build stage
FROM golang:1.25.5-alpine3.21 AS builder

# Add metadata labels
LABEL org.opencontainers.image.source="https://github.com/Hetti219/DTVN"
LABEL org.opencontainers.image.description="Distributed Ticket Validation Network - Validator Node"
LABEL org.opencontainers.image.licenses="MIT"

# Install build dependencies
RUN apk add --no-cache \
    git \
    make \
    gcc \
    musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies (cached unless go.mod/go.sum changes)
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build arguments for version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Build the application with optimizations
# -trimpath: removes file system paths from binary
# -ldflags: linker flags to reduce binary size and add version info
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build \
    -a \
    -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=${BUILD_DATE}" \
    -installsuffix cgo \
    -o /validator \
    cmd/validator/main.go

# Final stage - use specific Alpine version for reproducibility
FROM alpine:3.23

# Add metadata labels
LABEL org.opencontainers.image.source="https://github.com/Hetti219/DTVN"
LABEL org.opencontainers.image.description="Distributed Ticket Validation Network - Validator Node"
LABEL org.opencontainers.image.licenses="MIT"

# Install runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    wget \
    tzdata && \
    update-ca-certificates

# Create non-root user for security
RUN addgroup -g 1000 validator && \
    adduser -D -u 1000 -G validator validator && \
    mkdir -p /home/validator/data && \
    chown -R validator:validator /home/validator

# Switch to non-root user
USER validator
WORKDIR /home/validator

# Copy binary from builder with proper ownership
COPY --from=builder --chown=validator:validator /validator .

# Copy entrypoint script with proper ownership
COPY --chown=validator:validator docker/entrypoint.sh /home/validator/entrypoint.sh
RUN chmod +x /home/validator/entrypoint.sh

# Expose ports
# 4001: P2P port
# 8080: API port
# 9090: Metrics port
EXPOSE 4001 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

# Run the validator using entrypoint script
ENTRYPOINT ["/home/validator/entrypoint.sh"]
