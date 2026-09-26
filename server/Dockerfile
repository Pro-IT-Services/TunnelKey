# syntax=docker/dockerfile:1

# ---- build ---------------------------------------------------------------
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Pure-Go SQLite driver: no cgo, fully static binary.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tunnelkey-server . \
    && mkdir -p /out/data

# ---- runtime -------------------------------------------------------------
# Distroless: no shell or package manager, runs as uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/tunnelkey-server /usr/local/bin/tunnelkey-server
# Owned by the runtime user so a fresh named volume inherits writable permissions.
COPY --from=build --chown=65532:65532 /out/data /data

ENV TUNNELKEY_LISTEN=0.0.0.0:9897 \
    TUNNELKEY_DATA=/data

EXPOSE 9897
VOLUME ["/data"]
USER nonroot:nonroot

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/tunnelkey-server", "healthcheck"]

ENTRYPOINT ["/usr/local/bin/tunnelkey-server"]
