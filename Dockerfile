# Vendetta/X -- container image for the Go BBS server.
#
# Multi-stage build:
#   - Stage 1 builds a fully static binary (CGO_ENABLED=0). The SQLite driver is
#     modernc.org/sqlite (pure Go), so no libc is needed and the binary runs on a
#     scratch/distroless base.
#   - Stage 2 is gcr.io/distroless/static:nonroot -- a tiny image with no shell,
#     no package manager, and a built-in nonroot user (uid 65532).
#
# Build context is the REPO ROOT (so we can copy both server/ and art/):
#   docker build -t vendetta-x .

# ---- Stage 1: builder ------------------------------------------------------
FROM golang:1.25 AS builder

# The Go module lives in server/ (module path "vendetta-x/server").
WORKDIR /src/server

# Prime the module cache first so dependency download is cached independently of
# source changes. go.sum is needed for a verified `go mod download`.
COPY server/go.mod server/go.sum ./
RUN go mod download

# Now the rest of the server source.
COPY server/ ./

# Build a static, stripped Linux binary. -s -w drops the symbol table and DWARF
# to shrink the image. CGO_ENABLED=0 forces the pure-Go path (no libc linkage).
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/vendx .

# Pre-create the state directory owned by the distroless nonroot uid/gid (65532)
# so we can COPY it into the final image with the right ownership. distroless has
# no shell, so we cannot mkdir/chown in the final stage -- we stage it here.
RUN mkdir -p /data && chown 65532:65532 /data

# ---- Stage 2: final --------------------------------------------------------
# distroless/static:nonroot: just CA certs, tzdata, /etc/passwd with a nonroot
# user (uid/gid 65532), and nothing else. Perfect for a static Go binary.
FROM gcr.io/distroless/static:nonroot

# App layout: binary + art/ under /app, persistent state under /data.
WORKDIR /app

# The compiled server binary.
COPY --from=builder /out/vendx /app/vendx

# The ANSI .pp art screens (repo-root art/, sibling of server/). Templates and
# CSS are embedded in the binary via go:embed -- only art/ is external.
COPY art/ /app/art/

# Persistent state volume. The SQLite database (+ -wal/-shm sidecars) and the
# SSH host key live here so they survive container restarts/recreations.
#
# IMPORTANT: the volume mounted at /data MUST be writable by uid 65532 (the
# distroless "nonroot" user), because the DB and host key are created on first
# run. We copy the pre-created, nonroot-owned /data from the builder stage so a
# Docker named volume inherits that ownership on first mount. If you bind-mount a
# host directory instead, chown it to 65532:65532 first (see deploy/README.md).
COPY --from=builder --chown=65532:65532 /data /data
VOLUME ["/data"]

# telnet, ssh, http (all >1024, so no extra capabilities are required).
EXPOSE 2323 2222 8080

# Run as the built-in nonroot user (uid 65532).
USER nonroot:nonroot

# Default command: art from the image, DB + host key on the /data volume, and the
# default listen addresses (which already bind 0.0.0.0 inside the container).
ENTRYPOINT ["/app/vendx", \
    "-art=/app/art", \
    "-db=/data/vendetta.db", \
    "-hostkey=/data/host_key", \
    "-telnet=:2323", \
    "-ssh=:2222", \
    "-http=:8080"]
