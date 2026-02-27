# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /jimeng-server ./cmd/server

# Stage 2: Runtime
FROM alpine:3.19

# Install ca-certificates, timezone data, and wget (for health checks)
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /jimeng-server .

# Change ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Railway injects PORT env var, Default to 8080.
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:${PORT:-8080}/health || exit 1

# Entry point
ENTRYPOINT ["./jimeng-server"]

# Default command
CMD ["serve"]
