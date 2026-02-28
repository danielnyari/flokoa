# Build from operator directory context: docker build -f plugins/gcpdocai/Dockerfile .
FROM golang:1.24 AS builder

WORKDIR /workspace

# Copy the main operator module (needed for replace directive)
COPY go.mod go.sum ./
COPY api/ api/
COPY internal/ internal/

# Copy plugin go mod files
COPY plugins/gcpdocai/go.mod plugins/gcpdocai/go.sum ./plugins/gcpdocai/

# Download dependencies
WORKDIR /workspace/plugins/gcpdocai
RUN go mod download

# Copy plugin source code
WORKDIR /workspace
COPY plugins/gcpdocai/ plugins/gcpdocai/

# Build the binary
WORKDIR /workspace/plugins/gcpdocai
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o /gcpdocai-plugin .

# Runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /gcpdocai-plugin .

USER 65532:65532

EXPOSE 4355

ENTRYPOINT ["/gcpdocai-plugin"]
