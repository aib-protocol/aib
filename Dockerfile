# Multi-stage build - stage 1: compile
FROM golang:1.22-alpine AS builder

# Install dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /build

# Clone and compile
RUN git clone https://github.com/aib-protocol/aib . && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o aib-node ./cmd/aib-node

# Stage 2: runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk update && \
    apk add --no-cache ca-certificates wget tzdata && \
    rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 aib && \
    adduser -D -u 1000 -G aib aib

# Set working directory
WORKDIR /app

# Copy binary from the build stage
COPY --from=builder /build/aib-node /app/aib-node

# Create data directory
RUN mkdir -p /data && chown -R aib:aib /data

# Switch to non-root user
USER aib

# Expose ports
EXPOSE 51211 31415

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -s http://localhost:51211/health || exit 1

# Default command
CMD ["/app/aib-node", "--validator", "--api-port", "51211", "--p2p-port", "31415", "--data-dir", "/data", "--block-time", "60"]
