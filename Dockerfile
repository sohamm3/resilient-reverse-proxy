# ---------- BUILD STAGE ----------
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install CA certificates.
RUN apk add --no-cache ca-certificates

# Copy dependency manifests first for better layer caching.
COPY go.mod ./

# Download Go modules.
RUN go mod download

# Copy application source code.
COPY . .

# Build statically linked production binary.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o reverse-proxy \
    ./cmd/proxy

# ---------- RUNTIME STAGE ----------
# FROM scratch
FROM alpine:latest

WORKDIR /app

RUN adduser -D appuser

# Copy CA certificates for outbound HTTPS support.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy compiled application binary.
COPY --from=builder /app/reverse-proxy .

# Copy runtime configuration files.
COPY --from=builder /app/config ./config

USER appuser

# Expose reverse proxy port.
EXPOSE 8080

# HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/health || exit 1

# Start reverse proxy.
ENTRYPOINT ["./reverse-proxy"]