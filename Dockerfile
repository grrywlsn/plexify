# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26-alpine3.22 AS builder

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

# Build the binary for the platform the image is being produced for
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o plexify \
    main.go

# Final stage
FROM alpine:3.24

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
    org.opencontainers.image.description="Stateless sync of music-social playlists to Plex (no volumes required)"

# Set the binary as the entrypoint
ENTRYPOINT ["/app/plexify"]

# Default command (can be overridden)
CMD ["--help"]
