# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app
# We need to copy both gen/ and go-orchestrator/ because go.mod scopes ../gen locally in mono-repo
COPY gen/ /app/gen/
COPY go-orchestrator/ /app/go-orchestrator/

WORKDIR /app/go-orchestrator
RUN go mod download

# Build the statically linked orchestrator binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /distagent-orchestrator ./cmd/orchestrator

# Run stage
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /distagent-orchestrator /app/

# Expose the API Gateway (ConnectRPC) and the Internal gRPC Heartbeat plane
EXPOSE 8080 8081

CMD ["/app/distagent-orchestrator"]
