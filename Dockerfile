# Build stage
FROM golang:1.25-alpine3.22 AS builder

# Install git and ca-certificates (needed for HTTPS requests)
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=${VERSION:-dev}" \
    -o plexify \
    main.go

# Final stage
FROM alpine:3.22

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S plexify && \
    adduser -u 1001 -S plexify -G plexify

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/plexify /app/plexify

# Change ownership to non-root user
RUN chown -R plexify:plexify /app

# Switch to non-root user
USER plexify

LABEL org.opencontainers.image.source="https://github.com/grrywlsn/plexify" \
    org.opencontainers.image.description="Sync Spotify playlists to Plex"

# Set the binary as the entrypoint
ENTRYPOINT ["/app/plexify"]

# Default command (can be overridden)
CMD ["--help"]
