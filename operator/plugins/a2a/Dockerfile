# Build from operator directory context: docker build -f plugins/a2a/Dockerfile .
FROM golang:1.24 AS builder

WORKDIR /workspace

# Copy the main operator module (needed for replace directive)
COPY go.mod go.sum ./
COPY api/ api/

# Copy plugin go mod files
COPY plugins/a2a/go.mod plugins/a2a/go.sum ./plugins/a2a/

# Download dependencies
WORKDIR /workspace/plugins/a2a
RUN go mod download

# Copy plugin source code
WORKDIR /workspace
COPY plugins/a2a/ plugins/a2a/

# Build the binary
WORKDIR /workspace/plugins/a2a
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o /a2a-plugin .

# Runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /a2a-plugin .

USER 65532:65532

EXPOSE 4355

ENTRYPOINT ["/a2a-plugin"]
