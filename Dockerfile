# Build stage
FROM golang:1.21-alpine AS builder

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
RUN apk --no-cache add ca-certificates

# Create app directory
WORKDIR /root/

# Copy binary from builder
COPY --from=builder /validator .

# Create data directory
RUN mkdir -p /root/data

# Expose ports
# 4001: P2P port
# 8080: API port
# 9090: Metrics port
EXPOSE 4001 8080 9090

# Set environment variables
ENV NODE_ID=validator-1
ENV LISTEN_PORT=4001
ENV API_PORT=8080
ENV DATA_DIR=/root/data

# Run the validator
ENTRYPOINT ["./validator"]

# Default command line arguments
CMD ["-id", "${NODE_ID}", "-port", "4001", "-api-port", "8080", "-data-dir", "/root/data"]
