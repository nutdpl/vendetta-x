#!/usr/bin/env bash
# Build the Vendetta/X server as a fully static Linux binary.
#
# The SQLite driver is modernc.org/sqlite (pure Go), so CGO_ENABLED=0 yields a
# self-contained binary with no libc dependency -- ideal for scratch/distroless
# images and for dropping straight onto a host.
set -euo pipefail

# Resolve the repo root (this script lives in <root>/deploy).
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

out="${OUT:-${repo_root}/vendx}"

echo "Building static vendx binary..."
(
    cd "${repo_root}/server"
    CGO_ENABLED=0 GOOS="${GOOS:-linux}" GOARCH="${GOARCH:-amd64}" \
        go build -ldflags="-s -w" -o "${out}" .
)

echo "Built: ${out}"
ls -la "${out}"
