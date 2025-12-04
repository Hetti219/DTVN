# Build stage
FROM golang:1.25.1-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /validator cmd/validator/main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates wget

# Create app directory
WORKDIR /root/

# Copy binary from builder
COPY --from=builder /validator .

# Copy entrypoint script
COPY docker/entrypoint.sh /root/entrypoint.sh
RUN chmod +x /root/entrypoint.sh

# Create data directory
RUN mkdir -p /root/data

# Expose ports
# 4001: P2P port
# 8080: API port
# 9090: Metrics port
EXPOSE 4001 8080 9090

# Run the validator using entrypoint script
ENTRYPOINT ["/root/entrypoint.sh"]
