#!/usr/bin/env bash
#
# Build and cache a local pg-xpatch PostgreSQL image for testing.
#
# This clones the pg-xpatch repo, builds the Docker image locally, and tags
# it as pgit-xpatch-test:latest. The testdb package uses this image instead
# of pulling from GHCR (which requires authentication).
#
# Usage:
#   ./scripts/build-test-db.sh          # build (or use cache)
#   ./scripts/build-test-db.sh --force  # rebuild from scratch
#
# The image is cached — subsequent runs are instant unless --force is passed
# or the pg-xpatch repo has new commits.

set -euo pipefail

IMAGE_NAME="pgit-xpatch-test:latest"
REPO_URL="https://github.com/imgajeed76/pg-xpatch.git"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/pgit/pg-xpatch-src"
FORCE=false

for arg in "$@"; do
    case "$arg" in
        --force) FORCE=true ;;
        --help|-h)
            echo "Usage: $0 [--force]"
            echo "Build a local pg-xpatch Docker image for testing."
            echo ""
            echo "  --force    Rebuild even if the image is up to date"
            exit 0
            ;;
    esac
done

# --- Detect container runtime ---
if command -v docker &>/dev/null; then
    RUNTIME=docker
elif command -v podman &>/dev/null; then
    RUNTIME=podman
else
    echo "Error: neither docker nor podman found" >&2
    exit 1
fi

echo "Using container runtime: $RUNTIME"

# --- Check if image already exists ---
if [ "$FORCE" = false ] && $RUNTIME image inspect "$IMAGE_NAME" &>/dev/null; then
    echo "Image $IMAGE_NAME already exists (use --force to rebuild)"
    exit 0
fi

# --- Clone or update source ---
if [ -d "$CACHE_DIR/.git" ]; then
    echo "Updating pg-xpatch source in $CACHE_DIR ..."
    git -C "$CACHE_DIR" fetch --quiet origin
    git -C "$CACHE_DIR" reset --quiet --hard origin/main
else
    echo "Cloning pg-xpatch into $CACHE_DIR ..."
    mkdir -p "$(dirname "$CACHE_DIR")"
    git clone --quiet --depth 1 "$REPO_URL" "$CACHE_DIR"
fi

# --- Build image ---
echo "Building $IMAGE_NAME (this takes a few minutes on first run) ..."
$RUNTIME build \
    --tag "$IMAGE_NAME" \
    --file "$CACHE_DIR/Dockerfile" \
    "$CACHE_DIR"

echo ""
echo "Done. Image: $IMAGE_NAME"
echo "Run tests with: go test ./internal/... -count=1 -timeout 300s"
