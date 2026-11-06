# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Build stage
#
# CGO is disabled because the SQLite driver is modernc.org/sqlite (pure Go),
# not mattn/go-sqlite3. That is the whole reason the hand-written dialector in
# internal/database exists: it keeps this image free of a C toolchain and lets
# the binary be a static executable.
# ---------------------------------------------------------------------------
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Copy the module files first so dependency download is cached independently of
# source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Trim paths and omit the symbol table and DWARF data: smaller image, and the
# panic traces we do log remain readable.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/api .


# Verify the binary is actually static. If this fails the runtime stage would
# fail confusingly with "no such file or directory" instead.
RUN file /out/api 2>/dev/null || true

# ---------------------------------------------------------------------------
# Runtime stage
#
# alpine rather than scratch: we want a shell and wget for the healthcheck, and
# ca-certificates so the container can make outbound TLS calls if the email
# sender is ever pointed at a real provider.
# ---------------------------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget tzdata \
    && addgroup -S app \
    && adduser -S -G app -u 10001 app

WORKDIR /app

COPY --from=builder /out/api /app/api

# SQLite needs a writable directory; mount a volume here for persistence.
RUN mkdir -p /data && chown -R app:app /data /app

# Run unprivileged. Nothing in the image needs root at runtime.
USER 10001:10001

ENV APP_ENV=production \
    HTTP_ADDR=:8080 \
    DB_DRIVER=sqlite \
    DB_DSN="file:/data/booking.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)" \
    DB_AUTO_MIGRATE=true \
    SHUTDOWN_TIMEOUT=15s \
    EMAIL_WORKER_COUNT=4 \
    EMAIL_QUEUE_SIZE=256 \
    EMAIL_SEND_DELAY=2s

EXPOSE 8080

# The healthcheck hits the API's own endpoint rather than just checking the
# port is open, so a process that is up but cannot reach its database is
# correctly reported unhealthy.
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/api"]
