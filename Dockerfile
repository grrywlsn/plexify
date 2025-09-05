FROM golang:1.25-alpine AS builder

RUN apk --no-cache add ca-certificates tzdata git

RUN adduser -D -u 10001 appuser

WORKDIR /build

COPY . .

RUN go mod vendor

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-w -s" -o plexify

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /build/plexify ./plexify
COPY --from=builder /etc/passwd /etc/passwd

LABEL org.opencontainers.image.source="https://github.com/grrywlsn/plexify" \
    org.opencontainers.image.description="Sync Spotify playlists to Plex" \

ENTRYPOINT ["./plexify"]
