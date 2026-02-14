# UI build stage
FROM node:22-alpine AS ui-builder

RUN corepack enable pnpm

WORKDIR /ui

# Copy package files first for better caching
COPY ui/src/package.json ui/src/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Copy UI source and build
COPY ui/src/ .
RUN pnpm build

# Go build stage
FROM golang:1.24-alpine AS builder

WORKDIR /workspace

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Copy built UI into the embed directory
COPY --from=ui-builder /ui/.output/public/ ./ui/dist/

# Build the server binary with embedded UI
RUN CGO_ENABLED=0 GOOS=linux go build -a -o server ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the binary from builder
COPY --from=builder /workspace/server .

# Use non-root user
USER 65532:65532

# Expose gRPC and HTTP ports
EXPOSE 50051 8080

ENTRYPOINT ["/server"]
