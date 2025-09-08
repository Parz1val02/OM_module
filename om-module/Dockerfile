FROM golang:1.25-alpine AS builder

# Install git for go mod dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o om-module main.go

# Final stage
FROM alpine:latest

# Install required packages
RUN apk --no-cache add ca-certificates docker-cli curl

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/om-module .

# Create directories for configuration output
RUN mkdir -p /etc/prometheus/configs

# Expose all required ports
EXPOSE 9091 9092 9093 9094 9095 9096 8080 8081

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

# Run in orchestrator mode by default
CMD ["./om-module", "orchestrator"]
