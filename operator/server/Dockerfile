# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /workspace

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -o server ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the binary from builder
COPY --from=builder /workspace/server .

# Use non-root user
USER 65532:65532

# Expose gRPC port
EXPOSE 50051

ENTRYPOINT ["/server"]
