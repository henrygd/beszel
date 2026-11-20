# Build stage
FROM golang:1.27.3-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates unzip

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source files
COPY . ./

# Build the hub binary
RUN CGO_ENABLED=0 GOGC=75 go build -ldflags "-w -s" -o /beszel ./internal/cmd/hub

# Build the agent binary
RUN CGO_ENABLED=0 GOGC=75 go build -ldflags "-w -s" -o /beszel-agent ./internal/cmd/agent

# Final hub image
FROM scratch AS hub

COPY --from=builder /beszel /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

VOLUME ["/beszel_data"]
EXPOSE 8090

ENTRYPOINT ["/beszel"]
CMD ["serve", "--http=0.0.0.0:8090"]

# Final agent image
FROM scratch AS agent

COPY --from=builder /beszel-agent /agent
COPY --from=builder /tmp /tmp
COPY --from=builder /app/agent/test-data/amdgpu.ids /usr/share/libdrm/amdgpu.ids
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

VOLUME ["/var/lib/beszel-agent"]

ENTRYPOINT ["/agent"]
